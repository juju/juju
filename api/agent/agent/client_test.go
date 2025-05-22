// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/agent"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)


type clientSuite struct {
	testhelpers.IsolationSuite
}

func TestClientSuite(t *stdtesting.T) { tc.Run(t, &clientSuite{}) }
func (s *clientSuite) TestStateServingInfo(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Agent")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "StateServingInfo")
		c.Assert(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.StateServingInfo{})
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
	c.Assert(err, tc.ErrorIsNil)
	info, err := client.StateServingInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, controller.StateServingInfo{
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

func (s *clientSuite) TestIsControllerShortCircuits(c *tc.C) {
	result, err := agent.IsController(c.Context(), nil, names.NewControllerAgentTag("0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.IsTrue)
}

func (s *clientSuite) TestMachineEntity(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Agent")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetEntities")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-42"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.AgentGetEntitiesResults{})
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
	c.Assert(err, tc.ErrorIsNil)
	m, err := client.Entity(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m.Tag(), tc.Equals, tag.String())
	c.Assert(m.Life(), tc.Equals, life.Alive)
	c.Assert(m.Jobs(), tc.DeepEquals, []model.MachineJob{model.JobHostUnits})
}
