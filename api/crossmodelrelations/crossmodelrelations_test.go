// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	"encoding/json"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
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

	cache *crossmodelrelations.MacaroonCache
}

func (s *CrossModelRelationsSuite) SetUpTest(c *gc.C) {
	s.cache = crossmodelrelations.NewMacaroonCache(clock.WallClock)
}

func (s *CrossModelRelationsSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	c.Assert(client, gc.NotNil)
}

type mockDischargeAcquirer struct{}

func (m *mockDischargeAcquirer) AcquireDischarge(firstPartyLocation string, cav macaroon.Caveat) (*macaroon.Macaroon, error) {
	if cav.Id != "third party caveat" {
		return nil, errors.New("permission denied")
	}
	return macaroon.New(nil, "discharge mac", "")
}

func (s *CrossModelRelationsSuite) newDischargeMacaroon(c *gc.C) *macaroon.Macaroon {
	mac, err := macaroon.New(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)
	err = mac.AddThirdPartyCaveat(nil, "third party caveat", "third party location")
	c.Assert(err, jc.ErrorIsNil)
	return mac
}

func (s *CrossModelRelationsSuite) fillResponse(c *gc.C, resp interface{}, value interface{}) {
	b, err := json.Marshal(value)
	c.Assert(err, jc.ErrorIsNil)
	err = json.Unmarshal(b, resp)
	c.Assert(err, jc.ErrorIsNil)
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
				RelationToken: "token",
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
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	err = client.PublishRelationChange(params.RemoteRelationChangeEvent{
		RelationToken: "token",
		DepartedUnits: []int{1},
		Macaroons:     macaroon.Slice{mac},
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := macaroon.New(nil, "different", "")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	err = client.PublishRelationChange(params.RemoteRelationChangeEvent{
		RelationToken: "token",
		DepartedUnits: []int{1},
		Macaroons:     macaroon.Slice{different},
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestPublishRelationChangeDischargeRequired(c *gc.C) {
	var (
		callCount    int
		mac          *macaroon.Macaroon
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		if callCount == 0 {
			mac = s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: &params.ErrorInfo{
					Macaroon: mac,
				},
			}
		}
		argParam := arg.(params.RemoteRelationsChanges)
		dischargeMac = argParam.Changes[0].Macaroons
		resp := params.ErrorResults{
			Results: []params.ErrorResult{{Error: resultErr}},
		}
		s.fillResponse(c, result, resp)
		callCount++
		return nil
	})
	acquirer := &mockDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelrelations.NewClientWithCache(callerWithBakery, s.cache)
	err := client.PublishRelationChange(params.RemoteRelationChangeEvent{
		RelationToken: "token",
		DepartedUnits: []int{1},
	})
	c.Check(callCount, gc.Equals, 2)
	c.Check(err, jc.ErrorIsNil)
	c.Check(dischargeMac, gc.HasLen, 2)
	c.Assert(dischargeMac[0], jc.DeepEquals, mac)
	c.Assert(dischargeMac[1].Id(), gc.Equals, "discharge mac")
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms[0], jc.DeepEquals, mac)
	c.Assert(ms[1].Id(), gc.Equals, "discharge mac")
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
			Relations: []params.RegisterRemoteRelationArg{{
				RelationToken: "token",
				OfferUUID:     "offer-uuid",
				Macaroons:     macaroon.Slice{mac}}}})
		c.Assert(result, gc.FitsTypeOf, &params.RegisterRemoteRelationResults{})
		*(result.(*params.RegisterRemoteRelationResults)) = params.RegisterRemoteRelationResults{
			Results: []params.RegisterRemoteRelationResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	result, err := client.RegisterRemoteRelations(params.RegisterRemoteRelationArg{
		RelationToken: "token",
		OfferUUID:     "offer-uuid",
		Macaroons:     macaroon.Slice{mac},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := macaroon.New(nil, "different", "")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	result, err = client.RegisterRemoteRelations(params.RegisterRemoteRelationArg{
		RelationToken: "token",
		OfferUUID:     "offer-uuid",
		Macaroons:     macaroon.Slice{different},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
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
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	_, err := client.RegisterRemoteRelations(params.RegisterRemoteRelationArg{})
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *CrossModelRelationsSuite) TestRegisterRemoteRelationDischargeRequired(c *gc.C) {
	var (
		callCount    int
		mac          *macaroon.Macaroon
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		if callCount == 0 {
			mac = s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: &params.ErrorInfo{
					Macaroon: mac,
				},
			}
		}
		argParam := arg.(params.RegisterRemoteRelationArgs)
		dischargeMac = argParam.Relations[0].Macaroons
		resp := params.RegisterRemoteRelationResults{
			Results: []params.RegisterRemoteRelationResult{{Error: resultErr}},
		}
		s.fillResponse(c, result, resp)
		callCount++
		return nil
	})
	acquirer := &mockDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelrelations.NewClientWithCache(callerWithBakery, s.cache)
	result, err := client.RegisterRemoteRelations(params.RegisterRemoteRelationArg{
		RelationToken: "token",
		OfferUUID:     "offer-uuid"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 2)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.IsNil)
	c.Check(dischargeMac, gc.HasLen, 2)
	c.Assert(dischargeMac[0], jc.DeepEquals, mac)
	c.Assert(dischargeMac[1].Id(), gc.Equals, "discharge mac")
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms[0], jc.DeepEquals, mac)
	c.Assert(ms[1].Id(), gc.Equals, "discharge mac")
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
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	_, err = client.WatchRelationUnits(params.RemoteEntityArg{
		Token:     remoteRelationToken,
		Macaroons: macaroon.Slice{mac},
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := macaroon.New(nil, "different", "")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	_, err = client.WatchRelationUnits(params.RemoteEntityArg{
		Token:     remoteRelationToken,
		Macaroons: macaroon.Slice{different},
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestWatchRelationUnitsDischargeRequired(c *gc.C) {
	var (
		callCount    int
		mac          *macaroon.Macaroon
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		switch callCount {
		case 2, 3: //Watcher Next, Stop
			return nil
		case 0:
			mac = s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: &params.ErrorInfo{
					Macaroon: mac,
				},
			}
		case 1:
			argParam := arg.(params.RemoteEntityArgs)
			dischargeMac = argParam.Args[0].Macaroons
		}
		resp := params.RelationUnitsWatchResults{
			Results: []params.RelationUnitsWatchResult{{Error: resultErr}},
		}
		s.fillResponse(c, result, resp)
		callCount++
		return nil
	})
	acquirer := &mockDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelrelations.NewClientWithCache(callerWithBakery, s.cache)
	_, err := client.WatchRelationUnits(params.RemoteEntityArg{Token: "token"})
	c.Check(callCount, gc.Equals, 2)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(dischargeMac, gc.HasLen, 2)
	c.Assert(dischargeMac[0], jc.DeepEquals, mac)
	c.Assert(dischargeMac[1].Id(), gc.Equals, "discharge mac")
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms[0], jc.DeepEquals, mac)
	c.Assert(ms[1].Id(), gc.Equals, "discharge mac")
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
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	result, err := client.RelationUnitSettings([]params.RemoteRelationUnit{
		{RelationToken: remoteRelationToken, Unit: "u", Macaroons: macaroon.Slice{mac}}})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := macaroon.New(nil, "different", "")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	result, err = client.RelationUnitSettings([]params.RemoteRelationUnit{
		{RelationToken: remoteRelationToken, Unit: "u", Macaroons: macaroon.Slice{different}}})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestRelationUnitSettingsDischargeRequired(c *gc.C) {
	var (
		callCount    int
		mac          *macaroon.Macaroon
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		if callCount == 0 {
			mac = s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: &params.ErrorInfo{
					Macaroon: mac,
				},
			}
		}
		argParam := arg.(params.RemoteRelationUnits)
		dischargeMac = argParam.RelationUnits[0].Macaroons
		resp := params.SettingsResults{
			Results: []params.SettingsResult{{Error: resultErr}},
		}
		s.fillResponse(c, result, resp)
		callCount++
		return nil
	})
	acquirer := &mockDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelrelations.NewClientWithCache(callerWithBakery, s.cache)
	result, err := client.RelationUnitSettings([]params.RemoteRelationUnit{
		{RelationToken: "token", Unit: "u"}})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 2)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.IsNil)
	c.Check(dischargeMac, gc.HasLen, 2)
	c.Assert(dischargeMac[0], jc.DeepEquals, mac)
	c.Assert(dischargeMac[1].Id(), gc.Equals, "discharge mac")
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms[0], jc.DeepEquals, mac)
	c.Assert(ms[1].Id(), gc.Equals, "discharge mac")
}

