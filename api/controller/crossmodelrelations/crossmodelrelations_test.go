// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/crossmodelrelations"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&CrossModelRelationsSuite{})

type CrossModelRelationsSuite struct {
	coretesting.BaseSuite

	cache *crossmodelrelations.MacaroonCache
}

func (s *CrossModelRelationsSuite) SetUpTest(c *tc.C) {
	s.cache = crossmodelrelations.NewMacaroonCache(clock.WallClock)
	s.BaseSuite.SetUpTest(c)
}

func (s *CrossModelRelationsSuite) TestNewClient(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	c.Assert(client, tc.NotNil)
}

type mockDischargeAcquirer struct {
	base.MacaroonDischarger
}

func (m *mockDischargeAcquirer) DischargeAll(ctx context.Context, b *bakery.Macaroon) (macaroon.Slice, error) {
	if !bytes.Equal(b.M().Caveats()[0].Id, []byte("third party caveat")) {
		return nil, errors.New("permission denied")
	}
	mac, err := jujutesting.NewMacaroon("discharge mac")
	if err != nil {
		return nil, err
	}
	return macaroon.Slice{mac}, nil
}

func (s *CrossModelRelationsSuite) newDischargeMacaroon(c *tc.C) *macaroon.Macaroon {
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	err = mac.AddThirdPartyCaveat(nil, []byte("third party caveat"), "third party location")
	c.Assert(err, tc.ErrorIsNil)
	return mac
}

