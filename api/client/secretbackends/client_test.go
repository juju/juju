// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/client/secretbackends"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&SecretBackendsSuite{})

type SecretBackendsSuite struct {
	coretesting.BaseSuite
}

func (s *SecretBackendsSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := secretbackends.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretBackendsSuite) TestListSecretBackends(c *gc.C) {
	config := map[string]interface{}{"foo": "bar"}
	apiCaller := testing.BestVersionCaller{
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "SecretBackends")
			c.Check(version, gc.Equals, 1)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "ListSecretBackends")
			c.Check(arg, jc.DeepEquals, params.ListSecretBackendsArgs{Names: []string{"myvault"}, Reveal: true})
			c.Assert(result, gc.FitsTypeOf, &params.ListSecretBackendsResults{})
			*(result.(*params.ListSecretBackendsResults)) = params.ListSecretBackendsResults{
				[]params.SecretBackendResult{{
					Result: params.SecretBackend{
						Name:                "foo",
						BackendType:         "vault",
						TokenRotateInterval: ptr(666 * time.Minute),
						Config:              config,
					},
					ID:         "backend-id",
					NumSecrets: 666,
					Status:     "error",
					Message:    "vault is sealed",
				}},
			}
			return nil
		}), BestVersion: 1,
	}
	client := secretbackends.NewClient(apiCaller)
	result, err := client.ListSecretBackends([]string{"myvault"}, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []secretbackends.SecretBackend{{
		Name:                "foo",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              config,
		NumSecrets:          666,
		Status:              status.Error,
		Message:             "vault is sealed",
		ID:                  "backend-id",
	}})
}

func (s *SecretBackendsSuite) TestAddSecretsBackend(c *gc.C) {
	backend := secretbackends.CreateSecretBackend{
		ID:                  "backend-id",
		Name:                "foo",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              map[string]interface{}{"foo": "bar"},
	}
	apiCaller := testing.BestVersionCaller{
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "SecretBackends")
			c.Check(version, gc.Equals, 1)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "AddSecretBackends")
			c.Check(arg, jc.DeepEquals, params.AddSecretBackendArgs{
				Args: []params.AddSecretBackendArg{{
					ID: "backend-id",
					SecretBackend: params.SecretBackend{
						Name:                backend.Name,
						BackendType:         backend.BackendType,
						TokenRotateInterval: backend.TokenRotateInterval,
						Config:              backend.Config,
					},
				}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{
					Error: &params.Error{Message: "FAIL"},
				}},
			}
			return nil
		}), BestVersion: 1,
	}
	client := secretbackends.NewClient(apiCaller)
	err := client.AddSecretBackend(backend)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *SecretBackendsSuite) TestRemoveSecretsBackend(c *gc.C) {
	apiCaller := testing.BestVersionCaller{
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "SecretBackends")
			c.Check(version, gc.Equals, 1)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "RemoveSecretBackends")
			c.Check(arg, jc.DeepEquals, params.RemoveSecretBackendArgs{
				Args: []params.RemoveSecretBackendArg{{
					Name:  "foo",
					Force: true,
				}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{
					Error: &params.Error{Message: "FAIL"},
				}},
			}
			return nil
		}), BestVersion: 1,
	}
	client := secretbackends.NewClient(apiCaller)
	err := client.RemoveSecretBackend("foo", true)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *SecretBackendsSuite) TestUpdateSecretsBackend(c *gc.C) {
	backend := secretbackends.UpdateSecretBackend{
		Name:                "foo",
		NameChange:          ptr("new-name"),
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              map[string]interface{}{"foo": "bar"},
	}
	apiCaller := testing.BestVersionCaller{
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "SecretBackends")
			c.Check(version, gc.Equals, 1)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "UpdateSecretBackends")
			c.Check(arg, jc.DeepEquals, params.UpdateSecretBackendArgs{
				Args: []params.UpdateSecretBackendArg{{
					Name:                backend.Name,
					TokenRotateInterval: backend.TokenRotateInterval,
					Config:              backend.Config,
					NameChange:          backend.NameChange,
					Force:               true,
				}},
			})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{
					Error: &params.Error{Message: "FAIL"},
				}},
			}
			return nil
		}), BestVersion: 1,
	}
	client := secretbackends.NewClient(apiCaller)
	err := client.UpdateSecretBackend(backend, true)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}
