// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charm "gopkg.in/juju/charm.v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// uniterSuite implements common testing suite for all API
// versions. It's not intended to be used directly or registered as a
// suite, but embedded.
type uniterGoalStateSuite struct {
	testing.JujuConnSuite

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources
	uniter     *uniter.UniterAPI

	machine0      *state.Machine
	machine1      *state.Machine
	machine2      *state.Machine
	wordpress     *state.Application
	wpCharm       *state.Charm
	mysql         *state.Application
	wordpressUnit *state.Unit
	mysqlUnit     *state.Unit
}

var _ = gc.Suite(&uniterGoalStateSuite{})

func (s *uniterGoalStateSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create two machines, two applications and add a unit to each application.
	s.machine0 = s.Factory.MakeMachine(c, &factory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits, state.JobManageModel},
	})
	s.machine1 = s.Factory.MakeMachine(c, &factory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
	s.machine2 = s.Factory.MakeMachine(c, &factory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})

	mysqlCharm := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "mysql",
	})
	s.mysql = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql",
		Charm: mysqlCharm,
	})

	s.mysqlUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})

	s.wpCharm = s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress",
		URL:  "cs:quantal/wordpress-0",
	})
	s.wordpress = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: s.wpCharm,
	})
	s.wordpressUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine0,
	})

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming unit 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.wordpressUnit.Tag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	uniterAPI, err := uniter.NewUniterAPI(facadetest.Context{
		State_:             s.State,
		Resources_:         s.resources,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.State.LeadershipChecker(),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.uniter = uniterAPI
}

var (
	timestamp          = time.Date(2200, time.November, 5, 0, 0, 0, 0, time.UTC)
	expectedUnitStatus = params.GoalStateStatus{
		Status: "waiting",
		Since:  &timestamp,
	}
	expectedRelationStatus = params.GoalStateStatus{
		Status: "joining",
		Since:  &timestamp,
	}
	expectedUnitWordpress = params.UnitsGoalState{
		"wordpress/0": expectedUnitStatus,
	}
	expected2UnitsWordPress = params.UnitsGoalState{
		"wordpress/0": expectedUnitStatus,
		"wordpress/1": expectedUnitStatus,
	}
	expectedUnitMysql = params.UnitsGoalState{
		"mysql/0": expectedUnitStatus,
	}
)

// TestGoalStatesNoRelation tests single application with single unit.
func (s *uniterGoalStateSuite) TestGoalStatesNoRelation(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-mysql-0"},
	}}
	expected := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expectedUnitWordpress,
				},
			}, {
				Error: apiservertesting.ErrUnauthorized,
			},
		},
	}
	testGoalStates(c, s.uniter, args, expected)
}

// TestGoalStatesNoRelationTwoUnits adds a new unit to wordpress application.
func (s *uniterGoalStateSuite) TestPeerUnitsNoRelation(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-mysql-0"},
	}}

	addApplicationUnitToMachine(c, s.wordpress, s.machine1)

	expected := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expected2UnitsWordPress,
				},
			}, {
				Error: apiservertesting.ErrUnauthorized,
			},
		},
	}
	testGoalStates(c, s.uniter, args, expected)
}

// TestGoalStatesSingleRelation tests structure with two different
// application units and one relation between the units.
func (s *uniterGoalStateSuite) TestGoalStatesSingleRelation(c *gc.C) {

	err := s.addRelationToSuiteScope(c, s.wordpressUnit, s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-mysql-0"},
	}}
	expected := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expectedUnitWordpress,
					Relations: map[string]params.UnitsGoalState{
						"server": {
							"mysql":   expectedRelationStatus,
							"mysql/0": expectedUnitStatus,
						},
					},
				},
			}, {
				Error: apiservertesting.ErrUnauthorized,
			},
		},
	}
	testGoalStates(c, s.uniter, args, expected)
}

