// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/controller/crossmodelsecrets"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets"
	secretsprovider "github.com/juju/juju/secrets/provider"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
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
				Macaroons:        nil,
				BakeryVersion:    0,
				URI:              uri.String(),
				Latest:           true,
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
	content, backend, latestRevision, draining, err := client.GetRemoteSecretContentInfo(uri, 665, true, "token", 666)
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
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			[]params.SecretContentResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := crossmodelsecrets.NewClient(apiCaller)
	content, backend, _, _, err := client.GetRemoteSecretContentInfo(coresecrets.NewURI(), 665, false, "token", 666)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(content, gc.IsNil)
	c.Assert(backend, gc.IsNil)
}
