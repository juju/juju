// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apisecrets "github.com/juju/juju/apiserver/facades/client/secrets"
	"github.com/juju/juju/apiserver/facades/client/secrets/mocks"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type SecretsSuite struct {
	testing.IsolationSuite

	authorizer     *facademocks.MockAuthorizer
	authTag        names.Tag
	provider       *mocks.MockSecretBackendProvider
	backend        *mocks.MockSecretsBackend
	secretsState   *mocks.MockSecretsState
	secretConsumer *mocks.MockSecretsConsumer
	secretsBackend *mocks.MockSecretsBackend
}

var _ = gc.Suite(&SecretsSuite{})

func adminBackendConfigGetter() (*provider.ModelBackendConfigInfo, error) {
	return &provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "some-model",
				BackendConfig: provider.BackendConfig{
					BackendType: "active-type",
					Config:      map[string]interface{}{"foo": "active-type"},
				},
			},
			"other-backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "some-model",
				BackendConfig: provider.BackendConfig{
					BackendType: "other-type",
					Config:      map[string]interface{}{"foo": "other-type"},
				},
			},
		},
	}, nil
}

func backendConfigGetterForUserSecretsWrite(c *gc.C) func(backendID string) (*provider.ModelBackendConfigInfo, error) {
	return func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		c.Assert(backendID, gc.Equals, "backend-id")
		return &provider.ModelBackendConfigInfo{
			ActiveID: "backend-id",
			Configs: map[string]provider.ModelBackendConfig{
				"backend-id": {
					ControllerUUID: coretesting.ControllerTag.Id(),
					ModelUUID:      coretesting.ModelTag.Id(),
					ModelName:      "some-model",
					BackendConfig: provider.BackendConfig{
						BackendType: "active-type",
						Config:      map[string]interface{}{"foo": "active-type"},
					},
				},
			},
		}, nil
	}
}

func (s *SecretsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.authTag = names.NewUserTag("foo")
}

func (s *SecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.secretsState = mocks.NewMockSecretsState(ctrl)
	s.secretConsumer = mocks.NewMockSecretsConsumer(ctrl)
	s.secretsBackend = mocks.NewMockSecretsBackend(ctrl)
	s.provider = mocks.NewMockSecretBackendProvider(ctrl)
	s.backend = mocks.NewMockSecretsBackend(ctrl)
	s.PatchValue(&commonsecrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return s.provider, nil })
	return ctrl
}

func (s *SecretsSuite) expectAuthClient() {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *SecretsSuite) TestListSecrets(c *gc.C) {
	s.assertListSecrets(c, false, false)
}

func (s *SecretsSuite) TestListSecretsReveal(c *gc.C) {
	s.assertListSecrets(c, true, false)
}

