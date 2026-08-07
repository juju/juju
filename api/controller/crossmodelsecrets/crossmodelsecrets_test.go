// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets_test

import (
	"context"
	"encoding/json"
	"errors"
	stdtesting "testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/crossmodelsecrets"
	"github.com/juju/juju/core/application"
	relationtesting "github.com/juju/juju/core/relation/testing"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets"
	secretsprovider "github.com/juju/juju/internal/secrets/provider"
	coretesting "github.com/juju/juju/internal/testing"
	jujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

func TestCrossControllerSuite(t *stdtesting.T) {
	tc.Run(t, &CrossControllerSuite{})
}

type CrossControllerSuite struct {
	coretesting.BaseSuite
}

func (s *CrossControllerSuite) TestNewClient(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}

func (s *CrossControllerSuite) TestGetRemoteSecretContentInfo(c *tc.C) {
	uri := coresecrets.NewURI()
	macs := macaroon.Slice{jujujutesting.MustNewMacaroon("test")}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		c.Check(objType, tc.Equals, "CrossModelSecrets")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetSecretContentInfo")
		c.Check(arg, tc.DeepEquals, params.GetRemoteSecretContentArgs{
			Args: []params.GetRemoteSecretContentArg{{
				SourceControllerUUID: coretesting.ControllerTag.Id(),
				ApplicationToken:     "token",
				UnitId:               666,
				Revision:             new(665),
				Macaroons:            macs,
				BakeryVersion:        3,
				URI:                  uri.String(),
				Refresh:              true,
				Peek:                 true,
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{
					ValueRef: &params.SecretValueRef{
						BackendID:  "backend-id",
						RevisionID: "rev-id",
					},
				},
				BackendConfig: &params.SecretBackendConfigResult{
					ControllerUUID: coretesting.ControllerTag.Id(),
					ModelUUID:      coretesting.ModelTag.Id(),
					ModelName:      "fred",
					Draining:       true,
					Config: params.SecretBackendConfig{
						BackendType: "vault",
						Params:      map[string]any{"foo": "bar"},
					},
				},
				LatestRevision: new(666),
			}},
		}
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	content, backend, latestRevision, draining, err := client.GetRemoteSecretContentInfo(c.Context(), uri, 665, true, true, coretesting.ControllerTag.Id(), "token", 666, macs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(latestRevision, tc.Equals, 666)
	c.Assert(draining, tc.IsTrue)
	c.Assert(content, tc.DeepEquals, &secrets.ContentParams{
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		},
	})
	c.Assert(backend, tc.DeepEquals, &secretsprovider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: secretsprovider.BackendConfig{
			BackendType: "vault",
			Config:      map[string]any{"foo": "bar"},
		},
	})
}

func (s *CrossControllerSuite) TestControllerInfoError(c *tc.C) {
	s.PatchValue(&crossmodelsecrets.Clock, testclock.NewDilatedWallClock(time.Millisecond))
	attemptCount := 0
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		attemptCount++
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	content, backend, _, _, err := client.GetRemoteSecretContentInfo(c.Context(), coresecrets.NewURI(), 665, false, false, coretesting.ControllerTag.Id(), "token", 666, nil)
	c.Assert(err, tc.ErrorMatches, "attempt count exceeded: boom")
	c.Assert(content, tc.IsNil)
	c.Assert(backend, tc.IsNil)
	c.Assert(attemptCount, tc.Equals, 3)
}

// mockDischargeAcquirer implements base.MacaroonDischarger for testing
// macaroon discharge flows.
type mockDischargeAcquirer struct {
	base.MacaroonDischarger
}

func (m *mockDischargeAcquirer) DischargeAll(ctx context.Context, b *bakery.Macaroon) (macaroon.Slice, error) {
	mac, err := jujujutesting.NewMacaroon("discharge mac")
	if err != nil {
		return nil, err
	}
	return macaroon.Slice{mac}, nil
}

// testLocator resolves any third party location to a known public key.
type testLocator struct {
	PublicKey bakery.PublicKey
}

func (b testLocator) ThirdPartyInfo(ctx context.Context, loc string) (bakery.ThirdPartyInfo, error) {
	return bakery.ThirdPartyInfo{
		PublicKey: b.PublicKey,
		Version:   bakery.LatestVersion,
	}, nil
}

func fillResponse(c *tc.C, resp any, value any) {
	b, err := json.Marshal(value)
	c.Assert(err, tc.ErrorIsNil)
	err = json.Unmarshal(b, resp)
	c.Assert(err, tc.ErrorIsNil)
}

// TestGetRemoteSecretContentInfoDischargeRequired verifies that when the
// remote controller returns a discharge-required error, the macaroon is
// discharged and the call is retried immediately within the same Func
// invocation — without incurring the retry.Call delay.
func (s *CrossControllerSuite) TestGetRemoteSecretContentInfoDischargeRequired(c *tc.C) {
	uri := coresecrets.NewURI()
	key, err := bakery.GenerateKey()
	c.Assert(err, tc.ErrorIsNil)
	bk := bakery.New(bakery.BakeryParams{
		Key:     key,
		Locator: testLocator{key.Public},
	})
	dischargeMacaroon, err := bk.Oven.NewMacaroon(context.TODO(), bakery.LatestVersion, []checkers.Caveat{
		checkers.NeedDeclaredCaveat(checkers.Caveat{
			Location:  "third party location",
			Condition: "third party caveat",
		}),
	}, bakery.Op{Entity: "secret", Action: "read"})
	c.Assert(err, tc.ErrorIsNil)

	var (
		callCount     int
		dischargedMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		var resultErr *params.Error
		if callCount == 0 {
			// First call: return a discharge-required error.
			resultErr = &params.Error{
				Code: params.CodeDischargeRequired,
				Info: params.DischargeRequiredErrorInfo{
					BakeryMacaroon: dischargeMacaroon,
				}.AsMap(),
			}
		} else {
			// Second call: succeed, and capture the discharged macaroon
			// that was sent.
			argParam := arg.(params.GetRemoteSecretContentArgs)
			dischargedMac = argParam.Args[0].Macaroons
		}
		resp := params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Error: resultErr,
				Content: params.SecretContentParams{
					Data: map[string]string{"key": "value"},
				},
				LatestRevision: new(1),
			}},
		}
		fillResponse(c, result, resp)
		callCount++
		return nil
	})
	acquirer := &mockDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelsecrets.NewClient(callerWithBakery)
	// Patch the retry Clock with a test clock that is never advanced.
	// The discharge happens inside Func and Clock.After is
	// never called, so the call completes without advancing the clock.
	clock := testclock.NewClock(time.Now())
	s.PatchValue(&crossmodelsecrets.Clock, clock)
	content, _, latestRevision, _, err := client.GetRemoteSecretContentInfo(c.Context(), uri, 0, true, false, coretesting.ControllerTag.Id(), "token", 666, nil)
	c.Check(err, tc.ErrorIsNil)
	c.Check(latestRevision, tc.Equals, 1)
	c.Check(content.SecretValue.EncodedValues(), tc.DeepEquals, map[string]string{"key": "value"})
	// Only 2 API calls were made (1st: discharge-required, 2nd: success).
	c.Check(callCount, tc.Equals, 2)
	// The discharged macaroon was sent on the second call.
	c.Assert(dischargedMac, tc.HasLen, 1)
	c.Assert(dischargedMac[0].Id(), tc.DeepEquals, []byte("discharge mac"))
}

