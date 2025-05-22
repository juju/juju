// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/client/secretbackends"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestSecretBackendsSuite(t *stdtesting.T) {
	tc.Run(t, &SecretBackendsSuite{})
}

type SecretBackendsSuite struct {
	coretesting.BaseSuite
}

func (s *SecretBackendsSuite) TestNewClient(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := secretbackends.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretBackendsSuite) TestListSecretBackends(c *tc.C) {
	config := map[string]interface{}{"foo": "bar"}
	apiCaller := testing.BestVersionCaller{
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SecretBackends")
			c.Check(version, tc.Equals, 1)
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "ListSecretBackends")
			c.Check(arg, tc.DeepEquals, params.ListSecretBackendsArgs{Names: []string{"myvault"}, Reveal: true})
			c.Assert(result, tc.FitsTypeOf, &params.ListSecretBackendsResults{})
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
	result, err := client.ListSecretBackends(c.Context(), []string{"myvault"}, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []secretbackends.SecretBackend{{
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

func (s *SecretBackendsSuite) TestAddSecretsBackend(c *tc.C) {
	backend := secretbackends.CreateSecretBackend{
		ID:                  "backend-id",
		Name:                "foo",
		BackendType:         "vault",
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              map[string]interface{}{"foo": "bar"},
	}
	apiCaller := testing.BestVersionCaller{
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SecretBackends")
			c.Check(version, tc.Equals, 1)
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "AddSecretBackends")
			c.Check(arg, tc.DeepEquals, params.AddSecretBackendArgs{
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
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{
					Error: &params.Error{Message: "FAIL"},
				}},
			}
			return nil
		}), BestVersion: 1,
	}
	client := secretbackends.NewClient(apiCaller)
	err := client.AddSecretBackend(c.Context(), backend)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *SecretBackendsSuite) TestRemoveSecretsBackend(c *tc.C) {
	apiCaller := testing.BestVersionCaller{
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SecretBackends")
			c.Check(version, tc.Equals, 1)
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "RemoveSecretBackends")
			c.Check(arg, tc.DeepEquals, params.RemoveSecretBackendArgs{
				Args: []params.RemoveSecretBackendArg{{
					Name:  "foo",
					Force: true,
				}},
			})
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{
					Error: &params.Error{Message: "FAIL"},
				}},
			}
			return nil
		}), BestVersion: 1,
	}
	client := secretbackends.NewClient(apiCaller)
	err := client.RemoveSecretBackend(c.Context(), "foo", true)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *SecretBackendsSuite) TestUpdateSecretsBackend(c *tc.C) {
	backend := secretbackends.UpdateSecretBackend{
		Name:                "foo",
		NameChange:          ptr("new-name"),
		TokenRotateInterval: ptr(666 * time.Minute),
		Config:              map[string]interface{}{"foo": "bar"},
	}
	apiCaller := testing.BestVersionCaller{
		APICallerFunc: testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "SecretBackends")
			c.Check(version, tc.Equals, 1)
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "UpdateSecretBackends")
			c.Check(arg, tc.DeepEquals, params.UpdateSecretBackendArgs{
				Args: []params.UpdateSecretBackendArg{{
					Name:                backend.Name,
					TokenRotateInterval: backend.TokenRotateInterval,
					Config:              backend.Config,
					NameChange:          backend.NameChange,
					Force:               true,
				}},
			})
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{
					Error: &params.Error{Message: "FAIL"},
				}},
			}
			return nil
		}), BestVersion: 1,
	}
	client := secretbackends.NewClient(apiCaller)
	err := client.UpdateSecretBackend(c.Context(), backend, true)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}
