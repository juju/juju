// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/crossmodelrelations"
	apitesting "github.com/juju/juju/api/testing"
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
	s.BaseSuite.SetUpTest(c)
}

func (s *CrossModelRelationsSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	c.Assert(client, gc.NotNil)
}

type mockDischargeAcquirer struct {
	base.MacaroonDischarger
}

func (m *mockDischargeAcquirer) DischargeAll(ctx context.Context, b *bakery.Macaroon) (macaroon.Slice, error) {
	if !bytes.Equal(b.M().Caveats()[0].Id, []byte("third party caveat")) {
		return nil, errors.New("permission denied")
	}
	mac, err := apitesting.NewMacaroon("discharge mac")
	if err != nil {
		return nil, err
	}
	return macaroon.Slice{mac}, nil
}

func (s *CrossModelRelationsSuite) newDischargeMacaroon(c *gc.C) *macaroon.Macaroon {
	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	err = mac.AddThirdPartyCaveat(nil, []byte("third party caveat"), "third party location")
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
	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "PublishRelationChanges")
		c.Check(arg, gc.DeepEquals, params.RemoteRelationsChanges{
			Changes: []params.RemoteRelationChangeEvent{{
				RelationToken: "token",
				DepartedUnits: []int{1}, Macaroons: macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			}},
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
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := apitesting.NewMacaroon("different")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	err = client.PublishRelationChange(params.RemoteRelationChangeEvent{
		RelationToken: "token",
		DepartedUnits: []int{1},
		Macaroons:     macaroon.Slice{different},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestPublishRelationChangeDischargeRequired(c *gc.C) {
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
	err := client.PublishRelationChange(params.RemoteRelationChangeEvent{
		RelationToken: "token",
		DepartedUnits: []int{1},
	})
	c.Check(callCount, gc.Equals, 2)
	c.Check(err, jc.ErrorIsNil)
	c.Check(dischargeMac, gc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), jc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	apitesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestRegisterRemoteRelations(c *gc.C) {
	var callCount int
	mac, err := apitesting.NewMacaroon("id")
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
				Macaroons:     macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			}}})
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
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := apitesting.NewMacaroon("different")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	result, err = client.RegisterRemoteRelations(params.RegisterRemoteRelationArg{
		RelationToken: "token",
		OfferUUID:     "offer-uuid",
		Macaroons:     macaroon.Slice{different},
		BakeryVersion: bakery.LatestVersion,
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
	result, err := client.RegisterRemoteRelations(params.RegisterRemoteRelationArg{
		RelationToken: "token",
		OfferUUID:     "offer-uuid"})
	c.Check(err, jc.ErrorIsNil)
	c.Check(callCount, gc.Equals, 2)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Error, gc.IsNil)
	c.Assert(dischargeMac, gc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), jc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	apitesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestWatchRelationChanges(c *gc.C) {
	remoteRelationToken := "token"
	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	var callCount int
	apiCaller := testing.BestVersionCaller{
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "CrossModelRelations")
			c.Check(version, gc.Equals, 2)
			c.Check(id, gc.Equals, "")
			c.Check(arg, jc.DeepEquals, params.RemoteEntityArgs{Args: []params.RemoteEntityArg{{
				Token: remoteRelationToken, Macaroons: macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			}}})
			c.Check(request, gc.Equals, "WatchRelationChanges")
			c.Assert(result, gc.FitsTypeOf, &params.RemoteRelationWatchResults{})
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
		remoteRelationToken,
		"app-token",
		macaroon.Slice{mac},
	)
	c.Check(err, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := apitesting.NewMacaroon("different")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	_, err = client.WatchRelationChanges(
		remoteRelationToken,
		"app-token",
		macaroon.Slice{different},
	)
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestWatchRelationChangesDischargeRequired(c *gc.C) {
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
	_, err := client.WatchRelationChanges("token", "app-token", nil)
	c.Check(callCount, gc.Equals, 2)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(dischargeMac, gc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), jc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	apitesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestWatchRelationChangesV1Fallback(c *gc.C) {
	remoteRelationToken := "token"
	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	var calls jujutesting.Stub
	apiCaller := testing.BestVersionCaller{
		BestVersion: 1,
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			fullRequest := fmt.Sprintf("%s.%s", objType, request)
			calls.AddCall(fullRequest)
			c.Check(version, gc.Equals, 1)
			switch fullRequest {
			case "CrossModelRelations.WatchRelationUnits":
				c.Check(id, gc.Equals, "")
				c.Check(arg, jc.DeepEquals, params.RemoteEntityArgs{Args: []params.RemoteEntityArg{{
					Token: remoteRelationToken, Macaroons: macaroon.Slice{mac},
					BakeryVersion: bakery.LatestVersion,
				}}})
				c.Assert(result, gc.FitsTypeOf, &params.RelationUnitsWatchResults{})
				*(result.(*params.RelationUnitsWatchResults)) = params.RelationUnitsWatchResults{
					Results: []params.RelationUnitsWatchResult{{
						RelationUnitsWatcherId: "123",
						Changes: params.RelationUnitsChange{
							Changed: map[string]params.UnitSettings{
								"app/1": {456},
							},
						},
					}},
				}

			case "CrossModelRelations.RelationUnitSettings":
				c.Check(id, gc.Equals, "")
				c.Check(arg, jc.DeepEquals, params.RemoteRelationUnits{
					RelationUnits: []params.RemoteRelationUnit{{
						RelationToken: remoteRelationToken,
						Macaroons:     macaroon.Slice{mac},
						BakeryVersion: bakery.LatestVersion,
						Unit:          "unit-app-1",
					}}})
				c.Assert(result, gc.FitsTypeOf, &params.SettingsResults{})
				*(result.(*params.SettingsResults)) = params.SettingsResults{
					Results: []params.SettingsResult{{
						Settings: map[string]string{
							"cartel-madras": "goonda",
						},
					}},
				}

			case "RelationUnitsWatcher.Next":
				c.Check(id, gc.Equals, "123")
				c.Check(arg, gc.Equals, nil)

				// The client-side watcher handling for result types
				// means that we get something here that is just
				// interface{}, not an interface{} containing
				// something that the gc.FitsTypeOf checker can see. I
				// don't totally understand this, but writing it out
				// using the Lovecraftian machinations of
				// json.Unmarshal works.
				data, err := json.Marshal(params.RelationUnitsWatchResult{
					Error: &params.Error{Message: "UHOH"},
				})
				c.Check(err, jc.ErrorIsNil)
				err = json.Unmarshal(data, result)
				c.Check(err, jc.ErrorIsNil)

			case "RelationUnitsWatcher.Stop":
				c.Check(id, gc.Equals, "123")
				c.Check(arg, gc.Equals, nil)

			default:
				c.Fatalf("bad objtype %q", objType)
			}
			return nil
		}),
	}
	client := crossmodelrelations.NewClientWithCache(apiCaller, s.cache)
	w, err := client.WatchRelationChanges(
		remoteRelationToken,
		"app-token",
		macaroon.Slice{mac},
	)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case change, ok := <-w.Changes():
		c.Assert(ok, gc.Equals, true)
		c.Assert(change, gc.DeepEquals, params.RemoteRelationChangeEvent{
			RelationToken:    remoteRelationToken,
			ApplicationToken: "app-token",
			ChangedUnits: []params.RemoteRelationUnitChange{{
				UnitId: 1,
				Settings: map[string]interface{}{
					"cartel-madras": "goonda",
				},
			}},
		})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	err = workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "UHOH")

	// The calls occasionally come out of order - the watcher's
	// commonLoop manages to call Next before its main loop has called
	// RelationUnitSettings, so instead of seeing [WatchRelationUnits,
	// RelationUnitSettings, Next, Next, Stop] we see
	// [WatchRelationUnits, Next, RelationUnitSettings, Next,
	// Stop]. Just check that all 5 happen at some point for a more
	// reliable test.
	callNames := make([]string, len(calls.Calls()))
	for i, call := range calls.Calls() {
		callNames[i] = call.FuncName
	}
	c.Assert(callNames, jc.SameContents, []string{
		"CrossModelRelations.WatchRelationUnits",
		"CrossModelRelations.RelationUnitSettings",
		"RelationUnitsWatcher.Next",
		"RelationUnitsWatcher.Next",
		"RelationUnitsWatcher.Stop",
	})
}

func (s *CrossModelRelationsSuite) TestWatchRelationStatus(c *gc.C) {
	remoteRelationToken := "token"
	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(arg, jc.DeepEquals, params.RemoteEntityArgs{Args: []params.RemoteEntityArg{{
			Token: remoteRelationToken, Macaroons: macaroon.Slice{mac},
			BakeryVersion: bakery.LatestVersion,
		}}})
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
		Token:         remoteRelationToken,
		Macaroons:     macaroon.Slice{mac},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := apitesting.NewMacaroon("different")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	_, err = client.WatchRelationSuspendedStatus(params.RemoteEntityArg{
		Token:         remoteRelationToken,
		Macaroons:     macaroon.Slice{different},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestWatchRelationStatusDischargeRequired(c *gc.C) {
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
	_, err := client.WatchRelationSuspendedStatus(params.RemoteEntityArg{Token: "token"})
	c.Check(callCount, gc.Equals, 2)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(dischargeMac, gc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), jc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	apitesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestPublishIngressNetworkChange(c *gc.C) {
	mac, err := apitesting.NewMacaroon("id")
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
				Networks:      []string{"1.2.3.4/32"}, Macaroons: macaroon.Slice{mac},
				BakeryVersion: bakery.LatestVersion,
			}},
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
		Networks:      []string{"1.2.3.4/32"}, Macaroons: macaroon.Slice{mac},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := apitesting.NewMacaroon("different")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	err = client.PublishIngressNetworkChange(params.IngressNetworksChangeEvent{
		RelationToken: "token",
		Networks:      []string{"1.2.3.4/32"}, Macaroons: macaroon.Slice{different},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestPublishIngressNetworkChangeDischargeRequired(c *gc.C) {
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
	err := client.PublishIngressNetworkChange(params.IngressNetworksChangeEvent{
		RelationToken: "token",
		Networks:      []string{"1.2.3.4/32"}})
	c.Check(callCount, gc.Equals, 2)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(dischargeMac, gc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), jc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	apitesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestWatchEgressAddressesForRelation(c *gc.C) {
	var callCount int
	remoteRelationToken := "token"
	mac, err := apitesting.NewMacaroon("id")
	relation := params.RemoteEntityArg{
		Token: remoteRelationToken, Macaroons: macaroon.Slice{mac},
		BakeryVersion: bakery.LatestVersion,
	}
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
	different, err := apitesting.NewMacaroon("different")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("token", macaroon.Slice{mac})
	rel2 := relation
	rel2.Macaroons = macaroon.Slice{different}
	rel2.BakeryVersion = bakery.LatestVersion
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
	_, err := client.WatchEgressAddressesForRelation(params.RemoteEntityArg{Token: "token"})
	c.Check(callCount, gc.Equals, 2)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(dischargeMac, gc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), jc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("token")
	c.Assert(ok, jc.IsTrue)
	apitesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}

func (s *CrossModelRelationsSuite) TestWatchOfferStatus(c *gc.C) {
	offerUUID := "offer-uuid"
	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	var callCount int
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelRelations")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(arg, jc.DeepEquals, params.OfferArgs{Args: []params.OfferArg{{
			OfferUUID: offerUUID, Macaroons: macaroon.Slice{mac},
			BakeryVersion: bakery.LatestVersion,
		}}})
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
		OfferUUID:     offerUUID,
		Macaroons:     macaroon.Slice{mac},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	// Call again with a different macaroon but the first one will be
	// cached and override the passed in macaroon.
	different, err := apitesting.NewMacaroon("different")
	c.Assert(err, jc.ErrorIsNil)
	s.cache.Upsert("offer-uuid", macaroon.Slice{mac})
	_, err = client.WatchOfferStatus(params.OfferArg{
		OfferUUID:     offerUUID,
		Macaroons:     macaroon.Slice{different},
		BakeryVersion: bakery.LatestVersion,
	})
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(callCount, gc.Equals, 2)
}

func (s *CrossModelRelationsSuite) TestWatchOfferStatusDischargeRequired(c *gc.C) {
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
	_, err := client.WatchOfferStatus(params.OfferArg{OfferUUID: "offer-uuid"})
	c.Check(callCount, gc.Equals, 2)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(dischargeMac, gc.HasLen, 1)
	c.Assert(dischargeMac[0].Id(), jc.DeepEquals, []byte("discharge mac"))
	// Macaroon has been cached.
	ms, ok := s.cache.Get("offer-uuid")
	c.Assert(ok, jc.IsTrue)
	apitesting.MacaroonEquals(c, ms[0], dischargeMac[0])
}