// TestGoalStatesDeadUnitsExcluded tests dead units should not show in the GoalState result.
func (s *uniterGoalStateSuite) TestGoalStatesDeadUnitsExcluded(c *gc.C) {

	err := s.addRelationToSuiteScope(c, s.wordpressUnit, s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)

	newWordPressUnit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine1,
	})
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
	}}
	testGoalStates(c, s.uniter, args, params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expected2UnitsWordPress,
					Relations: map[string]params.UnitsGoalState{
						"server": {
							"mysql":   expectedRelationStatus,
							"mysql/0": expectedUnitStatus,
						},
					},
				},
			},
		},
	})

	newWordPressUnit.Destroy()

	testGoalStates(c, s.uniter, args, params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: params.UnitsGoalState{
						"wordpress/0": expectedUnitStatus,
					},
					Relations: map[string]params.UnitsGoalState{
						"server": {
							"mysql":   expectedRelationStatus,
							"mysql/0": expectedUnitStatus,
						},
					},
				},
			},
		},
	})
}

// preventUnitDestroyRemove sets a non-allocating status on the unit, and hence
// prevents it from being unceremoniously removed from state on Destroy. This
// is useful because several tests go through a unit's lifecycle step by step,
// asserting the behaviour of a given method in each state, and the unit quick-
// remove change caused many of these to fail.
func preventUnitDestroyRemove(c *gc.C, u *state.Unit) {
	// To have a non-allocating status, a unit needs to
	// be assigned to a machine.
	_, err := u.AssignedMachineId()
	if errors.IsNotAssigned(err) {
		err = u.AssignToNewMachine()
	}
	c.Assert(err, jc.ErrorIsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = u.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
}

// TestGoalStatesSingleRelationDyingUnits tests dying units showing dying status in the GoalState result.
func (s *uniterGoalStateSuite) TestGoalStatesSingleRelationDyingUnits(c *gc.C) {
	wordPressUnit2 := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine1,
	})

	err := s.addRelationToSuiteScope(c, wordPressUnit2, s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
	}}
	testGoalStates(c, s.uniter, args, params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expected2UnitsWordPress,
					Relations: map[string]params.UnitsGoalState{
						"server": {
							"mysql":   expectedRelationStatus,
							"mysql/0": expectedUnitStatus,
						},
					},
				},
			},
		},
	})
	preventUnitDestroyRemove(c, wordPressUnit2)
	err = wordPressUnit2.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	testGoalStates(c, s.uniter, args, params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: params.UnitsGoalState{
						"wordpress/0": expectedUnitStatus,
						"wordpress/1": params.GoalStateStatus{
							Status: "dying",
							Since:  &timestamp,
						},
					},
					Relations: map[string]params.UnitsGoalState{
						"server": {
							"mysql":   expectedRelationStatus,
							"mysql/0": expectedUnitStatus,
						},
					},
				},
			},
		},
	})
}

// TestGoalStatesCrossModelRelation tests remote relation application shows URL as key.
func (s *uniterGoalStateSuite) TestGoalStatesCrossModelRelation(c *gc.C) {
	err := s.addRelationToSuiteScope(c, s.wordpressUnit, s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
	}}
	testGoalStates(c, s.uniter, args, params.GoalStateResults{Results: []params.GoalStateResult{
		{
			Result: &params.GoalState{
				Units: params.UnitsGoalState{
					"wordpress/0": expectedUnitStatus,
				},
				Relations: map[string]params.UnitsGoalState{
					"server": {
						"mysql":   expectedRelationStatus,
						"mysql/0": expectedUnitStatus,
					},
				},
			},
		},
	}})
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql-remote",
		URL:         "ctrl1:admin/default.mysql",
		SourceModel: coretesting.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "server",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("wordpress", "mysql-remote")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	testGoalStates(c, s.uniter, args, params.GoalStateResults{Results: []params.GoalStateResult{
		{
			Result: &params.GoalState{
				Units: params.UnitsGoalState{
					"wordpress/0": expectedUnitStatus,
				},
				Relations: map[string]params.UnitsGoalState{
					"server": {
						"mysql":                     expectedRelationStatus,
						"mysql/0":                   expectedUnitStatus,
						"ctrl1:admin/default.mysql": expectedRelationStatus,
					},
				},
			},
		},
	}})
}

