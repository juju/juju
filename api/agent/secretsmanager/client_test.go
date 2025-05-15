// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager_test

import (
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/secretsmanager"
	"github.com/juju/juju/api/base/testing"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
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
	client := secretsmanager.NewClient(apiCaller)
	c.Assert(client, tc.NotNil)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) TestGetSecretBackendConfig(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetSecretBackendConfigs")
		c.Check(arg, tc.DeepEquals, params.SecretBackendArgs{
			BackendIDs: []string{"active-id"},
		})
		c.Assert(result, tc.FitsTypeOf, &params.SecretBackendConfigResults{})
		*(result.(*params.SecretBackendConfigResults)) = params.SecretBackendConfigResults{
			ActiveID: "active-id",
			Results: map[string]params.SecretBackendConfigResult{
				"active-id": {
					ControllerUUID: coretesting.ControllerTag.Id(),
					ModelUUID:      coretesting.ModelTag.Id(),
					ModelName:      "fred",
					Config: params.SecretBackendConfig{
						BackendType: "controller",
						Params:      map[string]interface{}{"foo": "bar"},
					},
				},
			},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.GetSecretBackendConfig(c.Context(), ptr("active-id"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, &provider.ModelBackendConfigInfo{
		ActiveID: "active-id",
		Configs: map[string]provider.ModelBackendConfig{
			"active-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "controller",
					Config:      map[string]interface{}{"foo": "bar"},
				},
			},
		},
	})
}

func (s *SecretsSuite) TestGetBackendConfigForDraining(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetSecretBackendConfigs")
		c.Check(arg, tc.DeepEquals, params.SecretBackendArgs{ForDrain: true, BackendIDs: []string{"active-id"}})
		c.Assert(result, tc.FitsTypeOf, &params.SecretBackendConfigResults{})
		*(result.(*params.SecretBackendConfigResults)) = params.SecretBackendConfigResults{
			ActiveID: "active-id",
			Results: map[string]params.SecretBackendConfigResult{
				"active-id": {
					ControllerUUID: coretesting.ControllerTag.Id(),
					ModelUUID:      coretesting.ModelTag.Id(),
					ModelName:      "fred",
					Config: params.SecretBackendConfig{
						BackendType: "controller",
						Params:      map[string]interface{}{"foo": "bar"},
					},
				},
			},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, activeID, err := client.GetBackendConfigForDrain(c.Context(), ptr("active-id"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "controller",
			Config:      map[string]interface{}{"foo": "bar"},
		},
	})
	c.Assert(activeID, tc.Equals, "active-id")
}

func (s *SecretsSuite) TestCreateSecretURIs(c *tc.C) {
	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "CreateSecretURIs")
		c.Check(arg, tc.DeepEquals, params.CreateSecretURIsArg{
			Count: 2,
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: uri.String(),
			}, {
				Result: uri2.String(),
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.CreateSecretURIs(c.Context(), 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []*coresecrets.URI{uri, uri2})
}

func (s *SecretsSuite) TestGetContentInfo(c *tc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetSecretContentInfo")
		c.Check(arg, tc.DeepEquals, params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				URI:     uri.String(),
				Label:   "label",
				Refresh: true,
				Peek:    true,
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetContentInfo(c.Context(), uri, "label", true, true)
	c.Assert(err, tc.ErrorIsNil)
	value := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(content, tc.DeepEquals, &secrets.ContentParams{SecretValue: value})
	c.Assert(backendConfig, tc.IsNil)
	c.Assert(draining, tc.IsFalse)
}

func (s *SecretsSuite) TestGetContentInfoExternal(c *tc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetSecretContentInfo")
		c.Check(arg, tc.DeepEquals, params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				URI:     uri.String(),
				Label:   "label",
				Refresh: true,
				Peek:    true,
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{ValueRef: &params.SecretValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				}},
				BackendConfig: &params.SecretBackendConfigResult{
					ControllerUUID: "controller-uuid",
					ModelUUID:      "model-uuid",
					ModelName:      "model",
					Draining:       true,
					Config: params.SecretBackendConfig{
						BackendType: "some-backend",
						Params:      map[string]interface{}{"foo": "bar"},
					},
				},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetContentInfo(c.Context(), uri, "label", true, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(content, tc.DeepEquals, &secrets.ContentParams{ValueRef: &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}})
	c.Assert(backendConfig, tc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: "controller-uuid",
		ModelUUID:      "model-uuid",
		ModelName:      "model",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
			Config:      map[string]interface{}{"foo": "bar"},
		},
	})
	c.Assert(draining, tc.IsTrue)
}