func (s *CrossModelRelationsSuite) fillResponse(c *tc.C, resp interface{}, value interface{}) {
	b, err := json.Marshal(value)
	c.Assert(err, tc.ErrorIsNil)
	err = json.Unmarshal(b, resp)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CrossModelRelationsSuite) TestPublishRelationChange(c *tc.C) {
	var callCount int
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CrossModelRelations")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "PublishRelationChanges")
		c.Check(arg, tc.DeepEquals, params.RemoteRelationsChanges{
			Changes: []params.RemoteRelationChangeEvent{{
				RelationToken: "token",
				DepartedUnits: []int{1}, Macaroons: macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	err = client.PublishRelationChange(c.Context(), params.RemoteRelationChangeEvent{
		RelationToken: "token",
		DepartedUnits: []int{1},
		Macaroons:     macaroon.Slice{mac},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, tc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := jujutesting.NewMacaroon("different")
	c.Assert(err, tc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	err = client.PublishRelationChange(c.Context(), params.RemoteRelationChangeEvent{
		RelationToken: "token",
		DepartedUnits: []int{1},
		Macaroons:     macaroon.Slice{different},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestPublishRelationChangeDischargeRequired(c *tc.C) {
	var (
		callCount    int
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		if callCount == 0 {
			mac := s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: params.DischargeRequiredErrorInfo{
					Macaroon: mac,
				}.AsMap(),
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
	err := client.PublishRelationChange(c.Context(), params.RemoteRelationChangeEvent{
		RelationToken: "token",
		DepartedUnits: []int{1},
	})
	c.Check(callCount, tc.Equals, 2)
	c.Check(err, tc.ErrorIsNil)
	c.Check(dischargeMac, tc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), tc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, tc.IsTrue)
	jujutesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestRegisterRemoteRelations(c *tc.C) {
	var callCount int
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CrossModelRelations")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "RegisterRemoteRelations")
		c.Check(arg, tc.DeepEquals, params.RegisterRemoteRelationArgs{
			Relations: []params.RegisterRemoteRelationArg{{
				RelationToken: "token",
				OfferUUID:     "offer-uuid",
				Macaroons:     macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			}}})
		c.Assert(result, tc.FitsTypeOf, &params.RegisterRemoteRelationResults{})
		*(result.(*params.RegisterRemoteRelationResults)) = params.RegisterRemoteRelationResults{
			Results: []params.RegisterRemoteRelationResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	result, err := client.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArg{
		RelationToken: "token",
		OfferUUID:     "offer-uuid",
		Macaroons:     macaroon.Slice{mac},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].Error, tc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := jujutesting.NewMacaroon("different")
	c.Assert(err, tc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	result, err = client.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArg{
		RelationToken: "token",
		OfferUUID:     "offer-uuid",
		Macaroons:     macaroon.Slice{different},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].Error, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestRegisterRemoteRelationCount(c *tc.C) {
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
	_, err := client.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArg{})
	c.Check(err, tc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *CrossModelRelationsSuite) TestRegisterRemoteRelationDischargeRequired(c *tc.C) {
	var (
		callCount    int
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		if callCount == 0 {
			mac := s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: params.DischargeRequiredErrorInfo{
					Macaroon: mac,
				}.AsMap(),
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
	result, err := client.RegisterRemoteRelations(c.Context(), params.RegisterRemoteRelationArg{
		RelationToken: "token",
		OfferUUID:     "offer-uuid"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(callCount, tc.Equals, 2)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].Error, tc.IsNil)
	c.Assert(dischargeMac, tc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), tc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, tc.IsTrue)
	jujutesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestWatchRelationChanges(c *tc.C) {
	remoteRelationToken := "token"
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	var callCount int
	apiCaller := testing.BestVersionCaller{
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "CrossModelRelations")
			c.Check(version, tc.Equals, 2)
			c.Check(id, tc.Equals, "")
			c.Check(arg, tc.DeepEquals, params.RemoteEntityArgs{Args: []params.RemoteEntityArg{{
				Token: remoteRelationToken, Macaroons: macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			}}})
			c.Check(request, tc.Equals, "WatchRelationChanges")
			c.Assert(result, tc.FitsTypeOf, &params.RemoteRelationWatchResults{})
			*(result.(*params.RemoteRelationWatchResults)) = params.RemoteRelationWatchResults{
				Results: []params.RemoteRelationWatchResult{{
					Error: &params.Error{Message: "FAIL"},
				}},
			}
			callCount++
			return nil
		}),
		BestVersion: 2,
	}
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	_, err = client.WatchRelationChanges(
		c.Context(),
		remoteRelationToken,
		"app-token",
		macaroon.Slice{mac},
	)
	c.Check(err, tc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := jujutesting.NewMacaroon("different")
	c.Assert(err, tc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	_, err = client.WatchRelationChanges(
		c.Context(),
		remoteRelationToken,
		"app-token",
		macaroon.Slice{different},
	)
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestWatchRelationChangesDischargeRequired(c *tc.C) {
	var (
		callCount    int
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.BestVersionCaller{
		BestVersion: 2,
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			var resultErr *params.Error
			c.Logf("called")
			switch callCount {
			case 2, 3: //Watcher Next, Stop
				return nil
			case 0:
				mac := s.newDischargeMacaroon(c)
				resultErr = &params.Error{
					Code: params.CodeDischargeRequired,
					Info: params.DischargeRequiredErrorInfo{
						Macaroon: mac,
					}.AsMap(),
				}
			case 1:
				argParam := arg.(params.RemoteEntityArgs)
				dischargeMac = argParam.Args[0].Macaroons
			}
			resp := params.RemoteRelationWatchResults{
				Results: []params.RemoteRelationWatchResult{{Error: resultErr}},
			}
			s.fillResponse(c, result, resp)
			callCount++
			return nil
		}),
	}
	acquirer := &mockDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelrelations.NewClientWithCache(callerWithBakery, s.cache)
	_, err := client.WatchRelationChanges(c.Context(), "token", "app-token", nil)
	c.Check(callCount, tc.Equals, 2)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(dischargeMac, tc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), tc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, tc.IsTrue)
	jujutesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestWatchRelationStatus(c *tc.C) {
	remoteRelationToken := "token"
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CrossModelRelations")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(arg, tc.DeepEquals, params.RemoteEntityArgs{Args: []params.RemoteEntityArg{{
			Token: remoteRelationToken, Macaroons: macaroon.Slice{mac},
			BakeryVersion: bakery.LatestVersion,
		}}})
		c.Check(request, tc.Equals, "WatchRelationsSuspendedStatus")
		c.Assert(result, tc.FitsTypeOf, &params.RelationStatusWatchResults{})
		*(result.(*params.RelationStatusWatchResults)) = params.RelationStatusWatchResults{
			Results: []params.RelationLifeSuspendedStatusWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	_, err = client.WatchRelationSuspendedStatus(c.Context(), params.RemoteEntityArg{
		Token:         remoteRelationToken,
		Macaroons:     macaroon.Slice{mac},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, tc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := jujutesting.NewMacaroon("different")
	c.Assert(err, tc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	_, err = client.WatchRelationSuspendedStatus(c.Context(), params.RemoteEntityArg{
		Token:         remoteRelationToken,
		Macaroons:     macaroon.Slice{different},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestWatchRelationStatusDischargeRequired(c *tc.C) {
	var (
		callCount    int
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		switch callCount {
		case 2, 3: //Watcher Next, Stop
			return nil
		case 0:
			mac := s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: params.DischargeRequiredErrorInfo{
					Macaroon: mac,
				}.AsMap(),
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
	_, err := client.WatchRelationSuspendedStatus(c.Context(), params.RemoteEntityArg{Token: "token"})
	c.Check(callCount, tc.Equals, 2)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(dischargeMac, tc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), tc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, tc.IsTrue)
	jujutesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestPublishIngressNetworkChange(c *tc.C) {
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CrossModelRelations")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "PublishIngressNetworkChanges")
		c.Check(arg, tc.DeepEquals, params.IngressNetworksChanges{
			Changes: []params.IngressNetworksChangeEvent{{
				RelationToken: "token",
				Networks:      []string{"1.2.3.4/32"}, Macaroons: macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	err = client.PublishIngressNetworkChange(c.Context(), params.IngressNetworksChangeEvent{
		RelationToken: "token",
		Networks:      []string{"1.2.3.4/32"}, Macaroons: macaroon.Slice{mac},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, tc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := jujutesting.NewMacaroon("different")
	c.Assert(err, tc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	err = client.PublishIngressNetworkChange(c.Context(), params.IngressNetworksChangeEvent{
		RelationToken: "token",
		Networks:      []string{"1.2.3.4/32"}, Macaroons: macaroon.Slice{different},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestPublishIngressNetworkChangeDischargeRequired(c *tc.C) {
	var (
		callCount    int
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		if callCount == 0 {
			mac := s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: params.DischargeRequiredErrorInfo{
					Macaroon: mac,
				}.AsMap(),
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
	err := client.PublishIngressNetworkChange(c.Context(), params.IngressNetworksChangeEvent{
		RelationToken: "token",
		Networks:      []string{"1.2.3.4/32"}})
	c.Check(callCount, tc.Equals, 2)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(dischargeMac, tc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), tc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, tc.IsTrue)
	jujutesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestWatchEgressAddressesForRelation(c *tc.C) {
	var callCount int
	remoteRelationToken := "token"
	mac, err := jujutesting.NewMacaroon("id")
	relation := params.RemoteEntityArg{
		Token: remoteRelationToken, Macaroons: macaroon.Slice{mac},
		BakeryVersion: bakery.LatestVersion,
	}
	c.Check(err, tc.ErrorIsNil)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CrossModelRelations")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchEgressAddressesForRelations")
		c.Check(arg, tc.DeepEquals, params.RemoteEntityArgs{
			Args: []params.RemoteEntityArg{relation}})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	_, err = client.WatchEgressAddressesForRelation(c.Context(), relation)
	c.Check(err, tc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := jujutesting.NewMacaroon("different")
	c.Assert(err, tc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	rel2 := relation
	rel2.Macaroons = macaroon.Slice{different}
	rel2.BakeryVersion = bakery.LatestVersion
	_, err = client.WatchEgressAddressesForRelation(c.Context(), rel2)
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestWatchEgressAddressesForRelationDischargeRequired(c *tc.C) {
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
				Info: params.DischargeRequiredErrorInfo{
					Macaroon: mac,
				}.AsMap(),
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
	_, err := client.WatchEgressAddressesForRelation(c.Context(), params.RemoteEntityArg{Token: "token"})
	c.Check(callCount, tc.Equals, 2)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(dischargeMac, tc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), tc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, tc.IsTrue)
	jujutesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestWatchOfferStatus(c *tc.C) {
	offerUUID := "offer-uuid"
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CrossModelRelations")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(arg, tc.DeepEquals, params.OfferArgs{Args: []params.OfferArg{{
			OfferUUID: offerUUID, Macaroons: macaroon.Slice{mac},
			BakeryVersion: bakery.LatestVersion,
		}}})
		c.Check(request, tc.Equals, "WatchOfferStatus")
		c.Assert(result, tc.FitsTypeOf, &params.OfferStatusWatchResults{})
		*(result.(*params.OfferStatusWatchResults)) = params.OfferStatusWatchResults{
			Results: []params.OfferStatusWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	})
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	_, err = client.WatchOfferStatus(c.Context(), params.OfferArg{
		OfferUUID:     offerUUID,
		Macaroons:     macaroon.Slice{mac},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, tc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := jujutesting.NewMacaroon("different")
	c.Assert(err, tc.ErrorIsNil)
	s.cache.Upsert("offer-uuid", macaroon.Slice{mac})
	_, err = client.WatchOfferStatus(c.Context(), params.OfferArg{
		OfferUUID:     offerUUID,
		Macaroons:     macaroon.Slice{different},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestWatchOfferStatusDischargeRequired(c *tc.C) {
	var (
		callCount    int
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		switch callCount {
		case 2, 3: //Watcher Next, Stop
			return nil
		case 0:
			mac := s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: params.DischargeRequiredErrorInfo{
					Macaroon: mac,
				}.AsMap(),
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
	_, err := client.WatchOfferStatus(c.Context(), params.OfferArg{OfferUUID: "offer-uuid"})
	c.Check(callCount, tc.Equals, 2)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(dischargeMac, tc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), tc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("offer-uuid")
	c.Assert(ok, tc.IsTrue)
	jujutesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestWatchConsumedSecretsChanges(c *tc.C) {
	appToken := "app-token"
	relToken := "rel-token"
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	var callCount int
	apiCaller := testing.BestVersionCaller{APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CrossModelRelations")
		c.Check(version, tc.Equals, 3)
		c.Check(id, tc.Equals, "")
		c.Check(arg, tc.DeepEquals, params.WatchRemoteSecretChangesArgs{Args: []params.WatchRemoteSecretChangesArg{{
			ApplicationToken: appToken, RelationToken: relToken, Macaroons: macaroon.Slice{mac},
			BakeryVersion: bakery.LatestVersion,
		}}})
		c.Check(request, tc.Equals, "WatchConsumedSecretsChanges")
		c.Assert(result, tc.FitsTypeOf, &params.SecretRevisionWatchResults{})
		*(result.(*params.SecretRevisionWatchResults)) = params.SecretRevisionWatchResults{
			Results: []params.SecretRevisionWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		callCount++
		return nil
	}), BestVersion: 3}
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	_, err = client.WatchConsumedSecretsChanges(c.Context(), appToken, relToken, mac)
	c.Check(err, tc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := jujutesting.NewMacaroon("different")
	c.Assert(err, tc.ErrorIsNil)
	s.cache.Upsert(relToken, macaroon.Slice{mac})
	_, err = client.WatchConsumedSecretsChanges(c.Context(), appToken, relToken, different)
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(callCount, tc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestWatchConsumedSecretsChangesDischargeRequired(c *tc.C) {
	var (
		callCount    int
		dischargeMac macaroon.Slice
	)
	apiCaller := testing.BestVersionCaller{APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		var resultErr *params.Error
		switch callCount {
		case 2, 3: //Watcher Next, Stop
			return nil
		case 0:
			mac := s.newDischargeMacaroon(c)
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: params.DischargeRequiredErrorInfo{
					Macaroon: mac,
				}.AsMap(),
			}
		case 1:
			argParam := arg.(params.WatchRemoteSecretChangesArgs)
			dischargeMac = argParam.Args[0].Macaroons
		}
		resp := params.SecretRevisionWatchResults{
			Results: []params.SecretRevisionWatchResult{{Error: resultErr}},
		}
		s.fillResponse(c, result, resp)
		callCount++
		return nil
	}), BestVersion: 3}
	acquirer := &mockDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelrelations.NewClientWithCache(callerWithBakery, s.cache)
	_, err := client.WatchConsumedSecretsChanges(c.Context(), "app-token", "rel-token", nil)
	c.Check(callCount, tc.Equals, 2)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(dischargeMac, tc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), tc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("rel-token")
	c.Assert(ok, tc.IsTrue)
	jujutesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}
