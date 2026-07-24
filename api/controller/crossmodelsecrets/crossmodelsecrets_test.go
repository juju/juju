// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/crossmodelsecrets"
	apitesting "github.com/juju/juju/api/testing"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	secretsprovider "github.com/juju/juju/secrets/provider"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&CrossControllerSuite{})

type CrossControllerSuite struct {
	coretesting.BaseSuite
}

func (s *CrossControllerSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *CrossControllerSuite) TestGetRemoteSecretContentInfo(c *gc.C) {
	uri := coresecrets.NewURI()
	macs := macaroon.Slice{apitesting.MustNewMacaroon("test")}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelSecrets")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSecretContentInfo")
		c.Check(arg, jc.DeepEquals, params.GetRemoteSecretContentArgs{
			Args: []params.GetRemoteSecretContentArg{{
				SourceControllerUUID: coretesting.ControllerTag.Id(),
				ApplicationToken:     "token",
				UnitId:               666,
				Revision:             ptr(665),
				Macaroons:            macs,
				BakeryVersion:        3,
				URI:                  uri.String(),
				Refresh:              true,
				Peek:                 true,
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			[]params.SecretContentResult{{
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
						Params:      map[string]interface{}{"foo": "bar"},
					},
				},
				LatestRevision: ptr(666),
			}},
		}
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	content, backend, latestRevision, draining, err := client.GetRemoteSecretContentInfo(uri, 665, true, true, coretesting.ControllerTag.Id(), "token", 666, macs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(latestRevision, gc.Equals, 666)
	c.Assert(draining, jc.IsTrue)
	c.Assert(content, jc.DeepEquals, &secrets.ContentParams{
		ValueRef: &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		},
	})
	c.Assert(backend, jc.DeepEquals, &secretsprovider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: secretsprovider.BackendConfig{
			BackendType: "vault",
			Config:      map[string]interface{}{"foo": "bar"},
		},
	})
}

func (s *CrossControllerSuite) TestControllerInfoError(c *gc.C) {
	s.PatchValue(&crossmodelsecrets.Clock, testclock.NewDilatedWallClock(time.Millisecond))
	attemptCount := 0
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		attemptCount++
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			[]params.SecretContentResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	content, backend, _, _, err := client.GetRemoteSecretContentInfo(coresecrets.NewURI(), 665, false, false, coretesting.ControllerTag.Id(), "token", 666, nil)
	c.Assert(err, gc.ErrorMatches, "attempt count exceeded: boom")
	c.Assert(content, gc.IsNil)
	c.Assert(backend, gc.IsNil)
	c.Assert(attemptCount, gc.Equals, 3)
}

// mockDischargeAcquirer implements base.MacaroonDischarger for testing
// macaroon discharge flows.
type mockDischargeAcquirer struct {
	base.MacaroonDischarger
}

func (m *mockDischargeAcquirer) DischargeAll(ctx context.Context, b *bakery.Macaroon) (macaroon.Slice, error) {
	mac, err := apitesting.NewMacaroon("discharge mac")
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

func fillResponse(c *gc.C, resp interface{}, value interface{}) {
	b, err := json.Marshal(value)
	c.Assert(err, jc.ErrorIsNil)
	err = json.Unmarshal(b, resp)
	c.Assert(err, jc.ErrorIsNil)
}

// TestGetRemoteSecretContentInfoDischargeRequired verifies that when the
// remote controller returns a discharge-required error, the macaroon is
// discharged and the call is retried immediately within the same Func
// invocation — without incurring the retry.Call delay.
func (s *CrossControllerSuite) TestGetRemoteSecretContentInfoDischargeRequired(c *gc.C) {
	uri := coresecrets.NewURI()
	key, err := bakery.GenerateKey()
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)

	var (
		callCount     int
		dischargedMac macaroon.Slice
	)
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
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
				LatestRevision: ptr(1),
			}},
		}
		fillResponse(c, result, resp)
		callCount++
		return nil
	})
	acquirer := &mockDischargeAcquirer{}
	callerWithBakery := testing.APICallerWithBakery(apiCaller, acquirer)
	client := crossmodelsecrets.NewClient(callerWithBakery)
	start := time.Now()
	content, _, latestRevision, _, err := client.GetRemoteSecretContentInfo(uri, 0, true, false, coretesting.ControllerTag.Id(), "token", 666, nil)
	elapsed := time.Since(start)
	c.Check(err, jc.ErrorIsNil)
	c.Check(latestRevision, gc.Equals, 1)
	c.Check(content.SecretValue.EncodedValues(), gc.DeepEquals, map[string]string{"key": "value"})
	// Only 2 API calls were made (1st: discharge-required, 2nd: success).
	c.Check(callCount, gc.Equals, 2)
	// The discharged macaroon was sent on the second call.
	c.Assert(dischargedMac, gc.HasLen, 1)
	c.Assert(dischargedMac[0].Id(), jc.DeepEquals, []byte("discharge mac"))
	c.Assert(elapsed < crossmodelsecrets.RetryDelay, jc.IsTrue, gc.Commentf("elapsed: %v", elapsed))
}

func (s *CrossControllerSuite) TestGetSecretAccessScope(c *gc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelSecrets")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSecretAccessScope")
		c.Check(arg, jc.DeepEquals, params.GetRemoteSecretAccessArgs{
			Args: []params.GetRemoteSecretAccessArg{{
				ApplicationToken: "app-token",
				UnitId:           666,
				URI:              uri.String(),
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			[]params.StringResult{{
				Result: "scope-token",
			}},
		}
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	scope, err := client.GetSecretAccessScope(uri, "app-token", 666)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scope, gc.Equals, "scope-token")
}