func (s *SecretsSuite) TestListSecretsRevealFromBackend(c *gc.C) {
	s.assertListSecrets(c, true, true)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) assertListSecrets(c *gc.C, reveal, withBackend bool) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	if reveal {
		s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)
	} else {
		s.authorizer.EXPECT().HasPermission(permission.ReadAccess, coretesting.ModelTag).Return(nil)
	}

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	now := time.Now()
	uri := coresecrets.NewURI()
	metadata := []*coresecrets.SecretMetadata{{
		URI:                    uri,
		Version:                1,
		OwnerTag:               "application-mysql",
		RotatePolicy:           coresecrets.RotateHourly,
		LatestRevision:         2,
		LatestRevisionChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		LatestExpireTime:       ptr(now),
		NextRotateTime:         ptr(now.Add(time.Hour)),
		Description:            "shhh",
		Label:                  "foobar",
		CreateTime:             now,
		UpdateTime:             now.Add(time.Second),
	}}
	revisions := []*coresecrets.SecretRevisionMetadata{{
		Revision:   666,
		CreateTime: now,
		UpdateTime: now.Add(time.Second),
		ExpireTime: ptr(now.Add(time.Hour)),
	}, {
		Revision:    667,
		BackendName: ptr("some backend"),
		CreateTime:  now,
		UpdateTime:  now.Add(2 * time.Second),
		ExpireTime:  ptr(now.Add(2 * time.Hour)),
	}}
	s.secretsState.EXPECT().ListSecrets(state.SecretsFilter{}).Return(
		metadata, nil,
	)
	s.secretsState.EXPECT().SecretGrants(uri, coresecrets.RoleView).Return([]coresecrets.AccessInfo{
		{
			Target: "application-gitlab",
			Scope:  "relation-key",
			Role:   coresecrets.RoleView,
		},
	}, nil)
	s.secretsState.EXPECT().ListSecretRevisions(uri).Return(
		revisions, nil,
	)

	var valueResult *params.SecretValueResult
	if reveal {
		valueResult = &params.SecretValueResult{
			Data: map[string]string{"foo": "bar"},
		}
		if withBackend {
			s.secretsState.EXPECT().GetSecretValue(uri, 2).Return(
				nil, &coresecrets.ValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				}, nil,
			)
			s.secretsBackend.EXPECT().GetContent(gomock.Any(), "rev-id").Return(
				coresecrets.NewSecretValue(valueResult.Data), nil,
			)
		} else {
			s.secretsState.EXPECT().GetSecretValue(uri, 2).Return(
				coresecrets.NewSecretValue(valueResult.Data), nil, nil,
			)
		}
	}

	results, err := facade.ListSecrets(params.ListSecretsArgs{ShowSecrets: reveal})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ListSecretResults{
		Results: []params.ListSecretResult{{
			URI:                    uri.String(),
			Version:                1,
			OwnerTag:               "application-mysql",
			RotatePolicy:           string(coresecrets.RotateHourly),
			LatestExpireTime:       ptr(now),
			NextRotateTime:         ptr(now.Add(time.Hour)),
			Description:            "shhh",
			Label:                  "foobar",
			LatestRevision:         2,
			LatestRevisionChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
			CreateTime:             now,
			UpdateTime:             now.Add(time.Second),
			Value:                  valueResult,
			Revisions: []params.SecretRevision{{
				Revision:    666,
				BackendName: ptr("internal"),
				CreateTime:  now,
				UpdateTime:  now.Add(time.Second),
				ExpireTime:  ptr(now.Add(time.Hour)),
			}, {
				Revision:    667,
				BackendName: ptr("some backend"),
				CreateTime:  now,
				UpdateTime:  now.Add(2 * time.Second),
				ExpireTime:  ptr(now.Add(2 * time.Hour)),
			}},
			Access: []params.AccessInfo{
				{TargetTag: "application-gitlab", ScopeTag: "relation-key", Role: "view"},
			},
		}},
	})
}

