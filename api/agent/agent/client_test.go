// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	stdtesting "testing"

	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/agent"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

type clientSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) TestStateServingInfo(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Agent")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "StateServingInfo")
		c.Assert(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StateServingInfo{})
		*result.(*params.StateServingInfo) = params.StateServingInfo{
			APIPort:           666,
			ControllerAPIPort: 668,
			StatePort:         669,
			Cert:              "some-cert",
			PrivateKey:        "some-key",
			CAPrivateKey:      "private-key",
			SharedSecret:      "secret",
			SystemIdentity:    "fred",
		}
		return nil
	})
	client, err := agent.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	info, err := client.StateServingInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, controller.StateServingInfo{
		APIPort:           666,
		ControllerAPIPort: 668,
		StatePort:         669,
		Cert:              "some-cert",
		PrivateKey:        "some-key",
		CAPrivateKey:      "private-key",
		SharedSecret:      "secret",
		SystemIdentity:    "fred",
	})
}

func (s *clientSuite) TestIsControllerShortCircuits(c *gc.C) {
	result, err := agent.IsController(nil, names.NewControllerAgentTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.IsTrue)
}

func (s *clientSuite) TestMachineEntity(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Agent")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetEntities")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-42"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.AgentGetEntitiesResults{})
		*result.(*params.AgentGetEntitiesResults) = params.AgentGetEntitiesResults{
			Entities: []params.AgentGetEntitiesResult{{
				Life: "alive",
				Jobs: []model.MachineJob{model.JobHostUnits},
			}},
		}
		return nil
	})
	tag := names.NewMachineTag("42")
	client, err := agent.NewClient(apiCaller)
	c.Assert(err, jc.ErrorIsNil)
	m, err := client.Entity(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Tag(), gc.Equals, tag.String())
	c.Assert(m.Life(), gc.Equals, life.Alive)
	c.Assert(m.Jobs(), gc.DeepEquals, []model.MachineJob{model.JobHostUnits})
}
