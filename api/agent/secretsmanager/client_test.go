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
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
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

func (s *SecretsSuite) TestGetSecretStoreConfig(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSecretStoreConfig")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.SecretStoreConfig{})
		*(result.(*params.SecretStoreConfig)) = params.SecretStoreConfig{
			StoreType: "juju",
			Params:    map[string]interface{}{"foo": "bar"},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.GetSecretBackendConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, &provider.BackendConfig{
		BackendType: "juju",
		Config:      map[string]interface{}{"foo": "bar"},
	})
}

func (s *SecretsSuite) TestCreateSecretURIs(c *gc.C) {
	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "CreateSecretURIs")
		c.Check(arg, jc.DeepEquals, params.CreateSecretURIsArg{
			Count: 2,
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			[]params.StringResult{{
				Result: uri.String(),
			}, {
				Result: uri2.String(),
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.CreateSecretURIs(2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []*coresecrets.URI{uri, uri2})
}

func (s *SecretsSuite) TestGetContentInfo(c *gc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSecretContentInfo")
		c.Check(arg, jc.DeepEquals, params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				URI:     uri.String(),
				Label:   "label",
				Refresh: true,
				Peek:    true,
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			[]params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.GetContentInfo(uri, "label", true, true)
	c.Assert(err, jc.ErrorIsNil)
	value := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(result, jc.DeepEquals, &secrets.ContentParams{SecretValue: value})
}

func (s *SecretsSuite) TestGetContentInfoLabelArgOnly(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSecretContentInfo")
		c.Check(arg, jc.DeepEquals, params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				Label:   "label",
				Refresh: true,
				Peek:    true,
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			[]params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.GetContentInfo(nil, "label", true, true)
	c.Assert(err, jc.ErrorIsNil)
	value := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(result, jc.DeepEquals, &secrets.ContentParams{SecretValue: value})
}

func (s *SecretsSuite) TestGetContentInfoError(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			[]params.SecretContentResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	uri := coresecrets.NewURI()
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.GetContentInfo(uri, "", true, true)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, gc.IsNil)
}

func (s *SecretsSuite) TestGetSecretMetadata(c *gc.C) {
	uri := coresecrets.NewURI()
	now := time.Now()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetSecretMetadata")
		c.Check(arg, jc.DeepEquals, params.ListSecretsArgs{
			Filter: params.SecretsFilter{OwnerTag: ptr("application-mariadb")},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ListSecretResults{})
		*(result.(*params.ListSecretResults)) = params.ListSecretResults{
			Results: []params.ListSecretResult{{
				URI:              uri.String(),
				OwnerTag:         "application-mariadb",
				Label:            "label",
				LatestRevision:   666,
				NextRotateTime:   &now,
				LatestExpireTime: &now,
				Revisions: []params.SecretRevision{{
					Revision:  666,
					BackendId: ptr("backend-id"),
				}, {
					Revision: 667,
				}},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.SecretMetadata(coresecrets.Filter{
		OwnerTag: ptr("application-mariadb"),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	for _, info := range result {
		c.Assert(info.Metadata.URI.String(), gc.Equals, uri.String())
		c.Assert(info.Metadata.OwnerTag, gc.Equals, "application-mariadb")
		c.Assert(info.Metadata.Label, gc.Equals, "label")
		c.Assert(info.Metadata.LatestRevision, gc.Equals, 666)
		c.Assert(info.Metadata.LatestExpireTime, gc.Equals, &now)
		c.Assert(info.Metadata.NextRotateTime, gc.Equals, &now)
		c.Assert(info.BackendIds, jc.DeepEquals, map[int]string{666: "backend-id"})
	}
}

func (s *SecretsSuite) TestWatchConsumedSecretsChanges(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchConsumedSecretsChanges")
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
	_, err := client.WatchConsumedSecretsChanges("foo/0")
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *SecretsSuite) GetConsumerSecretsRevisionInfo(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetConsumerSecretsRevisionInfo")
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
	var info map[string]coresecrets.SecretRevisionInfo
	client := secretsmanager.NewClient(apiCaller)
	info, err := client.GetConsumerSecretsRevisionInfo("foo-0", []string{
		"secret:9m4e2mr0ui3e8a215n4g", "secret:8n3e2mr0ui3e8a215n5h", "secret:7c5e2mr0ui3e8a2154r2"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, map[string]coresecrets.SecretRevisionInfo{})
}

func (s *SecretsSuite) TestWatchObsolete(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchObsolete")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-foo-0"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	_, err := client.WatchObsolete(names.NewUnitTag("foo/0"))
	c.Assert(err, gc.ErrorMatches, "FAIL")
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
		c.Assert(result, gc.FitsTypeOf, &params.SecretTriggerWatchResult{})
		*(result.(*params.SecretTriggerWatchResult)) = params.SecretTriggerWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	_, err := client.WatchSecretsRotationChanges(names.NewApplicationTag("app"))
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *SecretsSuite) TestSecretRotated(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SecretsRotated")
		c.Check(arg, jc.DeepEquals, params.SecretRotatedArgs{
			Args: []params.SecretRotatedArg{{
				URI:              "secret:9m4e2mr0ui3e8a215n4g",
				OriginalRevision: 666,
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
	err := client.SecretRotated("secret:9m4e2mr0ui3e8a215n4g", 666)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *SecretsSuite) TestWatchSecretRevisionsExpiryChanges(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchSecretRevisionsExpiryChanges")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "application-app"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.SecretTriggerWatchResult{})
		*(result.(*params.SecretTriggerWatchResult)) = params.SecretTriggerWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	_, err := client.WatchSecretRevisionsExpiryChanges(names.NewApplicationTag("app"))
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *SecretsSuite) TestGrant(c *gc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SecretsGrant")
		c.Check(arg, jc.DeepEquals, params.GrantRevokeSecretArgs{
			Args: []params.GrantRevokeSecretArg{{
				URI:         uri.String(),
				ScopeTag:    "relation-wordpress.db#mysql.server",
				SubjectTags: []string{"unit-wordpress-0"},
				Role:        "view",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	err := client.Grant(uri, &secretsmanager.SecretRevokeGrantArgs{
		UnitName:    ptr("wordpress/0"),
		RelationKey: ptr("wordpress:db mysql:server"),
		Role:        coresecrets.RoleView,
	})
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *SecretsSuite) TestRevoke(c *gc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "SecretsManager")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SecretsRevoke")
		c.Check(arg, jc.DeepEquals, params.GrantRevokeSecretArgs{
			Args: []params.GrantRevokeSecretArg{{
				URI:         uri.String(),
				ScopeTag:    "relation-wordpress.db#mysql.server",
				SubjectTags: []string{"application-wordpress"},
				Role:        "view",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	err := client.Revoke(uri, &secretsmanager.SecretRevokeGrantArgs{
		ApplicationName: ptr("wordpress"),
		RelationKey:     ptr("wordpress:db mysql:server"),
		Role:            coresecrets.RoleView,
	})
	c.Assert(err, gc.ErrorMatches, "FAIL")
}
