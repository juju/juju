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
	uri := secrets.NewURI()
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
					ExpireTime:   ptr(expiry),
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
				Result: uri.String(),
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	value := secrets.NewSecretValue(data)
	cfg := &secrets.SecretConfig{
		RotatePolicy: ptr(secrets.RotateDaily),
		ExpireTime:   ptr(expiry),
		Description:  ptr("my secret"),
		Label:        ptr("foo"),
		Params: map[string]interface{}{
			"password-length":        10,
			"password-special-chars": true,
		},
	}
	result, err := client.Create(cfg, names.NewApplicationTag("mysql"), value)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.ID, gc.Equals, uri.ID)
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
	c.Assert(result, gc.IsNil)
}

func (s *SecretsSuite) TestGetSecret(c *gc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSecretValues")
		c.Check(arg, jc.DeepEquals, params.GetSecretValueArgs{
			Args: []params.GetSecretValueArg{{
				URI:    uri.String(),
				Label:  "label",
				Update: true,
				Peek:   true,
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
	result, err := client.GetValue(uri, "label", true, true)
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
	uri := secrets.NewURI()
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.GetValue(uri, "", true, true)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, gc.IsNil)
}

func (s *SecretsSuite) TestGetSecretIds(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSecretIds")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.SecretIdResults{})
		*(result.(*params.SecretIdResults)) = params.SecretIdResults{
			Result: map[string]params.SecretIdResult{
				"secret:9m4e2mr0ui3e8a215n4g": {Label: "label"},
			},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.SecretIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	for uri, label := range result {
		c.Assert(uri.ShortString(), gc.Equals, "secret:9m4e2mr0ui3e8a215n4g")
		c.Assert(label, gc.Equals, "label")
	}
}

func (s *SecretsSuite) TestGetSecretIdsError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.SecretIdResults)) = params.SecretIdResults{
			Error: &params.Error{Message: "boom"},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.SecretIds()
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, gc.IsNil)
}

func (s *SecretsSuite) TestWatchSecretsChanges(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchSecretsChanges")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-foo-0"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	_, err := client.WatchSecretsChanges("foo/0")
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *SecretsSuite) GetLatestSecretsRevisionInfo(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetLatestSecretsRevisionInfo")
		c.Check(arg, jc.DeepEquals, params.GetSecretConsumerInfoArgs{
			ConsumerTag: "unit-foo-0",
			URIs: []string{
				"secret:9m4e2mr0ui3e8a215n4g", "secret:8n3e2mr0ui3e8a215n5h", "secret:7c5e2mr0ui3e8a2154r2"},
		})
		c.Assert(result, gc.FitsTypeOf, &params.SecretConsumerInfoResults{})
		*(result.(*params.SecretConsumerInfoResults)) = params.SecretConsumerInfoResults{
			Results: []params.SecretConsumerInfoResult{{
				Revision: 666,
				Label:    "foobar",
			}, {
				Error: &params.Error{Code: params.CodeUnauthorized},
			}, {
				Error: &params.Error{Code: params.CodeNotFound},
			}},
		}
		return nil
	})
	var info map[string]secretsmanager.SecretRevisionInfo
	client := secretsmanager.NewClient(apiCaller)
	info, err := client.GetLatestSecretsRevisionInfo("foo-0", []string{
		"secret:9m4e2mr0ui3e8a215n4g", "secret:8n3e2mr0ui3e8a215n5h", "secret:7c5e2mr0ui3e8a2154r2"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, map[string]secretsmanager.SecretRevisionInfo{})
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
