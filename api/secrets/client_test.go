// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	apisecrets "github.com/juju/juju/api/secrets"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/secrets"
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
	client := apisecrets.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func (s *SecretsSuite) TestListSecrets(c *gc.C) {
	data := map[string]string{"foo": "bar"}
	now := time.Now()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Secrets")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ListSecrets")
		c.Check(arg, gc.DeepEquals, params.ListSecretsArgs{
			ShowSecrets: true,
		})
		c.Assert(result, gc.FitsTypeOf, &params.ListSecretResults{})
		*(result.(*params.ListSecretResults)) = params.ListSecretResults{
			[]params.ListSecretResult{{
				Path:        "app.password",
				Scope:       "application",
				Version:     1,
				Description: "shhh",
				Tags:        map[string]string{"foo": "bar"},
				ID:          1,
				Provider:    "juju",
				ProviderID:  "provider-id",
				Revision:    2,
				CreateTime:  now,
				UpdateTime:  now.Add(time.Second),
				Value:       &params.SecretValueResult{Data: data},
			}},
		}
		return nil
	})
	client := apisecrets.NewClient(apiCaller)
	result, err := client.ListSecrets(true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []apisecrets.SecretDetails{{
		Metadata: secrets.SecretMetadata{
			Path:        "app.password",
			Scope:       "application",
			Version:     1,
			Description: "shhh",
			Tags:        map[string]string{"foo": "bar"},
			ID:          1,
			Provider:    "juju",
			ProviderID:  "provider-id",
			Revision:    2,
			CreateTime:  now,
			UpdateTime:  now.Add(time.Second),
		},
		Value: secrets.NewSecretValue(data),
	}})
}

func (s *SecretsSuite) TestListSecretsError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ListSecretResults)) = params.ListSecretResults{
			[]params.ListSecretResult{{
				Value: &params.SecretValueResult{
					Error: &params.Error{Message: "boom"},
				},
			}},
		}
		return nil
	})
	client := apisecrets.NewClient(apiCaller)
	result, err := client.ListSecrets(true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0].Error, gc.ErrorMatches, "boom")
}
