// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions_test

import (
	"testing"

	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/actions"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	jujuFactory "github.com/juju/juju/testing/factory"
)

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

type actionsSuite struct {
	jujutesting.JujuConnSuite

	actions    *actions.ActionsAPI
	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	charm         *state.Charm
	machine0      *state.Machine
	machine1      *state.Machine
	wordpress     *state.Service
	mysql         *state.Service
	wordpressUnit *state.Unit
	mysqlUnit     *state.Unit
}

var _ = gc.Suite(&actionsSuite{})

func (s *actionsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.actions, err = actions.NewActionsAPI(s.State, nil, s.authorizer)
	c.Assert(err, gc.IsNil)

	factory := jujuFactory.NewFactory(s.State)

	s.charm = factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "wordpress",
	})

	s.wordpress = factory.MakeService(c, &jujuFactory.ServiceParams{
		Name:    "wordpress",
		Charm:   s.charm,
		Creator: s.AdminUserTag(c),
	})
	s.machine0 = factory.MakeMachine(c, &jujuFactory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits, state.JobManageEnviron},
	})
	s.wordpressUnit = factory.MakeUnit(c, &jujuFactory.UnitParams{
		Service: s.wordpress,
		Machine: s.machine0,
	})

	mysqlCharm := factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "mysql",
	})
	s.mysql = factory.MakeService(c, &jujuFactory.ServiceParams{
		Name:    "mysql",
		Charm:   mysqlCharm,
		Creator: s.AdminUserTag(c),
	})
	s.machine1 = factory.MakeMachine(c, &jujuFactory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})
	s.mysqlUnit = factory.MakeUnit(c, &jujuFactory.UnitParams{
		Service: s.mysql,
		Machine: s.machine1,
	})
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
}

func (s *actionsSuite) TestEnqueue(c *gc.C) {
	// TODO(jcw4) implement
	c.Skip("Enqueue not yet implemented")
}

func (s *actionsSuite) TestListAll(c *gc.C) {
	// Make sure no Actions already exist on Unit.
	arg := params.Tags{Tags: []names.Tag{
		s.wordpressUnit.Tag(),
		s.mysqlUnit.Tag(),
	}}

	actionList, err := s.actions.ListAll(arg)
	c.Assert(err, gc.IsNil)
	expected := params.ActionsByTag{
		Actions: []params.Actions{{
			Receiver:    s.wordpressUnit.Tag(),
			ActionItems: []params.ActionItem{},
		}, {
			Receiver:    s.mysqlUnit.Tag(),
			ActionItems: []params.ActionItem{},
		}},
	}
	c.Assert(actionList, gc.DeepEquals, expected)

	// Add an Action.
	action, err := s.wordpressUnit.AddAction("foo", nil)
	c.Assert(err, gc.IsNil)

	// And make sure it is listed.
	arg = params.Tags{Tags: []names.Tag{
		s.wordpressUnit.Tag(),
		s.mysqlUnit.Tag(),
		s.wordpress.Tag(),
		s.mysql.Tag(),
	}}
	actionList, err = s.actions.ListAll(arg)
	c.Assert(err, gc.IsNil)
	expectError := &params.Error{Message: "id not found", Code: "not found"}
	expected = params.ActionsByTag{
		Actions: []params.Actions{{
			Receiver: s.wordpressUnit.Tag(),
			ActionItems: []params.ActionItem{{
				Tag:        action.ActionTag(),
				Name:       "foo",
				Status:     "pending",
				Parameters: map[string]interface{}{},
			}},
		}, {
			Receiver:    s.mysqlUnit.Tag(),
			ActionItems: []params.ActionItem{},
		}, {
			Error: expectError,
		}, {
			Error: expectError,
		}},
	}
	c.Assert(actionList, gc.DeepEquals, expected)
}

func (s *actionsSuite) TestListPending(c *gc.C) {
	// TODO(jcw4) implement
	c.Skip("ListPending not yet implemented")
}

func (s *actionsSuite) TestListCompleted(c *gc.C) {
	// TODO(jcw4) implement
	c.Skip("ListCompleted not yet implemented")
}

func (s *actionsSuite) TestCancel(c *gc.C) {
	// TODO(jcw4) implement
	c.Skip("Cancel not yet implemented")
}