func (s *SecretsSuite) TestGetContentInfoLabelArgOnly(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetSecretContentInfo")
		c.Check(arg, tc.DeepEquals, params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				Label:   "label",
				Refresh: true,
				Peek:    true,
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetContentInfo(c.Context(), nil, "label", true, true)
	c.Assert(err, tc.ErrorIsNil)
	value := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(content, tc.DeepEquals, &secrets.ContentParams{SecretValue: value})
	c.Assert(backendConfig, tc.IsNil)
	c.Assert(draining, tc.IsFalse)
}

func (s *SecretsSuite) TestGetContentInfoError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	uri := coresecrets.NewURI()
	client := secretsmanager.NewClient(apiCaller)
	content, backendConfig, _, err := client.GetContentInfo(c.Context(), uri, "", true, true)
	c.Assert(err, tc.ErrorMatches, "boom")
	c.Assert(content, tc.IsNil)
	c.Assert(backendConfig, tc.IsNil)
}

func (s *SecretsSuite) TestGetRevisionContentInfo(c *tc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetSecretRevisionContentInfo")
		c.Check(arg, tc.DeepEquals, params.SecretRevisionArg{
			URI:           uri.String(),
			Revisions:     []int{666},
			PendingDelete: true,
		})
		c.Assert(result, tc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetRevisionContentInfo(c.Context(), uri, 666, true)
	c.Assert(err, tc.ErrorIsNil)
	value := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	c.Assert(content, tc.DeepEquals, &secrets.ContentParams{SecretValue: value})
	c.Assert(backendConfig, tc.IsNil)
	c.Assert(draining, tc.IsFalse)
}

func (s *SecretsSuite) TestGetRevisionContentInfoExternal(c *tc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetSecretRevisionContentInfo")
		c.Check(arg, tc.DeepEquals, params.SecretRevisionArg{
			URI:           uri.String(),
			Revisions:     []int{666},
			PendingDelete: true,
		})
		c.Assert(result, tc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{ValueRef: &params.SecretValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				}},
				BackendConfig: &params.SecretBackendConfigResult{
					ControllerUUID: "controller-uuid",
					ModelUUID:      "model-uuid",
					ModelName:      "model",
					Draining:       true,
					Config: params.SecretBackendConfig{
						BackendType: "some-backend",
						Params:      map[string]interface{}{"foo": "bar"},
					},
				},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	content, backendConfig, draining, err := client.GetRevisionContentInfo(c.Context(), uri, 666, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(content, tc.DeepEquals, &secrets.ContentParams{ValueRef: &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}})
	c.Assert(backendConfig, tc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: "controller-uuid",
		ModelUUID:      "model-uuid",
		ModelName:      "model",
		BackendConfig: provider.BackendConfig{
			BackendType: "some-backend",
			Config:      map[string]interface{}{"foo": "bar"},
		},
	})
	c.Assert(draining, tc.IsTrue)
}

func (s *SecretsSuite) TestGetRevisionContentInfoError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	uri := coresecrets.NewURI()
	client := secretsmanager.NewClient(apiCaller)
	config, backendConfig, _, err := client.GetRevisionContentInfo(c.Context(), uri, 666, true)
	c.Assert(err, tc.ErrorMatches, "boom")
	c.Assert(config, tc.IsNil)
	c.Assert(backendConfig, tc.IsNil)
}