func (s *CrossModelRelationsSuite) TestWatchRelationStatus(c *gc.C) {
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
		c.Check(request, gc.Equals, "WatchRelationsSuspendedStatus")
		c.Assert(result, gc.FitsTypeOf, &params.RelationStatusWatchResults{})
		*(result.(*params.RelationStatusWatchResults)) = params.RelationStatusWatchResults{
			Results: []params.RelationLifeSuspendedStatusWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	_, err = client.WatchRelationSuspendedStatus(params.RemoteEntityArg{
		Token:     remoteRelationToken,
		Macaroons: macaroon.Slice{mac},
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := macaroon.New(nil, "different", "")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	_, err = client.WatchRelationSuspendedStatus(params.RemoteEntityArg{
		Token:     remoteRelationToken,
		Macaroons: macaroon.Slice{different},
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestWatchRelationStatusDischargeRequired(c *gc.C) {
	var (
		callCount    int
		mac          *macaroon.Macaroon
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		switch callCount {
		case 2, 3: //Watcher Next, Stop
			return nil
		case 0:
			mac = s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: &params.ErrorInfo{
					Macaroon: mac,
				},
			}
		case 1:
			argParam := arg.(params.RemoteEntityArgs)
			dischargeMac = argParam.Args[0].Macaroons
		}
		resp := params.RelationStatusWatchResults{
			Results: []params.RelationLifeSuspendedStatusWatchResult{{Error: resultErr}},
		}
		s.fillResponse(c, result, resp)
		callCount++
		return nil
	})
	acquirer := &mockDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelrelations.NewClientWithCache(callerWithBakery, s.cache)
	_, err := client.WatchRelationSuspendedStatus(params.RemoteEntityArg{Token: "token"})
	c.Check(callCount, gc.Equals, 2)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(dischargeMac, gc.HasLen, 2)
	c.Assert(dischargeMac[0], jc.DeepEquals, mac)
	c.Assert(dischargeMac[1].Id(), gc.Equals, "discharge mac")
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms[0], jc.DeepEquals, mac)
	c.Assert(ms[1].Id(), gc.Equals, "discharge mac")
}

func (s *CrossModelRelationsSuite) TestPublishIngressNetworkChange(c *gc.C) {
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
				RelationToken: "token",
				Networks:      []string{"1.2.3.4/32"}, Macaroons: macaroon.Slice{mac}}},
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
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	err = client.PublishIngressNetworkChange(params.IngressNetworksChangeEvent{
		RelationToken: "token",
		Networks:      []string{"1.2.3.4/32"}, Macaroons: macaroon.Slice{mac}})
	c.Check(err, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := macaroon.New(nil, "different", "")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	err = client.PublishIngressNetworkChange(params.IngressNetworksChangeEvent{
		RelationToken: "token",
		Networks:      []string{"1.2.3.4/32"}, Macaroons: macaroon.Slice{different}})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestPublishIngressNetworkChangeDischargeRequired(c *gc.C) {
	var (
		callCount    int
		mac          *macaroon.Macaroon
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		if callCount == 0 {
			mac = s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: &params.ErrorInfo{
					Macaroon: mac,
				},
			}
		}
		argParam := arg.(params.IngressNetworksChanges)
		dischargeMac = argParam.Changes[0].Macaroons
		resp := params.ErrorResults{
			Results: []params.ErrorResult{{Error: resultErr}},
		}
		s.fillResponse(c, result, resp)
		callCount++
		return nil
	})
	acquirer := &mockDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelrelations.NewClientWithCache(callerWithBakery, s.cache)
	err := client.PublishIngressNetworkChange(params.IngressNetworksChangeEvent{
		RelationToken: "token",
		Networks:      []string{"1.2.3.4/32"}})
	c.Check(callCount, gc.Equals, 2)
	c.Check(err, jc.ErrorIsNil)
	c.Check(dischargeMac, gc.HasLen, 2)
	c.Assert(dischargeMac[0], jc.DeepEquals, mac)
	c.Assert(dischargeMac[1].Id(), gc.Equals, "discharge mac")
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms[0], jc.DeepEquals, mac)
	c.Assert(ms[1].Id(), gc.Equals, "discharge mac")
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
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	_, err = client.WatchEgressAddressesForRelation(relation)
	c.Check(err, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := macaroon.New(nil, "different", "")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	rel2 := relation
	rel2.Macaroons = macaroon.Slice{different}
	_, err = client.WatchEgressAddressesForRelation(rel2)
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestWatchEgressAddressesForRelationDischargeRequired(c *gc.C) {
	var (
		callCount    int
		mac          *macaroon.Macaroon
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		switch callCount {
		case 2, 3: //Watcher Next, Stop
			return nil
		case 0:
			mac = s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: &params.ErrorInfo{
					Macaroon: mac,
				},
			}
		case 1:
			argParam := arg.(params.RemoteEntityArgs)
			dischargeMac = argParam.Args[0].Macaroons
		}
		resp := params.StringsWatchResults{
			Results: []params.StringsWatchResult{{Error: resultErr}},
		}
		s.fillResponse(c, result, resp)
		callCount++
		return nil
	})
	acquirer := &mockDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelrelations.NewClientWithCache(callerWithBakery, s.cache)
	_, err := client.WatchEgressAddressesForRelation(params.RemoteEntityArg{Token: "token"})
	c.Check(callCount, gc.Equals, 2)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(dischargeMac, gc.HasLen, 2)
	c.Assert(dischargeMac[0], jc.DeepEquals, mac)
	c.Assert(dischargeMac[1].Id(), gc.Equals, "discharge mac")
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms[0], jc.DeepEquals, mac)
	c.Assert(ms[1].Id(), gc.Equals, "discharge mac")
}