func (s *SecretsSuite) TestListSecretsPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.ReadAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.ListSecrets(params.ListSecretsArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestListSecretsPermissionDeniedShow(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))
	s.authorizer.EXPECT().HasPermission(permission.AdminAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.ListSecrets(params.ListSecretsArgs{ShowSecrets: true})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestCreateSecretsPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.CreateSecrets(params.CreateSecretArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestCreateSecretsEmptyData(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	uriStrPtr := ptr(uri.String())

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		})
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.CreateSecrets(params.CreateSecretArgs{
		Args: []params.CreateSecretArg{
			{
				OwnerTag: coretesting.ModelTag.Id(),
				URI:      uriStrPtr,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error.Message, gc.DeepEquals, "empty secret value not valid")
}

func (s *SecretsSuite) assertCreateSecrets(c *gc.C, isInternal bool, finalStepFailed bool) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	uriStrPtr := ptr(uri.String())
	if isInternal {
		s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
			Return("", errors.NotSupportedf("not supported"))
	} else {
		s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
			Return("rev-id", nil)
	}
	s.secretsState.EXPECT().CreateSecret(gomock.Any(), gomock.Any()).DoAndReturn(func(arg1 *coresecrets.URI, params state.CreateSecretParams) (*coresecrets.SecretMetadata, error) {
		c.Assert(arg1, gc.DeepEquals, uri)
		c.Assert(params.Version, gc.Equals, 1)
		c.Assert(params.Owner, gc.Equals, coretesting.ModelTag)
		c.Assert(params.UpdateSecretParams.Description, gc.DeepEquals, ptr("this is a user secret."))
		c.Assert(params.UpdateSecretParams.Label, gc.DeepEquals, ptr("label"))
		if isInternal {
			c.Assert(params.UpdateSecretParams.ValueRef, gc.IsNil)
			c.Assert(params.UpdateSecretParams.Data, gc.DeepEquals, coresecrets.SecretData(map[string]string{"foo": "bar"}))
		} else {
			c.Assert(params.UpdateSecretParams.ValueRef, gc.DeepEquals, &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			})
			c.Assert(params.UpdateSecretParams.Data, gc.IsNil)
		}
		c.Assert(params.UpdateSecretParams.Checksum, gc.Equals, "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b")
		if finalStepFailed {
			return nil, errors.New("some error")
		}
		return &coresecrets.SecretMetadata{URI: uri}, nil
	})
	if !finalStepFailed {
		s.secretConsumer.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any()).DoAndReturn(func(arg1 *coresecrets.URI, params state.SecretAccessParams) error {
			c.Assert(arg1, gc.DeepEquals, uri)
			c.Assert(params.Scope, gc.Equals, coretesting.ModelTag)
			c.Assert(params.Subject, gc.Equals, coretesting.ModelTag)
			c.Assert(params.Role, gc.Equals, coresecrets.RoleManage)
			return nil
		})
	}
	if finalStepFailed && !isInternal {
		s.secretsBackend.EXPECT().DeleteContent(gomock.Any(), "rev-id").Return(nil)
	}
	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		})
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.CreateSecrets(params.CreateSecretArgs{
		Args: []params.CreateSecretArg{
			{
				OwnerTag: coretesting.ModelTag.Id(),
				URI:      uriStrPtr,
				UpsertSecretArg: params.UpsertSecretArg{
					Description: ptr("this is a user secret."),
					Label:       ptr("label"),
					Content: params.SecretContentParams{
						Data: map[string]string{"foo": "bar"},
					},
				},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	if finalStepFailed {
		c.Assert(result.Results[0].Error.Message, gc.DeepEquals, "some error")
	} else {
		c.Assert(result.Results[0], gc.DeepEquals, params.StringResult{Result: uri.String()})
	}
}

func (s *SecretsSuite) TestCreateSecretsExternalBackend(c *gc.C) {
	s.assertCreateSecrets(c, false, false)
}

func (s *SecretsSuite) TestCreateSecretsExternalBackendFailedAndCleanup(c *gc.C) {
	s.assertCreateSecrets(c, false, true)
}

func (s *SecretsSuite) TestCreateSecretsInternalBackend(c *gc.C) {
	s.assertCreateSecrets(c, true, false)
}

func (s *SecretsSuite) assertUpdateSecrets(c *gc.C, uri *coresecrets.URI, isInternal bool, currentChecksum string, finalStepFailed bool) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

	var (
		uriString, existingLabel string
	)
	if uri == nil {
		existingLabel = "my-secret"
		uri = coresecrets.NewURI()
		s.secretsState.EXPECT().ListSecrets(state.SecretsFilter{
			Label:     ptr("my-secret"),
			OwnerTags: []names.Tag{coretesting.ModelTag},
		}).Return([]*coresecrets.SecretMetadata{{
			URI: uri,
		}}, nil)
	} else {
		uriString = uri.String()
	}
	s.secretsState.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		URI:                    uri,
		LatestRevision:         2,
		LatestRevisionChecksum: currentChecksum,
	}, nil)
	differentChecksums := currentChecksum != "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b"
	if differentChecksums {
		if isInternal {
			s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 3, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
				Return("", errors.NotSupportedf("not supported"))
		} else {
			s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 3, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
				Return("rev-id", nil)
		}
	}
	s.secretsState.EXPECT().UpdateSecret(gomock.Any(), gomock.Any()).DoAndReturn(func(arg1 *coresecrets.URI, params state.UpdateSecretParams) (*coresecrets.SecretMetadata, error) {
		c.Assert(arg1, gc.DeepEquals, uri)
		c.Assert(params.Description, gc.DeepEquals, ptr("this is a user secret."))
		c.Assert(params.Label, gc.DeepEquals, ptr("label"))
		c.Assert(params.AutoPrune, gc.DeepEquals, ptr(true))
		if differentChecksums {
			if isInternal {
				c.Assert(params.ValueRef, gc.IsNil)
				c.Assert(params.Data, gc.DeepEquals, coresecrets.SecretData(map[string]string{"foo": "bar"}))
			} else {
				c.Assert(params.ValueRef, gc.DeepEquals, &coresecrets.ValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				})
				c.Assert(params.Data, gc.IsNil)
			}
			c.Assert(params.Checksum, gc.Equals, "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b")
		}
		if finalStepFailed {
			return nil, errors.New("some error")
		}
		result := &coresecrets.SecretMetadata{URI: uri, LatestRevision: 3}
		if params.AutoPrune != nil {
			result.AutoPrune = *params.AutoPrune
		}
		return result, nil
	})
	if finalStepFailed && !isInternal {
		s.secretsBackend.EXPECT().DeleteContent(gomock.Any(), "rev-id").Return(nil)
	}
	if !finalStepFailed {
		s.secretsState.EXPECT().ListUnusedSecretRevisions(uri).Return([]int{1, 2}, nil)
		// Prune the unused revisions.
		s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)
		s.secretsState.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{URI: uri, OwnerTag: coretesting.ModelTag.String()}, nil).Times(2)
		s.secretsState.EXPECT().GetSecretRevision(uri, 1).Return(&coresecrets.SecretRevisionMetadata{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "rev-1"},
		}, nil)
		s.secretsState.EXPECT().GetSecretRevision(uri, 2).Return(&coresecrets.SecretRevisionMetadata{
			Revision: 2,
			ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "rev-2"},
		}, nil)

		cfg := &provider.ModelBackendConfig{
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      coretesting.ModelTag.Id(),
			ModelName:      "some-model",
			BackendConfig: provider.BackendConfig{
				BackendType: "active-type",
				Config:      map[string]interface{}{"foo": "active-type"},
			},
		}
		s.provider.EXPECT().NewBackend(cfg).Return(s.backend, nil)
		s.backend.EXPECT().DeleteContent(gomock.Any(), "rev-1").Return(nil)
		s.backend.EXPECT().DeleteContent(gomock.Any(), "rev-2").Return(nil)
		s.provider.EXPECT().CleanupSecrets(
			cfg, coretesting.ModelTag,
			provider.SecretRevisions{uri.ID: set.NewStrings("rev-1", "rev-2")},
		).Return(nil)

		s.secretsState.EXPECT().DeleteSecret(uri, []int{1, 2}).Return(nil, nil)
	}
	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		})
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.UpdateSecrets(params.UpdateUserSecretArgs{
		Args: []params.UpdateUserSecretArg{
			{
				AutoPrune:     ptr(true),
				URI:           uriString,
				ExistingLabel: existingLabel,
				UpsertSecretArg: params.UpsertSecretArg{
					Description: ptr("this is a user secret."),
					Label:       ptr("label"),
					Content: params.SecretContentParams{
						Data: map[string]string{"foo": "bar"},
					},
				},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	if finalStepFailed {
		c.Assert(result.Results[0].Error.Message, gc.DeepEquals, "some error")
	} else {
		c.Assert(result.Results[0].Error, gc.IsNil)
	}
}

func (s *SecretsSuite) TestUpdateSecretsExternalBackend(c *gc.C) {
	s.assertUpdateSecrets(c, coresecrets.NewURI(), false, "deadbeef", false)
}

func (s *SecretsSuite) TestUpdateSecretsExternalBackendFailedAndCleanup(c *gc.C) {
	s.assertUpdateSecrets(c, coresecrets.NewURI(), false, "deadbeef", true)
}

func (s *SecretsSuite) TestUpdateSecretsInternalBackend(c *gc.C) {
	s.assertUpdateSecrets(c, coresecrets.NewURI(), true, "deadbeef", false)
}

func (s *SecretsSuite) TestUpdateSecretsByName(c *gc.C) {
	s.assertUpdateSecrets(c, nil, true, "deadbeef", false)
}

func (s *SecretsSuite) TestUpdateSecretsSameChecksum(c *gc.C) {
	s.assertUpdateSecrets(c, coresecrets.NewURI(), true, "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b", false)
}

func (s *SecretsSuite) TestUpdateSecretJustContent(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	uriString := uri.String()

	s.secretsState.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		URI:                    uri,
		LatestRevision:         2,
		LatestRevisionChecksum: "deadbeef",
	}, nil)
	s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 3, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
		Return("rev-id", nil)
	s.secretsState.EXPECT().UpdateSecret(gomock.Any(), gomock.Any()).DoAndReturn(func(arg1 *coresecrets.URI, params state.UpdateSecretParams) (*coresecrets.SecretMetadata, error) {
		c.Assert(arg1, gc.DeepEquals, uri)
		c.Assert(params.ValueRef, gc.DeepEquals, &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		})
		c.Assert(params.Data, gc.IsNil)
		c.Assert(params.Checksum, gc.Equals, "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b")
		result := &coresecrets.SecretMetadata{URI: uri, LatestRevision: 3}
		if params.AutoPrune != nil {
			result.AutoPrune = *params.AutoPrune
		}
		return result, nil
	})

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		})
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.UpdateSecrets(params.UpdateUserSecretArgs{
		Args: []params.UpdateUserSecretArg{
			{
				URI: uriString,
				UpsertSecretArg: params.UpsertSecretArg{
					Content: params.SecretContentParams{
						Data: map[string]string{"foo": "bar"},
					},
				},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *SecretsSuite) TestRemoveSecrets(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectAuthClient()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)
	s.secretsState.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{URI: uri, OwnerTag: coretesting.ModelTag.String()}, nil).Times(2)
	s.secretsState.EXPECT().GetSecretRevision(&expectURI, 666).Return(&coresecrets.SecretRevisionMetadata{
		Revision: 666,
		ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "rev-666"},
	}, nil)
	s.secretsState.EXPECT().DeleteSecret(&expectURI, []int{666}).Return([]coresecrets.ValueRef{{
		BackendID:  "backend-id",
		RevisionID: "rev-666",
	}}, nil)

	cfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "some-model",
		BackendConfig: provider.BackendConfig{
			BackendType: "active-type",
			Config:      map[string]interface{}{"foo": "active-type"},
		},
	}
	s.provider.EXPECT().NewBackend(cfg).Return(s.backend, nil)
	s.backend.EXPECT().DeleteContent(gomock.Any(), "rev-666").Return(nil)
	s.provider.EXPECT().CleanupSecrets(
		cfg, coretesting.ModelTag,
		provider.SecretRevisions{uri.ID: set.NewStrings("rev-666")},
	).Return(nil)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		})
	c.Assert(err, jc.ErrorIsNil)
	results, err := facade.RemoveSecrets(params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *SecretsSuite) TestRemoveSecretsFailedNotModelAdmin(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectAuthClient()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(apiservererrors.ErrPerm)
	s.secretsState.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{URI: uri, OwnerTag: names.NewModelTag("1cfde5b3-663d-47bf-8799-71b84fa2df3f").String()}, nil).Times(1)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		})
	c.Assert(err, jc.ErrorIsNil)
	results, err := facade.RemoveSecrets(params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error.Message, jc.DeepEquals, "permission denied")
}

