// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
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

	newFactory := factory.NewFactory(s.State)
	// Create two machines, two applications and add a unit to each service.
	s.machine0 = newFactory.MakeMachine(c, &factory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits, state.JobManageModel},
	})
	s.machine1 = newFactory.MakeMachine(c, &factory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
	s.machine2 = newFactory.MakeMachine(c, &factory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})

	s.wpCharm = newFactory.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress",
		URL:  "cs:quantal/wordpress-0",
	})
	s.wordpress = newFactory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: s.wpCharm,
	})
	s.wordpressUnit = newFactory.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine0,
	})

	mysqlCharm := newFactory.MakeCharm(c, &factory.CharmParams{
		Name: "mysql",
	})
	s.mysql = newFactory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql",
		Charm: mysqlCharm,
	})
	s.mysqlUnit = newFactory.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming unit 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.mysqlUnit.Tag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	uniterAPI, err := uniter.NewUniterAPI(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.uniter = uniterAPI
}

var (
	timestamp          = time.Date(2200, time.November, 5, 0, 0, 0, 0, time.UTC)
	charmStatusWaiting = params.GoalStateStatus{
		Status: "waiting",
		Since:  &timestamp,
	}
	expectedUnitWordpress = params.UnitsGoalState{
		"wordpress/0": charmStatusWaiting,
	}
	expectedUnitMysql = params.UnitsGoalState{
		"mysql/0": charmStatusWaiting,
	}
	expected2UnitsMysql = params.UnitsGoalState{
		"mysql/0": charmStatusWaiting,
		"mysql/1": charmStatusWaiting,
	}
)

// TestGoalStatesNoRelation tests single application with single unit.
func (s *uniterGoalStateSuite) TestGoalStatesNoRelation(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
	}}
	expected := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expectedUnitMysql,
				},
			}, {
				Error: apiservertesting.ErrUnauthorized,
			},
		},
	}
	testGoalStates(c, s.uniter, args, expected)
}

// TestGoalStatesNoRelationTwoUnits adds a new unit to mysql application.
func (s *uniterGoalStateSuite) TestPeerUnitsNoRelation(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
	}}

	addApplicationUnitToMachine(c, s.mysql, s.machine1)

	expected := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expected2UnitsMysql,
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
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
	}}
	expected := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expectedUnitMysql,
					Relations: map[string]params.UnitsGoalState{
						"db":     expectedUnitWordpress,
						"server": expectedUnitMysql,
					},
				},
			}, {
				Error: apiservertesting.ErrUnauthorized,
			},
		},
	}
	testGoalStates(c, s.uniter, args, expected)
}

// TestGoalStatesMultipleRelations tests GoalStates with three
// applications one application has two units and each unit is related
// to a different application unit.
func (s *uniterGoalStateSuite) TestGoalStatesMultipleRelations(c *gc.C) {

	// Add another mysql unit on machine 1.
	addApplicationUnitToMachine(c, s.mysql, s.machine1)

	// new application needs: charm, app and unit instanciated in the new machine
	newFactory := factory.NewFactory(s.State)
	wpCharm1 := newFactory.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress",
		URL:  "cs:quantal/wordpress-1",
	})
	wordpress1 := newFactory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress1",
		Charm: wpCharm1,
	})
	wordpress1Unit := newFactory.MakeUnit(c, &factory.UnitParams{
		Application: wordpress1,
		Machine:     s.machine2,
	})

	var err error
	err = s.addRelationToSuiteScope(c, wordpress1Unit, s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)

	err = s.addRelationToSuiteScope(c, s.wordpressUnit, s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
	}}

	expected := params.GoalStateResults{
		Results: []params.GoalStateResult{
			{
				Result: &params.GoalState{
					Units: expected2UnitsMysql,
					Relations: map[string]params.UnitsGoalState{
						"db": params.UnitsGoalState{
							"wordpress/0":  charmStatusWaiting,
							"wordpress1/0": charmStatusWaiting,
						},
						"server": expected2UnitsMysql,
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

// goalStates call untier.GoalStates API and compares the output with the
// expected result
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
// avoid a time check which otherwise would be impossible to pass
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