func (s *SecretsSuite) TestSecretMetadata(c *tc.C) {
	uri := coresecrets.NewURI()
	now := time.Now()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetSecretMetadata")
		c.Check(arg, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.ListSecretResults{})
		*(result.(*params.ListSecretResults)) = params.ListSecretResults{
			Results: []params.ListSecretResult{{
				URI:                    uri.String(),
				OwnerTag:               coretesting.ModelTag.String(),
				Label:                  "label",
				LatestRevision:         667,
				LatestRevisionChecksum: "checksum",
				NextRotateTime:         &now,
				LatestExpireTime:       &now,
				Revisions: []params.SecretRevision{{
					Revision: 666,
					ValueRef: &params.SecretValueRef{
						BackendID:  "backend-id",
						RevisionID: "rev-id",
					},
				}, {
					Revision: 667,
				}},
				Access: []params.AccessInfo{
					{
						TargetTag: "application-gitlab",
						ScopeTag:  coretesting.ModelTag.Id(),
						Role:      coresecrets.RoleView,
					},
				},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	result, err := client.SecretMetadata(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	for _, info := range result {
		c.Assert(info.Metadata.URI.String(), tc.Equals, uri.String())
		c.Assert(info.Metadata.Owner, tc.DeepEquals, coresecrets.Owner{Kind: coresecrets.ModelOwner, ID: coretesting.ModelTag.Id()})
		c.Assert(info.Metadata.Label, tc.Equals, "label")
		c.Assert(info.Metadata.LatestRevision, tc.Equals, 667)
		c.Assert(info.Metadata.LatestRevisionChecksum, tc.Equals, "checksum")
		c.Assert(info.Metadata.LatestExpireTime, tc.Equals, &now)
		c.Assert(info.Metadata.NextRotateTime, tc.Equals, &now)
		c.Assert(info.Revisions, tc.DeepEquals, []int{666, 667})
		c.Assert(info.Metadata.Access, tc.DeepEquals, []coresecrets.AccessInfo{
			{
				Target: "application-gitlab",
				Scope:  coretesting.ModelTag.Id(),
				Role:   coresecrets.RoleView,
			},
		})
	}
}

func (s *SecretsSuite) TestWatchConsumedSecretsChanges(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchConsumedSecretsChanges")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-foo-0"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	_, err := client.WatchConsumedSecretsChanges(c.Context(), "foo/0")
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *SecretsSuite) GetConsumerSecretsRevisionInfo(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetConsumerSecretsRevisionInfo")
		c.Check(arg, tc.DeepEquals, params.GetSecretConsumerInfoArgs{
			ConsumerTag: "unit-foo-0",
			URIs: []string{
				"secret:9m4e2mr0ui3e8a215n4g", "secret:8n3e2mr0ui3e8a215n5h", "secret:7c5e2mr0ui3e8a2154r2"},
		})
		c.Assert(result, tc.FitsTypeOf, &params.SecretConsumerInfoResults{})
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
	info, err := client.GetConsumerSecretsRevisionInfo(
		c.Context(),
		"foo-0", []string{
			"secret:9m4e2mr0ui3e8a215n4g", "secret:8n3e2mr0ui3e8a215n5h", "secret:7c5e2mr0ui3e8a2154r2"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, map[string]coresecrets.SecretRevisionInfo{})
}

func (s *SecretsSuite) TestWatchObsolete(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchObsolete")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-foo-0"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	_, err := client.WatchObsolete(c.Context(), names.NewUnitTag("foo/0"))
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *SecretsSuite) TestWatchSecretsRotationChanges(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchSecretsRotationChanges")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "application-app"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.SecretTriggerWatchResult{})
		*(result.(*params.SecretTriggerWatchResult)) = params.SecretTriggerWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	_, err := client.WatchSecretsRotationChanges(c.Context(), names.NewApplicationTag("app"))
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *SecretsSuite) TestSecretRotated(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SecretsRotated")
		c.Check(arg, tc.DeepEquals, params.SecretRotatedArgs{
			Args: []params.SecretRotatedArg{{
				URI:              "secret:9m4e2mr0ui3e8a215n4g",
				OriginalRevision: 666,
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	err := client.SecretRotated(c.Context(), "secret:9m4e2mr0ui3e8a215n4g", 666)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *SecretsSuite) TestWatchSecretRevisionsExpiryChanges(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchSecretRevisionsExpiryChanges")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "application-app"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.SecretTriggerWatchResult{})
		*(result.(*params.SecretTriggerWatchResult)) = params.SecretTriggerWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	_, err := client.WatchSecretRevisionsExpiryChanges(c.Context(), names.NewApplicationTag("app"))
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *SecretsSuite) TestGrant(c *tc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SecretsGrant")
		c.Check(arg, tc.DeepEquals, params.GrantRevokeSecretArgs{
			Args: []params.GrantRevokeSecretArg{{
				URI:         uri.String(),
				ScopeTag:    "relation-wordpress.db#mysql.server",
				SubjectTags: []string{"unit-wordpress-0"},
				Role:        "view",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	err := client.Grant(c.Context(), uri, &secretsmanager.SecretRevokeGrantArgs{
		UnitName:    ptr("wordpress/0"),
		RelationKey: ptr("wordpress:db mysql:server"),
		Role:        coresecrets.RoleView,
	})
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *SecretsSuite) TestRevoke(c *tc.C) {
	uri := coresecrets.NewURI()
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "SecretsManager")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SecretsRevoke")
		c.Check(arg, tc.DeepEquals, params.GrantRevokeSecretArgs{
			Args: []params.GrantRevokeSecretArg{{
				URI:         uri.String(),
				ScopeTag:    "relation-wordpress.db#mysql.server",
				SubjectTags: []string{"application-wordpress"},
				Role:        "view",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	client := secretsmanager.NewClient(apiCaller)
	err := client.Revoke(c.Context(), uri, &secretsmanager.SecretRevokeGrantArgs{
		ApplicationName: ptr("wordpress"),
		RelationKey:     ptr("wordpress:db mysql:server"),
		Role:            coresecrets.RoleView,
	})
	c.Assert(err, tc.ErrorMatches, "FAIL")
}