// TestGoalStatesMultipleRelations tests GoalStates with three
// applications one application has two units and each unit is related
// to a different application unit.
func (s *uniterGoalStateSuite) TestGoalStatesMultipleRelations(c *gc.C) {

	// Add another mysql unit on machine 1.
	addApplicationUnitToMachine(c, s.wordpress, s.machine1)

	mysqlCharm1 := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "mysql",
	})
	mysql1 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql1",
		Charm: mysqlCharm1,
	})

	mysqlUnit1 := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: mysql1,
		Machine:     s.machine2,
	})

	err := s.addRelationToSuiteScope(c, s.wordpressUnit, mysqlUnit1)
	c.Assert(err, jc.ErrorIsNil)

	err = s.addRelationToSuiteScope(c, s.wordpressUnit, s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-mysql-0"},
	}}

	expected := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expected2UnitsWordPress,
					Relations: map[string]params.UnitsGoalState{
						"server": {
							"mysql":    expectedRelationStatus,
							"mysql/0":  expectedUnitStatus,
							"mysql1":   expectedRelationStatus,
							"mysql1/0": expectedUnitStatus,
						},
					},
				},
			}, {
				Error: apiservertesting.ErrUnauthorized,
			},
		},
	}

	testGoalStates(c, s.uniter, args, expected)
}

func (s *uniterGoalStateSuite) addRelationToSuiteScope(c *gc.C, unit1 *state.Unit, unit2 *state.Unit) error {
	app1, err := unit1.Application()
	c.Assert(err, jc.ErrorIsNil)

	app2, err := unit2.Application()
	c.Assert(err, jc.ErrorIsNil)

	relation := s.addRelation(c, app1.Name(), app2.Name())
	relationUnit, err := relation.Unit(unit1)
	c.Assert(err, jc.ErrorIsNil)

	settings := map[string]interface{}{
		"some": "settings",
	}
	err = relationUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relationUnit, true)
	return err
}

func (s *uniterGoalStateSuite) addRelation(c *gc.C, first, second string) *state.Relation {
	eps, err := s.State.InferEndpoints(first, second)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	return rel
}

func (s *uniterGoalStateSuite) assertInScope(c *gc.C, relUnit *state.RelationUnit, inScope bool) {
	ok, err := relUnit.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, gc.Equals, inScope)
}

// goalStates call uniter.GoalStates API and compares the output with the
// expected result.
func testGoalStates(c *gc.C, thisUniter *uniter.UniterAPI, args params.Entities, expected params.GoalStateResults) {
	result, err := thisUniter.GoalStates(args)
	c.Assert(err, jc.ErrorIsNil)
	for i := range result.Results {
		if result.Results[i].Error != nil {
			break
		}
		setSinceToNil(c, result.Results[i].Result)
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expected)
}

// setSinceToNil will set the field `since` to nil in order to
// avoid a time check which otherwise would be impossible to pass.
func setSinceToNil(c *gc.C, goalState *params.GoalState) {

	for i, u := range goalState.Units {
		c.Assert(u.Since, gc.NotNil)
		u.Since = &timestamp
		goalState.Units[i] = u
	}
	for endPoint, gs := range goalState.Relations {
		for key, m := range gs {
			c.Assert(m.Since, gc.NotNil)
			m.Since = &timestamp
			gs[key] = m
		}
		goalState.Relations[endPoint] = gs
	}
}

func addApplicationUnitToMachine(c *gc.C, app *state.Application, machine *state.Machine) {
	mysqlUnit1, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlUnit1.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
}
