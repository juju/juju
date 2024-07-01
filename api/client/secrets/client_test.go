// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/testing"
	apisecrets "github.com/juju/juju/api/client/secrets"
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
	client := apisecrets.NewClient(apiCaller)
	c.Assert(client, gc.NotNil)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) TestListSecrets(c *gc.C) {
	data := map[string]string{"foo": "bar"}
	now := time.Now()
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Secrets")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ListSecrets")
		c.Check(arg, gc.DeepEquals, params.ListSecretsArgs{
			ShowSecrets: true,
			Filter: params.SecretsFilter{
				URI:      ptr(uri.String()),
				Revision: ptr(666),
				OwnerTag: ptr("application-mysql"),
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ListSecretResults{})
		*(result.(*params.ListSecretResults)) = params.ListSecretResults{
			[]params.ListSecretResult{{
				URI:              uri.String(),
				Version:          1,
				OwnerTag:         "application-mysql",
				RotatePolicy:     string(secrets.RotateHourly),
				LatestExpireTime: ptr(now),
				NextRotateTime:   ptr(now.Add(time.Hour)),
				Description:      "shhh",
				Label:            "foobar",
				LatestRevision:   2,
				CreateTime:       now,
				UpdateTime:       now.Add(time.Second),
				Revisions: []params.SecretRevision{{
					Revision:   666,
					CreateTime: now,
					UpdateTime: now.Add(time.Second),
					ExpireTime: ptr(now.Add(time.Hour)),
				}, {
					Revision:    667,
					CreateTime:  now,
					UpdateTime:  now.Add(time.Second),
					ExpireTime:  ptr(now.Add(time.Hour)),
					BackendName: ptr("some backend"),
				}},
				Value: &params.SecretValueResult{Data: data},
				Access: []params.AccessInfo{
					{
						TargetTag: "application-gitlab",
						ScopeTag:  "relation-key",
						Role:      "view",
					},
				},
			}},
		}
		return nil
	})
	client := apisecrets.NewClient(apiCaller)
	owner := secrets.Owner{Kind: secrets.ApplicationOwner, ID: "mysql"}
	result, err := client.ListSecrets(true, secrets.Filter{
		URI: uri, Owner: ptr(owner), Revision: ptr(666)})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []apisecrets.SecretDetails{{
		Metadata: secrets.SecretMetadata{
			URI:              uri,
			Version:          1,
			Owner:            owner,
			RotatePolicy:     secrets.RotateHourly,
			LatestRevision:   2,
			LatestExpireTime: ptr(now),
			NextRotateTime:   ptr(now.Add(time.Hour)),
			Description:      "shhh",
			Label:            "foobar",
			CreateTime:       now,
			UpdateTime:       now.Add(time.Second),
		},
		Revisions: []secrets.SecretRevisionMetadata{{
			Revision:   666,
			CreateTime: now,
			UpdateTime: now.Add(time.Second),
			ExpireTime: ptr(now.Add(time.Hour)),
		}, {
			Revision:    667,
			BackendName: ptr("some backend"),
			CreateTime:  now,
			UpdateTime:  now.Add(time.Second),
			ExpireTime:  ptr(now.Add(time.Hour)),
		}},
		Value: secrets.NewSecretValue(data),
		Access: []secrets.AccessInfo{
			{
				Target: "application-gitlab",
				Scope:  "relation-key",
				Role:   secrets.RoleView,
			},
		},
	}})
}

func (s *SecretsSuite) TestListSecretsError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ListSecretResults)) = params.ListSecretResults{
			[]params.ListSecretResult{{
				URI: "secret:9m4e2mr0ui3e8a215n4g",
				Value: &params.SecretValueResult{
					Error: &params.Error{Message: "boom"},
				},
			}},
		}
		return nil
	})
	client := apisecrets.NewClient(apiCaller)
	result, err := client.ListSecrets(true, secrets.Filter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0].Error, gc.Equals, "boom")
}

func (s *SecretsSuite) TestCreateSecretError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 1}
	client := apisecrets.NewClient(caller)
	_, err := client.CreateSecret("label", "this is a secret.", map[string]string{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, "user secrets not supported")
}

func (s *SecretsSuite) TestCreateSecret(c *gc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "CreateSecrets")
		c.Assert(arg, gc.DeepEquals, params.CreateSecretArgs{
			Args: []params.CreateSecretArg{
				{
					UpsertSecretArg: params.UpsertSecretArg{
						Label:       ptr("my-secret"),
						Description: ptr("this is a secret."),
						Content:     params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
					},
				},
			},
		})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{
				{Result: uri.String()},
			},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	result, err := client.CreateSecret("my-secret", "this is a secret.", map[string]string{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, uri.String())
}

func (s *SecretsSuite) TestUpdateSecretError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 1}
	client := apisecrets.NewClient(caller)
	uri := secrets.NewURI()
	err := client.UpdateSecret(uri, "", ptr(true), "new-name", "this is a secret.", map[string]string{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, "user secrets not supported")
}

