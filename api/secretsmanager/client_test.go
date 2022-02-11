// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/secretsmanager"
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

func durationPtr(d time.Duration) *time.Duration {
	return &d
}

func stringPtr(s string) *string {
	return &s
}

func statusPtr(s secrets.SecretStatus) *secrets.SecretStatus {
	return &s
}

func tagPtr(t map[string]string) *map[string]string {
	return &t
}

func (s *SecretsSuite) TestCreateSecret(c *gc.C) {
	data := map[string]string{"foo": "bar"}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "CreateSecrets")
		c.Check(arg, gc.DeepEquals, params.CreateSecretArgs{
			Args: []params.CreateSecretArg{{
				Type:           "password",
				Path:           "app/password",
				RotateInterval: time.Hour,
				Status:         "active",
				Description:    "my secret",
				Tags:           map[string]string{"foo": "bar"},
				Params: map[string]interface{}{
					"password-length":        10,
					"password-special-chars": true,
				},
				Data: data,
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			[]params.StringResult{{
				Result: "secret://app/password",
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	value := secrets.NewSecretValue(data)
	cfg := secrets.NewPasswordSecretConfig(10, true, "app", "password")
	cfg.RotateInterval = durationPtr(time.Hour)
	cfg.Description = stringPtr("my secret")
	cfg.Status = statusPtr(secrets.StatusActive)
	cfg.Tags = tagPtr(map[string]string{"foo": "bar"})
	result, err := client.Create(cfg, secrets.TypePassword, value)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "secret://app/password")
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
	result, err := client.Create(secrets.NewSecretConfig("app", "password"), secrets.TypeBlob, value)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, gc.Equals, "")
}

func (s *SecretsSuite) TestUpdateSecret(c *gc.C) {
	data := map[string]string{"foo": "bar"}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "UpdateSecrets")
		c.Check(arg, gc.DeepEquals, params.UpdateSecretArgs{
			Args: []params.UpdateSecretArg{{
				URL:            "secret://app/foo",
				RotateInterval: durationPtr(time.Hour),
				Status:         stringPtr("active"),
				Description:    stringPtr("my secret"),
				Tags:           tagPtr(map[string]string{"foo": "bar"}),
				Params: map[string]interface{}{
					"password-length":        10,
					"password-special-chars": true,
				},
				Data: data,
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			[]params.StringResult{{
				Result: "secret://app/foo/2",
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	value := secrets.NewSecretValue(data)
	cfg := secrets.NewPasswordSecretConfig(10, true, "app", "password")
	cfg.RotateInterval = durationPtr(time.Hour)
	cfg.Description = stringPtr("my secret")
	cfg.Status = statusPtr(secrets.StatusActive)
	cfg.Tags = tagPtr(map[string]string{"foo": "bar"})
	result, err := client.Update("secret://app/foo", cfg, value)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "secret://app/foo/2")
}

func (s *SecretsSuite) TestUpdateSecretsError(c *gc.C) {
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
	result, err := client.Update("secret://app/foo", secrets.NewSecretConfig("app", "password"), value)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, gc.Equals, "")
}

func (s *SecretsSuite) TestGetSecretByURL(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSecretValues")
		c.Check(arg, gc.DeepEquals, params.GetSecretArgs{
			Args: []params.GetSecretArg{{
				URL: "secret://app/foo",
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
	result, err := client.GetValue("secret://app/foo")
	c.Assert(err, jc.ErrorIsNil)
	value := secrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(result, jc.DeepEquals, value)
}

func (s *SecretsSuite) TestGetSecretByID(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSecretValues")
		c.Check(arg, gc.DeepEquals, params.GetSecretArgs{
			Args: []params.GetSecretArg{{
				ID: "1234",
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
	result, err := client.GetValue("1234")
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
	result, err := client.GetValue("secret://app/foo")
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, gc.IsNil)
}

func (s *SecretsSuite) TestWatchSecretsRotationChanges(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchSecretsRotationChanges")
		c.Check(arg, gc.DeepEquals, params.Entities{
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
		c.Check(arg, gc.DeepEquals, params.SecretRotatedArgs{
			Args: []params.SecretRotatedArg{{
				URL:  "secret://app/foo",
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
	err := client.SecretRotated("secret://app/foo", now)
	c.Assert(err, gc.ErrorMatches, "boom")
}