func (s *SecretsSuite) TestRemoveSecretsFailedNotModelOwned(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectAuthClient()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)
	s.secretsState.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{URI: uri, OwnerTag: names.NewModelTag("1cfde5b3-663d-47bf-8799-71b84fa2df3f").String()}, nil).Times(2)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		})
	c.Assert(err, jc.ErrorIsNil)
	results, err := facade.RemoveSecrets(params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error.Message, jc.DeepEquals, "permission denied")
}

func (s *SecretsSuite) TestRemoveSecretRevision(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectAuthClient()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)
	s.secretsState.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{URI: uri, OwnerTag: coretesting.ModelTag.String()}, nil).Times(2)
	s.secretsState.EXPECT().GetSecretRevision(&expectURI, 666).Return(&coresecrets.SecretRevisionMetadata{
		Revision: 666,
		ValueRef: &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "rev-666"},
	}, nil)
	s.secretsState.EXPECT().DeleteSecret(&expectURI, []int{666}).Return(nil, nil)
	cfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "some-model",
		BackendConfig: provider.BackendConfig{
			BackendType: "active-type",
			Config:      map[string]interface{}{"foo": "active-type"},
		},
	}
	s.provider.EXPECT().NewBackend(cfg).Return(s.backend, nil)
	s.backend.EXPECT().DeleteContent(gomock.Any(), "rev-666").Return(nil)
	s.provider.EXPECT().CleanupSecrets(
		cfg, coretesting.ModelTag,
		provider.SecretRevisions{uri.ID: set.NewStrings("rev-666")},
	).Return(nil)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		})
	c.Assert(err, jc.ErrorIsNil)
	results, err := facade.RemoveSecrets(params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *SecretsSuite) TestRemoveSecretNotFound(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectAuthClient()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretsState.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{}, errors.NotFoundf("not found"))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		})
	c.Assert(err, jc.ErrorIsNil)
	results, err := facade.RemoveSecrets(params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *SecretsSuite) TestGrantSecret(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	s.secretConsumer.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any()).DoAndReturn(
		func(arg *coresecrets.URI, params state.SecretAccessParams) error {
			c.Assert(arg, gc.DeepEquals, uri)
			c.Assert(params.Scope, gc.Equals, coretesting.ModelTag)
			c.Assert(params.Subject, gc.Equals, names.NewApplicationTag("gitlab"))
			c.Assert(params.Role, gc.Equals, coresecrets.RoleView)
			return nil
		},
	)
	s.secretConsumer.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any()).DoAndReturn(
		func(arg *coresecrets.URI, params state.SecretAccessParams) error {
			c.Assert(arg, gc.DeepEquals, uri)
			c.Assert(params.Scope, gc.Equals, coretesting.ModelTag)
			c.Assert(params.Subject, gc.Equals, names.NewApplicationTag("mysql"))
			c.Assert(params.Role, gc.Equals, coresecrets.RoleView)
			return nil
		},
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		})
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.GrantSecret(params.GrantRevokeUserSecretArg{
		URI: uri.String(),
		Applications: []string{
			"gitlab", "mysql",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}, {Error: nil}}})
}

