// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"context"
	"time"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api/base/testing"
	apisecrets "github.com/juju/juju/api/client/secrets"
	"github.com/juju/juju/core/secrets"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

var _ = tc.Suite(&SecretsSuite{})

type SecretsSuite struct {
	coretesting.BaseSuite
}

func (s *SecretsSuite) TestNewClient(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	client := apisecrets.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) TestListSecrets(c *tc.C) {
	data := map[string]string{"foo": "bar"}
	now := time.Now()
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Secrets")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ListSecrets")
		c.Check(arg, tc.DeepEquals, params.ListSecretsArgs{
			ShowSecrets: true,
			Filter: params.SecretsFilter{
				URI:      ptr(uri.String()),
				Revision: ptr(666),
				OwnerTag: ptr("application-mysql"),
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ListSecretResults{})
		*(result.(*params.ListSecretResults)) = params.ListSecretResults{
			Results: []params.ListSecretResult{{
				URI:                    uri.String(),
				Version:                1,
				OwnerTag:               "application-mysql",
				RotatePolicy:           string(secrets.RotateHourly),
				LatestExpireTime:       ptr(now),
				NextRotateTime:         ptr(now.Add(time.Hour)),
				Description:            "shhh",
				Label:                  "foobar",
				LatestRevision:         2,
				LatestRevisionChecksum: "checksum",
				CreateTime:             now,
				UpdateTime:             now.Add(time.Second),
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
	result, err := client.ListSecrets(context.Background(), true, secrets.Filter{
		URI: uri, Owner: ptr(owner), Revision: ptr(666)})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []apisecrets.SecretDetails{{
		Metadata: secrets.SecretMetadata{
			URI:                    uri,
			Version:                1,
			Owner:                  owner,
			RotatePolicy:           secrets.RotateHourly,
			LatestRevision:         2,
			LatestRevisionChecksum: "checksum",
			LatestExpireTime:       ptr(now),
			NextRotateTime:         ptr(now.Add(time.Hour)),
			Description:            "shhh",
			Label:                  "foobar",
			CreateTime:             now,
			UpdateTime:             now.Add(time.Second),
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

func (s *SecretsSuite) TestListSecretsError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ListSecretResults)) = params.ListSecretResults{
			Results: []params.ListSecretResult{{
				URI: "secret:9m4e2mr0ui3e8a215n4g",
				Value: &params.SecretValueResult{
					Error: &params.Error{Message: "boom"},
				},
			}},
		}
		return nil
	})
	client := apisecrets.NewClient(apiCaller)
	result, err := client.ListSecrets(context.Background(), true, secrets.Filter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Assert(result[0].Error, tc.Equals, "boom")
}

func (s *SecretsSuite) TestCreateSecretError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 1}
	client := apisecrets.NewClient(caller)
	_, err := client.CreateSecret(context.Background(), "label", "this is a secret.", map[string]string{"foo": "bar"})
	c.Assert(err, tc.ErrorMatches, "user secrets not supported")
}

func (s *SecretsSuite) TestCreateSecret(c *tc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Secrets")
		c.Assert(request, tc.Equals, "CreateSecrets")
		c.Assert(arg, tc.DeepEquals, params.CreateSecretArgs{
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
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 2}
	client := apisecrets.NewClient(caller)
	result, err := client.CreateSecret(context.Background(), "my-secret", "this is a secret.", map[string]string{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, uri.String())
}

func (s *SecretsSuite) TestUpdateSecretError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 1}
	client := apisecrets.NewClient(caller)
	uri := secrets.NewURI()
	err := client.UpdateSecret(context.Background(), uri, "", ptr(true), "new-name", "this is a secret.", map[string]string{"foo": "bar"})
	c.Assert(err, tc.ErrorMatches, "user secrets not supported")
}

func (s *SecretsSuite) TestUpdateSecretWithoutContent(c *tc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Secrets")
		c.Assert(request, tc.Equals, "UpdateSecrets")
		c.Assert(arg, tc.DeepEquals, params.UpdateUserSecretArgs{
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
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 2}
	client := apisecrets.NewClient(caller)
	err := client.UpdateSecret(context.Background(), uri, "", ptr(true), "new-name", "this is a secret.", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestUpdateSecretByName(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Secrets")
		c.Assert(request, tc.Equals, "UpdateSecrets")
		c.Assert(arg, tc.DeepEquals, params.UpdateUserSecretArgs{
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
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 2}
	client := apisecrets.NewClient(caller)
	err := client.UpdateSecret(context.Background(), nil, "name", ptr(true), "new-name", "this is a secret.", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestUpdateSecret(c *tc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Secrets")
		c.Assert(request, tc.Equals, "UpdateSecrets")
		c.Assert(arg, tc.DeepEquals, params.UpdateUserSecretArgs{
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
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 2}
	client := apisecrets.NewClient(caller)
	err := client.UpdateSecret(context.Background(), uri, "", ptr(true), "label", "this is a secret.", map[string]string{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestRemoveSecretError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 1}
	client := apisecrets.NewClient(caller)
	uri := secrets.NewURI()
	err := client.RemoveSecret(context.Background(), uri, "", ptr(1))
	c.Assert(err, tc.ErrorMatches, "user secrets not supported")
}

func (s *SecretsSuite) TestRemoveSecret(c *tc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Secrets")
		c.Assert(request, tc.Equals, "RemoveSecrets")
		c.Assert(arg, tc.DeepEquals, params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{
				{URI: uri.String()},
			},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 2}
	client := apisecrets.NewClient(caller)
	err := client.RemoveSecret(context.Background(), uri, "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestRemoveSecretByName(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Secrets")
		c.Assert(request, tc.Equals, "RemoveSecrets")
		c.Assert(arg, tc.DeepEquals, params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{
				{Label: "my-secret"},
			},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 2}
	client := apisecrets.NewClient(caller)
	err := client.RemoveSecret(context.Background(), nil, "my-secret", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestRemoveSecretWithRevision(c *tc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Secrets")
		c.Assert(request, tc.Equals, "RemoveSecrets")
		c.Assert(arg, tc.DeepEquals, params.DeleteSecretArgs{
			Args: []params.DeleteSecretArg{
				{URI: uri.String(), Revisions: []int{1}},
			},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 2}
	client := apisecrets.NewClient(caller)
	err := client.RemoveSecret(context.Background(), uri, "", ptr(1))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecretsSuite) TestGrantSecretError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 1}
	client := apisecrets.NewClient(caller)
	uri := secrets.NewURI()
	_, err := client.GrantSecret(context.Background(), uri, "", []string{"gitlab"})
	c.Assert(err, tc.ErrorMatches, "user secrets not supported")
}

func (s *SecretsSuite) TestGrantSecret(c *tc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Secrets")
		c.Assert(request, tc.Equals, "GrantSecret")
		c.Assert(arg, tc.DeepEquals, params.GrantRevokeUserSecretArg{
			URI: uri.String(), Applications: []string{"gitlab"},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 2}
	client := apisecrets.NewClient(caller)
	result, err := client.GrantSecret(context.Background(), uri, "", []string{"gitlab"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []error{nil})
}

func (s *SecretsSuite) TestGrantSecretByName(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Secrets")
		c.Assert(request, tc.Equals, "GrantSecret")
		c.Assert(arg, tc.DeepEquals, params.GrantRevokeUserSecretArg{
			Label: "my-secret", Applications: []string{"gitlab"},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 2}
	client := apisecrets.NewClient(caller)
	result, err := client.GrantSecret(context.Background(), nil, "my-secret", []string{"gitlab"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []error{nil})
}

func (s *SecretsSuite) TestRevokeSecretError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 1}
	client := apisecrets.NewClient(caller)
	uri := secrets.NewURI()
	_, err := client.RevokeSecret(context.Background(), uri, "", []string{"gitlab"})
	c.Assert(err, tc.ErrorMatches, "user secrets not supported")
}

func (s *SecretsSuite) TestRevokeSecret(c *tc.C) {
	uri := secrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Secrets")
		c.Assert(request, tc.Equals, "RevokeSecret")
		c.Assert(arg, tc.DeepEquals, params.GrantRevokeUserSecretArg{
			URI: uri.String(), Applications: []string{"gitlab"},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 2}
	client := apisecrets.NewClient(caller)
	result, err := client.RevokeSecret(context.Background(), uri, "", []string{"gitlab"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []error{nil})
}

func (s *SecretsSuite) TestRevokeSecretByName(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Secrets")
		c.Assert(request, tc.Equals, "RevokeSecret")
		c.Assert(arg, tc.DeepEquals, params.GrantRevokeUserSecretArg{
			Label: "my-secret", Applications: []string{"gitlab"},
		})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: nil}},
		}
		return nil
	})
	caller := testing.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 2}
	client := apisecrets.NewClient(caller)
	result, err := client.RevokeSecret(context.Background(), nil, "my-secret", []string{"gitlab"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []error{nil})
}