// failingDischargeAcquirer implements base.MacaroonDischarger for testing
// macaroon discharge failures.
type failingDischargeAcquirer struct {
	base.MacaroonDischarger
}

func (m *failingDischargeAcquirer) DischargeAll(ctx context.Context, b *bakery.Macaroon) (macaroon.Slice, error) {
	return nil, errors.New("discharge failed")
}

// TestGetRemoteSecretContentInfoDischargeFailure verifies that when the
// macaroon discharge itself fails, the error is treated as fatal and not
// retried.
func (s *CrossControllerSuite) TestGetRemoteSecretContentInfoDischargeFailure(c *tc.C) {
	uri := coresecrets.NewURI()
	key, err := bakery.GenerateKey()
	c.Assert(err, tc.ErrorIsNil)
	bk := bakery.New(bakery.BakeryParams{
		Key:     key,
		Locator: testLocator{key.Public},
	})
	dischargeMacaroon, err := bk.Oven.NewMacaroon(context.TODO(), bakery.LatestVersion, []checkers.Caveat{
		checkers.NeedDeclaredCaveat(checkers.Caveat{
			Location:  "third party location",
			Condition: "third party caveat",
		}),
	}, bakery.Op{Entity: "secret", Action: "read"})
	c.Assert(err, tc.ErrorIsNil)

	callCount := 0
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		callCount++
		resp := params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Error: &params.Error{
					Code: params.CodeDischargeRequired,
					Info: params.DischargeRequiredErrorInfo{
						BakeryMacaroon: dischargeMacaroon,
					}.AsMap(),
				},
			}},
		}
		fillResponse(c, result, resp)
		return nil
	})
	acquirer := &failingDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelsecrets.NewClient(callerWithBakery)
	// Use a test clock that is never advanced. If the discharge failure
	// were retried, retry.Call would call Clock.After and block forever.
	clock := testclock.NewClock(time.Now())
	s.PatchValue(&crossmodelsecrets.Clock, clock)
	content, _, _, _, err := client.GetRemoteSecretContentInfo(c.Context(), uri, 0, true, false, coretesting.ControllerTag.Id(), "token", 666, nil)
	c.Check(err, tc.NotNil)
	c.Check(content, tc.IsNil)
	// Only 1 API call was made — the discharge failure is fatal and not
	// retried.
	c.Check(callCount, tc.Equals, 1)
}

func (s *CrossControllerSuite) TestGetSecretAccessScope(c *tc.C) {
	uri := coresecrets.NewURI()
	appUUID := tc.Must(c, application.NewUUID)
	relUUID := relationtesting.GenRelationUUID(c)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		c.Check(objType, tc.Equals, "CrossModelSecrets")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetSecretAccessScope")
		c.Check(arg, tc.DeepEquals, params.GetRemoteSecretAccessArgs{
			Args: []params.GetRemoteSecretAccessArg{{
				ApplicationToken: appUUID.String(),
				UnitId:           666,
				URI:              uri.String(),
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: relUUID.String(),
			}},
		}
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	scope, err := client.GetSecretAccessScope(c.Context(), uri, appUUID, 666)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(scope, tc.Equals, relUUID)
}