func (s *SecretsSuite) TestUpdateSecretWithoutContent(c *gc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "UpdateSecrets")
		c.Assert(arg, gc.DeepEquals, params.UpdateUserSecretArgs{
			Args: []params.UpdateUserSecretArg{
				{
					URI:       uri.String(),
					AutoPrune: ptr(true),
					UpsertSecretArg: params.UpsertSecretArg{
						Label:       ptr("new-name"),
						Description: ptr("this is a secret."),
					},
				},
			},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{Results: []params.ErrorResult{{}}}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	err := client.UpdateSecret(uri, "", ptr(true), "new-name", "this is a secret.", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestUpdateSecretByName(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "UpdateSecrets")
		c.Assert(arg, gc.DeepEquals, params.UpdateUserSecretArgs{
			Args: []params.UpdateUserSecretArg{
				{
					ExistingLabel: "name",
					AutoPrune:     ptr(true),
					UpsertSecretArg: params.UpsertSecretArg{
						Label:       ptr("new-name"),
						Description: ptr("this is a secret."),
					},
				},
			},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{Results: []params.ErrorResult{{}}}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	err := client.UpdateSecret(nil, "name", ptr(true), "new-name", "this is a secret.", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestUpdateSecret(c *gc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "UpdateSecrets")
		c.Assert(arg, gc.DeepEquals, params.UpdateUserSecretArgs{
			Args: []params.UpdateUserSecretArg{
				{
					URI:       uri.String(),
					AutoPrune: ptr(true),
					UpsertSecretArg: params.UpsertSecretArg{
						Label:       ptr("label"),
						Description: ptr("this is a secret."),
						Content:     params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
					},
				},
			},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{Results: []params.ErrorResult{{}}}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	err := client.UpdateSecret(uri, "", ptr(true), "label", "this is a secret.", map[string]string{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestRemoveSecretError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 1}
	client := apisecrets.NewClient(caller)
	uri := secrets.NewURI()
	err := client.RemoveSecret(uri, "", ptr(1))
	c.Assert(err, gc.ErrorMatches, "user secrets not supported")
}

func (s *SecretsSuite) TestRemoveSecret(c *gc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "RemoveSecrets")
		c.Assert(arg, gc.DeepEquals, params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{
				{URI: uri.String()},
			},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	err := client.RemoveSecret(uri, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestRemoveSecretByName(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "RemoveSecrets")
		c.Assert(arg, gc.DeepEquals, params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{
				{Label: "my-secret"},
			},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	err := client.RemoveSecret(nil, "my-secret", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestRemoveSecretWithRevision(c *gc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "RemoveSecrets")
		c.Assert(arg, gc.DeepEquals, params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{
				{URI: uri.String(), Revisions: []int{1}},
			},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	err := client.RemoveSecret(uri, "", ptr(1))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestGrantSecretError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 1}
	client := apisecrets.NewClient(caller)
	uri := secrets.NewURI()
	_, err := client.GrantSecret(uri, "", []string{"gitlab"})
	c.Assert(err, gc.ErrorMatches, "user secrets not supported")
}

func (s *SecretsSuite) TestGrantSecret(c *gc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "GrantSecret")
		c.Assert(arg, gc.DeepEquals, params.GrantRevokeUserSecretArg{
			URI: uri.String(), Applications: []string{"gitlab"},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{nil}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	result, err := client.GrantSecret(uri, "", []string{"gitlab"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []error{nil})
}

func (s *SecretsSuite) TestGrantSecretByName(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "GrantSecret")
		c.Assert(arg, gc.DeepEquals, params.GrantRevokeUserSecretArg{
			Label: "my-secret", Applications: []string{"gitlab"},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{nil}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	result, err := client.GrantSecret(nil, "my-secret", []string{"gitlab"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []error{nil})
}

func (s *SecretsSuite) TestRevokeSecretError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 1}
	client := apisecrets.NewClient(caller)
	uri := secrets.NewURI()
	_, err := client.RevokeSecret(uri, "", []string{"gitlab"})
	c.Assert(err, gc.ErrorMatches, "user secrets not supported")
}

func (s *SecretsSuite) TestRevokeSecret(c *gc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "RevokeSecret")
		c.Assert(arg, gc.DeepEquals, params.GrantRevokeUserSecretArg{
			URI: uri.String(), Applications: []string{"gitlab"},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{nil}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	result, err := client.RevokeSecret(uri, "", []string{"gitlab"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []error{nil})
}

func (s *SecretsSuite) TestRevokeSecretByName(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "RevokeSecret")
		c.Assert(arg, gc.DeepEquals, params.GrantRevokeUserSecretArg{
			Label: "my-secret", Applications: []string{"gitlab"},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{nil}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	result, err := client.RevokeSecret(nil, "my-secret", []string{"gitlab"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []error{nil})
}

func (s *SecretsSuite) TestGetModelSecretBackendNotSupported(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	_, err := client.GetModelSecretBackend(context.Background())
	c.Assert(err, gc.ErrorMatches, "getting model secret backend not supported")
}

func (s *SecretsSuite) TestGetModelSecretBackend(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "GetModelSecretBackend")
		*(result.(*params.StringResult)) = params.StringResult{
			Result: "backend-id",
		}
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 3}
	client := apisecrets.NewClient(caller)
	result, err := client.GetModelSecretBackend(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "backend-id")
}

func (s *SecretsSuite) TestSetModelSecretBackendNotSupported(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	err := client.SetModelSecretBackend(context.Background(), "backend-id")
	c.Assert(err, gc.ErrorMatches, "setting model secret backend not supported")
}

func (s *SecretsSuite) TestSetModelSecretBackendEmptyArg(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 2}
	client := apisecrets.NewClient(caller)
	err := client.SetModelSecretBackend(context.Background(), "")
	c.Assert(err, gc.ErrorMatches, "secret backend name cannot be empty")
}

func (s *SecretsSuite) TestSetModelSecretBackend(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Secrets")
		c.Assert(request, gc.Equals, "SetModelSecretBackend")
		c.Assert(arg, gc.DeepEquals, params.SetModelSecretBackendArg{
			SecretBackendName: "backend-id",
		})
		return nil
	})
	caller := testing.BestVersionCaller{apiCaller, 3}
	client := apisecrets.NewClient(caller)
	err := client.SetModelSecretBackend(context.Background(), "backend-id")
	c.Assert(err, jc.ErrorIsNil)
}