func (s *CrossModelRelationsSuite) TestWatchOfferStatus(c *gc.C) {
	offerUUID := "offer-uuid"
	mac, err := macaroon.New(nil, "", "")
	c.Assert(err, jc.ErrorIsNil)
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(arg, jc.DeepEquals, params.OfferArgs{Args: []params.OfferArg{{
			OfferUUID: offerUUID, Macaroons: macaroon.Slice{mac}}}})
		c.Check(request, gc.Equals, "WatchOfferStatus")
		c.Assert(result, gc.FitsTypeOf, &params.OfferStatusWatchResults{})
		*(result.(*params.OfferStatusWatchResults)) = params.OfferStatusWatchResults{
			Results: []params.OfferStatusWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	_, err = client.WatchOfferStatus(params.OfferArg{
		OfferUUID: offerUUID,
		Macaroons: macaroon.Slice{mac},
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := macaroon.New(nil, "different", "")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("offer-uuid", macaroon.Slice{mac})
	_, err = client.WatchOfferStatus(params.OfferArg{
		OfferUUID: offerUUID,
		Macaroons: macaroon.Slice{different},
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestWatchOfferStatusDischargeRequired(c *gc.C) {
	var (
		callCount    int
		mac          *macaroon.Macaroon
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		switch callCount {
		case 2, 3: //Watcher Next, Stop
			return nil
		case 0:
			mac = s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: &params.ErrorInfo{
					Macaroon: mac,
				},
			}
		case 1:
			argParam := arg.(params.OfferArgs)
			dischargeMac = argParam.Args[0].Macaroons
		}
		resp := params.OfferStatusWatchResults{
			Results: []params.OfferStatusWatchResult{{Error: resultErr}},
		}
		s.fillResponse(c, result, resp)
		callCount++
		return nil
	})
	acquirer := &mockDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelrelations.NewClientWithCache(callerWithBakery, s.cache)
	_, err := client.WatchOfferStatus(params.OfferArg{OfferUUID: "offer-uuid"})
	c.Check(callCount, gc.Equals, 2)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(dischargeMac, gc.HasLen, 2)
	c.Assert(dischargeMac[0], jc.DeepEquals, mac)
	c.Assert(dischargeMac[1].Id(), gc.Equals, "discharge mac")
	// Macaroon has been cached.
	ms, ok := s.cache.Get("offer-uuid")
	c.Assert(ok, jc.IsTrue)
	c.Assert(ms[0], jc.DeepEquals, mac)
	c.Assert(ms[1].Id(), gc.Equals, "discharge mac")
}
