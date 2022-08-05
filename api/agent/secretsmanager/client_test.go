// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager_test

import (
	"time"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/secretsmanager"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&SecretsSuite{})

type SecretsSuite struct {
	coretesting.BaseSuite
}

func (s *SecretsSuite) TestNewClient(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) TestCreateSecret(c *gc.C) {
	data := map[string]string{"foo": "bar"}
	expiry := time.Now()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "CreateSecrets")
		c.Check(arg, jc.DeepEquals, params.CreateSecretArgs{
			Args: []params.CreateSecretArg{{
				OwnerTag: "application-mysql",
				UpsertSecretArg: params.UpsertSecretArg{
					RotatePolicy: ptr(secrets.RotateDaily),
					Expiry:       ptr(expiry),
					Description:  ptr("my secret"),
					Label:        ptr("foo"),
					Params: map[string]interface{}{
						"password-length":        10,
						"password-special-chars": true,
					},
					Data: data,
				},
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			[]params.StringResult{{
				Result: "secret:9m4e2mr0ui3e8a215n4g",
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	value := secrets.NewSecretValue(data)
	cfg := &secrets.SecretConfig{
		RotatePolicy: ptr(secrets.RotateDaily),
		Expiry:       ptr(expiry),
		Description:  ptr("my secret"),
		Label:        ptr("foo"),
		Params: map[string]interface{}{
			"password-length":        10,
			"password-special-chars": true,
		},
	}
	result, err := client.Create(cfg, names.NewApplicationTag("mysql"), value)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "secret:9m4e2mr0ui3e8a215n4g")
}

func (s *SecretsSuite) TestCreateSecretsError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.StringResults)) = params.StringResults{
			[]params.StringResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	value := secrets.NewSecretValue(nil)
	result, err := client.Create(&secrets.SecretConfig{}, names.NewApplicationTag("mysql"), value)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, gc.Equals, "")
}

func (s *SecretsSuite) TestUpdateSecret(c *gc.C) {
	data := map[string]string{"foo": "bar"}
	expiry := time.Now()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "UpdateSecrets")
		c.Check(arg, jc.DeepEquals, params.UpdateSecretArgs{
			Args: []params.UpdateSecretArg{{
				URI: "secret:9m4e2mr0ui3e8a215n4g",
				UpsertSecretArg: params.UpsertSecretArg{
					RotatePolicy: ptr(secrets.RotateDaily),
					Expiry:       ptr(expiry),
					Description:  ptr("my secret"),
					Label:        ptr("foo"),
					Params: map[string]interface{}{
						"password-length":        10,
						"password-special-chars": true,
					},
					Data: data,
				},
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			[]params.ErrorResult{{}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	value := secrets.NewSecretValue(data)
	cfg := &secrets.SecretConfig{
		RotatePolicy: ptr(secrets.RotateDaily),
		Expiry:       ptr(expiry),
		Description:  ptr("my secret"),
		Label:        ptr("foo"),
		Params: map[string]interface{}{
			"password-length":        10,
			"password-special-chars": true,
		},
	}
	err := client.Update("secret:9m4e2mr0ui3e8a215n4g", cfg, value)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestUpdateSecretsError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			[]params.ErrorResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	value := secrets.NewSecretValue(nil)
	err := client.Update("secret:9m4e2mr0ui3e8a215n4g", &secrets.SecretConfig{}, value)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *SecretsSuite) TestGetSecret(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSecretValues")
		c.Check(arg, jc.DeepEquals, params.GetSecretArgs{
			Args: []params.GetSecretArg{{
				URI: "secret:9m4e2mr0ui3e8a215n4g",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.SecretValueResults{})
		*(result.(*params.SecretValueResults)) = params.SecretValueResults{
			[]params.SecretValueResult{{
				Data: map[string]string{"foo": "bar"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.GetValue("secret:9m4e2mr0ui3e8a215n4g")
	c.Assert(err, jc.ErrorIsNil)
	value := secrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(result, jc.DeepEquals, value)
}

func (s *SecretsSuite) TestGetSecretsError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.SecretValueResults)) = params.SecretValueResults{
			[]params.SecretValueResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.GetValue("secret:9m4e2mr0ui3e8a215n4g")
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, gc.IsNil)
}

func (s *SecretsSuite) TestWatchSecretsRotationChanges(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchSecretsRotationChanges")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "application-app"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.SecretRotationWatchResults{})
		*(result.(*params.SecretRotationWatchResults)) = params.SecretRotationWatchResults{
			[]params.SecretRotationWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	_, err := client.WatchSecretsRotationChanges("application-app")
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *SecretsSuite) TestSecretRotated(c *gc.C) {
	now := time.Now()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SecretsRotated")
		c.Check(arg, jc.DeepEquals, params.SecretRotatedArgs{
			Args: []params.SecretRotatedArg{{
				URI:  "secret:9m4e2mr0ui3e8a215n4g",
				When: now,
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			[]params.ErrorResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	err := client.SecretRotated("secret:9m4e2mr0ui3e8a215n4g", now)
	c.Assert(err, gc.ErrorMatches, "boom")
}
