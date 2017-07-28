// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/crossmodelrelations"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&CrossModelRelationsSuite{})

type CrossModelRelationsSuite struct {
	coretesting.BaseSuite
}

func (s *CrossModelRelationsSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := crossmodelrelations.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func (s *CrossModelRelationsSuite) TestPublishRelationChange(c *gc.C) {
	var callCount int
	mac, err := macaroon.New(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "PublishRelationChanges")
		c.Check(arg, gc.DeepEquals, params.RemoteRelationsChanges{
			Changes: []params.RemoteRelationChangeEvent{{
				DepartedUnits: []int{1}, Macaroons: macaroon.Slice{mac}}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClient(apiCaller)
	err = client.PublishRelationChange(params.RemoteRelationChangeEvent{
		DepartedUnits: []int{1},
		Macaroons:     macaroon.Slice{mac},
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *CrossModelRelationsSuite) TestRegisterRemoteRelations(c *gc.C) {
	var callCount int
	mac, err := macaroon.New(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RegisterRemoteRelations")
		c.Check(arg, gc.DeepEquals, params.RegisterRemoteRelationArgs{
			Relations: []params.RegisterRemoteRelationArg{{OfferName: "offeredapp", Macaroons: macaroon.Slice{mac}}}})
		c.Assert(result, gc.FitsTypeOf, &params.RegisterRemoteRelationResults{})
		*(result.(*params.RegisterRemoteRelationResults)) = params.RegisterRemoteRelationResults{
			Results: []params.RegisterRemoteRelationResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClient(apiCaller)
	result, err := client.RegisterRemoteRelations(params.RegisterRemoteRelationArg{
		OfferName: "offeredapp",
		Macaroons: macaroon.Slice{mac},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *CrossModelRelationsSuite) TestRegisterRemoteRelationCount(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.RegisterRemoteRelationResults)) = params.RegisterRemoteRelationResults{
			Results: []params.RegisterRemoteRelationResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	client := crossmodelrelations.NewClient(apiCaller)
	_, err := client.RegisterRemoteRelations(params.RegisterRemoteRelationArg{})
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *CrossModelRelationsSuite) TestWatchRelationUnits(c *gc.C) {
	remoteRelationToken := "token"
	mac, err := macaroon.New(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(arg, jc.DeepEquals, params.RemoteEntityArgs{Args: []params.RemoteEntityArg{{
			Token: remoteRelationToken, Macaroons: macaroon.Slice{mac}}}})
		c.Check(request, gc.Equals, "WatchRelationUnits")
		c.Assert(result, gc.FitsTypeOf, &params.RelationUnitsWatchResults{})
		*(result.(*params.RelationUnitsWatchResults)) = params.RelationUnitsWatchResults{
			Results: []params.RelationUnitsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClient(apiCaller)
	_, err = client.WatchRelationUnits(params.RemoteEntityArg{
		Token:     remoteRelationToken,
		Macaroons: macaroon.Slice{mac},
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *CrossModelRelationsSuite) TestRelationUnitSettings(c *gc.C) {
	remoteRelationToken := "token"
	mac, err := macaroon.New(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "RelationUnitSettings")
		c.Check(arg, gc.DeepEquals, params.RemoteRelationUnits{
			RelationUnits: []params.RemoteRelationUnit{{
				RelationToken: remoteRelationToken, Unit: "u", Macaroons: macaroon.Slice{mac}}}})
		c.Assert(result, gc.FitsTypeOf, &params.SettingsResults{})
		*(result.(*params.SettingsResults)) = params.SettingsResults{
			Results: []params.SettingsResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClient(apiCaller)
	result, err := client.RelationUnitSettings([]params.RemoteRelationUnit{
		{RelationToken: remoteRelationToken, Unit: "u", Macaroons: macaroon.Slice{mac}}})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *CrossModelRelationsSuite) TestIngressNetworkChange(c *gc.C) {
	mac, err := macaroon.New(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "PublishIngressNetworkChanges")
		c.Check(arg, gc.DeepEquals, params.IngressNetworksChanges{
			Changes: []params.IngressNetworksChangeEvent{{
				Networks: []string{"1.2.3.4/32"}, Macaroons: macaroon.Slice{mac}}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClient(apiCaller)
	err = client.PublishIngressNetworkChange(params.IngressNetworksChangeEvent{
		Networks: []string{"1.2.3.4/32"}, Macaroons: macaroon.Slice{mac}})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}

func (s *CrossModelRelationsSuite) TestWatchEgressAddressesForRelation(c *gc.C) {
	var callCount int
	remoteRelationToken := "token"
	mac, err := macaroon.New(nil, "", "")
	relation := params.RemoteEntityArg{
		Token: remoteRelationToken, Macaroons: macaroon.Slice{mac}}
	c.Check(err, jc.ErrorIsNil)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchEgressAddressesForRelations")
		c.Check(arg, gc.DeepEquals, params.RemoteEntityArgs{
			Args: []params.RemoteEntityArg{relation}})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClient(apiCaller)
	_, err = client.WatchEgressAddressesForRelation(relation)
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 1)
}