func (s *SecretsSuite) TestGrantSecretByName(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	s.secretsState.EXPECT().ListSecrets(state.SecretsFilter{
		Label:     ptr("my-secret"),
		OwnerTags: []names.Tag{coretesting.ModelTag},
	}).Return([]*coresecrets.SecretMetadata{{
		URI: uri,
	}}, nil)
	s.secretConsumer.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any()).DoAndReturn(
		func(arg *coresecrets.URI, params state.SecretAccessParams) error {
			c.Assert(arg, gc.DeepEquals, uri)
			c.Assert(params.Scope, gc.Equals, coretesting.ModelTag)
			c.Assert(params.Subject, gc.Equals, names.NewApplicationTag("gitlab"))
			c.Assert(params.Role, gc.Equals, coresecrets.RoleView)
			return nil
		},
	)
	s.secretConsumer.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any()).DoAndReturn(
		func(arg *coresecrets.URI, params state.SecretAccessParams) error {
			c.Assert(arg, gc.DeepEquals, uri)
			c.Assert(params.Scope, gc.Equals, coretesting.ModelTag)
			c.Assert(params.Subject, gc.Equals, names.NewApplicationTag("mysql"))
			c.Assert(params.Role, gc.Equals, coresecrets.RoleView)
			return nil
		},
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		})
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.GrantSecret(params.GrantRevokeUserSecretArg{
		Label: "my-secret",
		Applications: []string{
			"gitlab", "mysql",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}, {Error: nil}}})
}

