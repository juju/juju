// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets_test

import (
	"time"

	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/crossmodelsecrets"
	coresecrets "github.com/juju/juju/core/secrets"
	jujujutesting "github.com/juju/juju/juju/testing"
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
	macs := macaroon.Slice{jujujutesting.MustNewMacaroon("test")}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CrossModelSecrets")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSecretContentInfo")
		c.Check(arg, jc.DeepEquals, params.GetRemoteSecretContentArgs{
			Args: []params.GetRemoteSecretContentArg{{
				ApplicationToken: "token",
				UnitId:           666,
				Revision:         ptr(665),
				Macaroons:        macs,
				BakeryVersion:    3,
				URI:              uri.String(),
				Refresh:          true,
				Peek:             true,
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
	content, backend, latestRevision, draining, err := client.GetRemoteSecretContentInfo(uri, 665, true, true, "token", 666, macs)
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
	content, backend, _, _, err := client.GetRemoteSecretContentInfo(coresecrets.NewURI(), 665, false, false, "token", 666, nil)
	c.Assert(err, gc.ErrorMatches, "attempt count exceeded: boom")
	c.Assert(content, gc.IsNil)
	c.Assert(backend, gc.IsNil)
	c.Assert(attemptCount, gc.Equals, 3)
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
