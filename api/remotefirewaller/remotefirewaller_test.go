// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/remotefirewaller"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&RemoteFirewallersSuite{})

type RemoteFirewallersSuite struct {
	coretesting.BaseSuite
}

func (s *RemoteFirewallersSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := remotefirewaller.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func (s *RemoteFirewallersSuite) TestWatchIngressAddressesForRelation(c *gc.C) {
	var callCount int
	remoteRelationId := params.RemoteEntityId{ModelUUID: "model-uuid", Token: "token"}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteFirewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchIngressAddressesForRelation")
		c.Assert(arg, gc.DeepEquals, params.RemoteEntities{Entities: []params.RemoteEntityId{remoteRelationId}})
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResult{})
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		callCount++
		return nil
	})
	client := remotefirewaller.NewClient(apiCaller)
	_, err := client.WatchIngressAddressesForRelation(remoteRelationId)
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *RemoteFirewallersSuite) TestIngressSubnetsForRelation(c *gc.C) {
	var callCount int
	remoteRelationId := params.RemoteEntityId{ModelUUID: "model-uuid", Token: "token"}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "RemoteFirewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "IngressSubnetsForRelations")
		c.Assert(arg, gc.DeepEquals, params.RemoteEntities{Entities: []params.RemoteEntityId{remoteRelationId}})
		c.Assert(result, gc.FitsTypeOf, &params.IngressSubnetResults{})
		*(result.(*params.IngressSubnetResults)) = params.IngressSubnetResults{
			Results: []params.IngressSubnetResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := remotefirewaller.NewClient(apiCaller)
	_, err := client.IngressSubnetsForRelation(remoteRelationId)
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}
