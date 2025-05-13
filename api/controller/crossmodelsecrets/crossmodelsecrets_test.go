// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/crossmodelsecrets"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets"
	secretsprovider "github.com/juju/juju/internal/secrets/provider"
	coretesting "github.com/juju/juju/internal/testing"
	jujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&CrossControllerSuite{})

type CrossControllerSuite struct {
	coretesting.BaseSuite
}

func (s *CrossControllerSuite) TestNewClient(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *CrossControllerSuite) TestGetRemoteSecretContentInfo(c *tc.C) {
	uri := coresecrets.NewURI()
	macs := macaroon.Slice{jujujutesting.MustNewMacaroon("test")}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CrossModelSecrets")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetSecretContentInfo")
		c.Check(arg, tc.DeepEquals, params.GetRemoteSecretContentArgs{
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
						Params:      map[string]interface{}{"foo": "bar"},
					},
				},
				LatestRevision: ptr(666),
			}},
		}
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	content, backend, latestRevision, draining, err := client.GetRemoteSecretContentInfo(context.Background(), uri, 665, true, true, coretesting.ControllerTag.Id(), "token", 666, macs)
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
			Config:      map[string]interface{}{"foo": "bar"},
		},
	})
}

func (s *CrossControllerSuite) TestControllerInfoError(c *tc.C) {
	s.PatchValue(&crossmodelsecrets.Clock, testclock.NewDilatedWallClock(time.Millisecond))
	attemptCount := 0
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		attemptCount++
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	content, backend, _, _, err := client.GetRemoteSecretContentInfo(context.Background(), coresecrets.NewURI(), 665, false, false, coretesting.ControllerTag.Id(), "token", 666, nil)
	c.Assert(err, tc.ErrorMatches, "attempt count exceeded: boom")
	c.Assert(content, tc.IsNil)
	c.Assert(backend, tc.IsNil)
	c.Assert(attemptCount, tc.Equals, 3)
}

func (s *CrossControllerSuite) TestGetSecretAccessScope(c *tc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CrossModelSecrets")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetSecretAccessScope")
		c.Check(arg, tc.DeepEquals, params.GetRemoteSecretAccessArgs{
			Args: []params.GetRemoteSecretAccessArg{{
				ApplicationToken: "app-token",
				UnitId:           666,
				URI:              uri.String(),
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "scope-token",
			}},
		}
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	scope, err := client.GetSecretAccessScope(context.Background(), uri, "app-token", 666)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(scope, tc.Equals, "scope-token")
}