func (s *SecretsSuite) TestGrantSecretPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission),
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.GrantSecret(params.GrantRevokeUserSecretArg{Label: "my-secret"})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestRevokeSecret(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	s.secretConsumer.EXPECT().RevokeSecretAccess(gomock.Any(), gomock.Any()).DoAndReturn(
		func(arg *coresecrets.URI, params state.SecretAccessParams) error {
			c.Assert(arg, gc.DeepEquals, uri)
			c.Assert(params.Scope, gc.Equals, coretesting.ModelTag)
			c.Assert(params.Subject, gc.Equals, names.NewApplicationTag("gitlab"))
			c.Assert(params.Role, gc.Equals, coresecrets.RoleView)
			return nil
		},
	)
	s.secretConsumer.EXPECT().RevokeSecretAccess(gomock.Any(), gomock.Any()).DoAndReturn(
		func(arg *coresecrets.URI, params state.SecretAccessParams) error {
			c.Assert(arg, gc.DeepEquals, uri)
			c.Assert(params.Scope, gc.Equals, coretesting.ModelTag)
			c.Assert(params.Subject, gc.Equals, names.NewApplicationTag("mysql"))
			c.Assert(params.Role, gc.Equals, coresecrets.RoleView)
			return nil
		},
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer,
		adminBackendConfigGetter, backendConfigGetterForUserSecretsWrite(c),
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		})
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.RevokeSecret(params.GrantRevokeUserSecretArg{
		URI: uri.String(),
		Applications: []string{
			"gitlab", "mysql",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}, {Error: nil}}})
}

func (s *SecretsSuite) TestRevokeSecretPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission),
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretsState, s.secretConsumer, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.RevokeSecret(params.GrantRevokeUserSecretArg{Label: "my-secret"})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
