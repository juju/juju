// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"context"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apisecrets "github.com/juju/juju/apiserver/facades/client/secrets"
	"github.com/juju/juju/apiserver/facades/client/secrets/mocks"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type SecretsSuite struct {
	testhelpers.IsolationSuite

	authorizer           *facademocks.MockAuthorizer
	authTag              names.Tag
	secretService        *mocks.MockSecretService
	secretBackendService *mocks.MockSecretBackendService
	modelName            string
}

func TestSecretsSuite(t *testing.T) {
	tc.Run(t, &SecretsSuite{})
}

<<<<<<< HEAD
func (s *SecretsSuite) SetUpTest(c *tc.C) {
=======
func backendConfigGetterForUserSecretsWrite(c *gc.C) func(string, []*coresecrets.URI) (*provider.ModelBackendConfigInfo, error) {
	return func(backendID string, _ []*coresecrets.URI) (*provider.ModelBackendConfigInfo, error) {
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
>>>>>>> 3.6
	s.IsolationSuite.SetUpTest(c)

	s.authTag = names.NewUserTag("foo")
	s.modelName = "testmodel"
}

func (s *SecretsSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.secretService = mocks.NewMockSecretService(ctrl)
	s.secretBackendService = mocks.NewMockSecretBackendService(ctrl)
	return ctrl
}

func (s *SecretsSuite) expectAuthClient() {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *SecretsSuite) TestListSecrets(c *tc.C) {
	s.assertListSecrets(c, false)
}

// This test is to verify that when the secret backend name
// is missing from the service layer, we return an error.
func (s *SecretsSuite) TestListSecretsErrNoBackendName(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, coretesting.ModelTag).Return(nil)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	now := time.Now()
	uri := coresecrets.NewURI()
	metadata := []*coresecrets.SecretMetadata{{
		URI:                    uri,
		Version:                1,
		Owner:                  coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"},
		RotatePolicy:           coresecrets.RotateHourly,
		LatestRevision:         2,
		LatestRevisionChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		LatestExpireTime:       new(now),
		NextRotateTime:         new(now.Add(time.Hour)),
		Description:            "shhh",
		Label:                  "foobar",
		CreateTime:             now,
		UpdateTime:             now.Add(time.Second),
	}}

	// Revision backend name should have been populated in the service layer, even for unknowns.
	// If there is no backend name for any revision, we return an rpc error, indicating there is a bug
	// in the service layer.
	revisions := [][]*coresecrets.SecretRevisionMetadata{
		{{
			// Revision backend name should have been populated in the service layer, even for unknowns.
			// If there is no backend name, we return an error.
			Revision:   666,
			CreateTime: now,
			UpdateTime: now.Add(time.Second),
			ExpireTime: new(now.Add(time.Hour)),
		}, {
			// Revision backend name should have been populated in the service layer, even for unknowns.
			// If there is no backend ID, backend name should be set to "<unknown>" to indicate that.
			Revision: 667,
			ValueRef: &coresecrets.ValueRef{
				BackendID: "not-a-valid-backend-id",
			},
			BackendName: new("<unknown>"),
			CreateTime:  now,
			UpdateTime:  now.Add(2 * time.Second),
			ExpireTime:  new(now.Add(2 * time.Hour)),
		}, {
			// Valid backend name returned which will be retained.
			Revision:    668,
			BackendName: new("some backend"),
			CreateTime:  now,
			UpdateTime:  now.Add(2 * time.Second),
			ExpireTime:  new(now.Add(2 * time.Hour)),
		}},
		{},
	}

	s.secretService.EXPECT().ListSecrets(gomock.Any(), nil, secret.NilRevision, secret.NilLabels).Return(
		metadata, revisions, nil,
	)
	s.secretService.EXPECT().GetSecretGrants(gomock.Any(), uri, coresecrets.RoleView).Return([]secretservice.SecretAccess{
		{
			Scope: secret.SecretAccessScope{
				Kind: secret.RelationAccessScope,
				ID:   "gitlab:server mysql:db",
			},
			Subject: secret.SecretAccessor{
				Kind: secret.ApplicationAccessor,
				ID:   "gitlab",
			},
			Role: coresecrets.RoleView,
		},
	}, nil)

	_, err = facade.ListSecrets(c.Context(), params.ListSecretsArgs{ShowSecrets: false})
	c.Assert(err, tc.ErrorMatches, "retrieving secret revision backend name for secret foobar")
}

func (s *SecretsSuite) TestListSecretsReveal(c *tc.C) {
	s.assertListSecrets(c, true)
}

func (s *SecretsSuite) assertListSecrets(c *tc.C, reveal bool) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	if reveal {
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)
	} else {
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, coretesting.ModelTag).Return(nil)
	}

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	now := time.Now()
	uri := coresecrets.NewURI()
	metadata := []*coresecrets.SecretMetadata{{
		URI:                    uri,
		Version:                1,
		Owner:                  coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"},
		RotatePolicy:           coresecrets.RotateHourly,
		LatestRevision:         2,
		LatestRevisionChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
		LatestExpireTime:       new(now),
		NextRotateTime:         new(now.Add(time.Hour)),
		Description:            "shhh",
		Label:                  "foobar",
		CreateTime:             now,
		UpdateTime:             now.Add(time.Second),
	}}
	revisions := [][]*coresecrets.SecretRevisionMetadata{
		{{
			// Revision backend name should have been populated in the service layer, even for unknowns.
			// If there is no backend ID, backend name should be set to "<unknown>" to indicate that.
			Revision:    666,
			BackendName: new("<unknown>"),
			CreateTime:  now,
			UpdateTime:  now.Add(time.Second),
			ExpireTime:  new(now.Add(time.Hour)),
		}, {
			// Revision backend name should have been populated in the service layer, even for unknowns.
			// If there is no backend ID, backend name should be set to "<unknown>" to indicate that.
			Revision: 667,
			ValueRef: &coresecrets.ValueRef{
				BackendID: "not-a-valid-backend-id",
			},
			BackendName: new("<unknown>"),
			CreateTime:  now,
			UpdateTime:  now.Add(2 * time.Second),
			ExpireTime:  new(now.Add(2 * time.Hour)),
		}, {
			// Valid backend name returned which will be retained.
			Revision:    668,
			BackendName: new("some backend"),
			CreateTime:  now,
			UpdateTime:  now.Add(2 * time.Second),
			ExpireTime:  new(now.Add(2 * time.Hour)),
		}, {
			// Backend name kubernetes should be transformed to the built-in name (model_name-local).
			Revision:    669,
			BackendName: new("kubernetes"),
			CreateTime:  now,
			UpdateTime:  now.Add(2 * time.Second),
			ExpireTime:  new(now.Add(2 * time.Hour)),
		}, {
			// Default backend name will be retained.
			Revision:    670,
			ValueRef:    &coresecrets.ValueRef{},
			BackendName: new("internal"),
			CreateTime:  now,
			UpdateTime:  now.Add(2 * time.Second),
			ExpireTime:  new(now.Add(2 * time.Hour)),
		}},
		{},
	}

	s.secretService.EXPECT().ListSecrets(gomock.Any(), nil, secret.NilRevision, secret.NilLabels).Return(
		metadata, revisions, nil,
	)
	s.secretService.EXPECT().GetSecretGrants(gomock.Any(), uri, coresecrets.RoleView).Return([]secretservice.SecretAccess{
		{
			Scope: secret.SecretAccessScope{
				Kind: secret.RelationAccessScope,
				ID:   "gitlab:server mysql:db",
			},
			Subject: secret.SecretAccessor{
				Kind: secret.ApplicationAccessor,
				ID:   "gitlab",
			},
			Role: coresecrets.RoleView,
		},
	}, nil)

	var valueResult *params.SecretValueResult
	if reveal {
		valueResult = &params.SecretValueResult{
			Data: map[string]string{"foo": "bar"},
		}
		s.secretService.EXPECT().GetSecretContentFromBackend(gomock.Any(), uri, 2).Return(
			coresecrets.NewSecretValue(valueResult.Data), nil,
		)
	}

	results, err := facade.ListSecrets(c.Context(), params.ListSecretsArgs{ShowSecrets: reveal})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ListSecretResults{
		Results: []params.ListSecretResult{{
			URI:                    uri.String(),
			Version:                1,
			OwnerTag:               "application-mysql",
			RotatePolicy:           string(coresecrets.RotateHourly),
			LatestExpireTime:       new(now),
			NextRotateTime:         new(now.Add(time.Hour)),
			Description:            "shhh",
			Label:                  "foobar",
			LatestRevision:         2,
			LatestRevisionChecksum: "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b",
			CreateTime:             now,
			UpdateTime:             now.Add(time.Second),
			Value:                  valueResult,
			Revisions: []params.SecretRevision{{
				Revision:    666,
				BackendName: new("<unknown>"),
				CreateTime:  now,
				UpdateTime:  now.Add(time.Second),
				ExpireTime:  new(now.Add(time.Hour)),
			}, {
				Revision:    667,
				BackendName: new("<unknown>"),
				CreateTime:  now,
				UpdateTime:  now.Add(2 * time.Second),
				ExpireTime:  new(now.Add(2 * time.Hour)),
			}, {
				Revision:    668,
				BackendName: new("some backend"),
				CreateTime:  now,
				UpdateTime:  now.Add(2 * time.Second),
				ExpireTime:  new(now.Add(2 * time.Hour)),
			}, {
				Revision:    669,
				BackendName: new("testmodel-local"),
				CreateTime:  now,
				UpdateTime:  now.Add(2 * time.Second),
				ExpireTime:  new(now.Add(2 * time.Hour)),
			}, {
				Revision:    670,
				BackendName: new("internal"),
				CreateTime:  now,
				UpdateTime:  now.Add(2 * time.Second),
				ExpireTime:  new(now.Add(2 * time.Hour)),
			}},
			Access: []params.AccessInfo{
				{TargetTag: "application-gitlab", ScopeTag: "relation-gitlab.server#mysql.db", Role: "view"},
			},
		}},
	})
}

func (s *SecretsSuite) TestListSecretsPermissionDenied(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	_, err = facade.ListSecrets(c.Context(), params.ListSecretsArgs{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestListSecretsPermissionDeniedShow(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	_, err = facade.ListSecrets(c.Context(), params.ListSecretsArgs{ShowSecrets: true})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestCreateSecretsPermissionDenied(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	_, err = facade.CreateSecrets(c.Context(), params.CreateSecretArgs{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestCreateSecretsEmptyData(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	uriStrPtr := new(uri.String())

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	result, err := facade.CreateSecrets(c.Context(), params.CreateSecretArgs{
		Args: []params.CreateSecretArg{
			{
				OwnerTag: coretesting.ModelTag.Id(),
				URI:      uriStrPtr,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error.Message, tc.DeepEquals, "empty secret value not valid")
}

func (s *SecretsSuite) TestCreateSecrets(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(nil)

<<<<<<< HEAD
	uri := coresecrets.NewURI()
	uriStrPtr := new(uri.String())
	s.secretService.EXPECT().CreateUserSecret(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg1 *coresecrets.URI, params secretservice.CreateUserSecretParams) error {
		c.Assert(arg1, tc.DeepEquals, uri)
		c.Assert(params.Version, tc.Equals, 1)
		c.Assert(params.UpdateUserSecretParams.Description, tc.DeepEquals, new("this is a user secret."))
		c.Assert(params.UpdateUserSecretParams.Label, tc.DeepEquals, new("label"))
		c.Assert(params.UpdateUserSecretParams.Data, tc.DeepEquals, coresecrets.SecretData(map[string]string{"foo": "bar"}))
		c.Assert(params.UpdateUserSecretParams.Checksum, tc.Equals, "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b")
		return nil
=======
	var uri *coresecrets.URI
	s.secretsState.EXPECT().ReserveSecret(
		gomock.Any(), coretesting.ModelTag,
	).DoAndReturn(func(arg1 *coresecrets.URI, owner names.Tag) error {
		uri = arg1
		return nil
	})

	if isInternal {
		s.secretsBackend.EXPECT().SaveContent(gomock.Any(), gomock.Any(), 1, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
			Return("", errors.NotSupportedf("not supported"))
	} else {
		s.secretsBackend.EXPECT().SaveContent(gomock.Any(), gomock.Any(), 1, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
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
>>>>>>> 3.6
	})
	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	result, err := facade.CreateSecrets(c.Context(), params.CreateSecretArgs{
		Args: []params.CreateSecretArg{
			{
				OwnerTag: coretesting.ModelTag.Id(),
				UpsertSecretArg: params.UpsertSecretArg{
					Description: new("this is a user secret."),
					Label:       new("label"),
					Content: params.SecretContentParams{
						Data: map[string]string{"foo": "bar"},
					},
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0], tc.DeepEquals, params.StringResult{Result: uri.String()})
}

func (s *SecretsSuite) assertUpdateSecrets(c *tc.C, uri *coresecrets.URI) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(nil)

	var (
		uriString, existingLabel string
	)
	if uri == nil {
		existingLabel = "my-secret"
		uri = coresecrets.NewURI()
<<<<<<< HEAD
		s.secretService.EXPECT().GetUserSecretURIByLabel(gomock.Any(), "my-secret").Return(uri, nil)
=======
		s.secretsState.EXPECT().ListSecrets(state.SecretsFilter{
			Labels:    []string{"my-secret"},
			OwnerTags: []names.Tag{coretesting.ModelTag},
		}).Return([]*coresecrets.SecretMetadata{{
			URI: uri,
		}}, nil)
>>>>>>> 3.6
	} else {
		uriString = uri.String()
	}
	s.secretService.EXPECT().UpdateUserSecret(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg1 *coresecrets.URI, params secretservice.UpdateUserSecretParams) error {
		c.Assert(arg1, tc.DeepEquals, uri)
		c.Assert(params.Description, tc.DeepEquals, new("this is a user secret."))
		c.Assert(params.Label, tc.DeepEquals, new("label"))
		c.Assert(params.AutoPrune, tc.DeepEquals, new(true))
		c.Assert(params.Data, tc.DeepEquals, coresecrets.SecretData(map[string]string{"foo": "bar"}))
		c.Assert(params.Checksum, tc.Equals, "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b")
		return nil
	})
	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	result, err := facade.UpdateSecrets(c.Context(), params.UpdateUserSecretArgs{
		Args: []params.UpdateUserSecretArg{
			{
				AutoPrune:     new(true),
				URI:           uriString,
				ExistingLabel: existingLabel,
				UpsertSecretArg: params.UpsertSecretArg{
					Description: new("this is a user secret."),
					Label:       new("label"),
					Content: params.SecretContentParams{
						Data: map[string]string{"foo": "bar"},
					},
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *SecretsSuite) TestUpdateSecrets(c *tc.C) {
	s.assertUpdateSecrets(c, coresecrets.NewURI())
}

func (s *SecretsSuite) TestUpdateSecretsByName(c *tc.C) {
	s.assertUpdateSecrets(c, nil)
}

func (s *SecretsSuite) TestRemoveSecrets(c *tc.C) {
	defer s.setup(c).Finish()
	s.expectAuthClient()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(nil)
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secret.DeleteSecretParams{
		Accessor:  secret.SecretAccessor{Kind: secret.ModelAccessor, ID: coretesting.ModelTag.Id()},
		Revisions: []int{666},
	}).Return(nil)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)
	results, err := facade.RemoveSecrets(c.Context(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *SecretsSuite) TestRemoveSecretsFailedNotModelAdmin(c *tc.C) {
	defer s.setup(c).Finish()
	s.expectAuthClient()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(apiservererrors.ErrPerm)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)
	_, err = facade.RemoveSecrets(c.Context(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestRemoveSecretRevision(c *tc.C) {
	defer s.setup(c).Finish()
	s.expectAuthClient()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(nil)
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secret.DeleteSecretParams{
		Accessor:  secret.SecretAccessor{Kind: secret.ModelAccessor, ID: coretesting.ModelTag.Id()},
		Revisions: []int{666},
	}).Return(nil)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)
	results, err := facade.RemoveSecrets(c.Context(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *SecretsSuite) TestRemoveSecretNotFound(c *tc.C) {
	defer s.setup(c).Finish()
	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secret.DeleteSecretParams{
		Accessor:  secret.SecretAccessor{Kind: secret.ModelAccessor, ID: coretesting.ModelTag.Id()},
		Revisions: []int{666},
	}).Return(secreterrors.SecretNotFound)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)
	results, err := facade.RemoveSecrets(c.Context(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.Satisfies, params.IsCodeSecretNotFound)
}

func (s *SecretsSuite) TestGrantSecret(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secret.SecretAccessParams) error {
			c.Assert(arg, tc.DeepEquals, uri)
			c.Assert(params.Scope, tc.DeepEquals,
				secret.SecretAccessScope{Kind: secret.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, tc.DeepEquals,
				secret.SecretAccessor{Kind: secret.ApplicationAccessor, ID: "gitlab"})
			c.Assert(params.Role, tc.Equals, coresecrets.RoleView)
			return nil
		},
	)
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secret.SecretAccessParams) error {
			c.Assert(arg, tc.DeepEquals, uri)
			c.Assert(params.Scope, tc.DeepEquals,
				secret.SecretAccessScope{Kind: secret.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, tc.DeepEquals,
				secret.SecretAccessor{Kind: secret.ApplicationAccessor, ID: "mysql"})
			c.Assert(params.Role, tc.Equals, coresecrets.RoleView)
			return nil
		},
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	result, err := facade.GrantSecret(c.Context(), params.GrantRevokeUserSecretArg{
		URI: uri.String(),
		Applications: []string{
			"gitlab", "mysql",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}, {Error: nil}}})
}

func (s *SecretsSuite) TestGrantSecretByName(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
<<<<<<< HEAD
	s.secretService.EXPECT().GetUserSecretURIByLabel(gomock.Any(), "my-secret").Return(uri, nil)
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secret.SecretAccessParams) error {
			c.Assert(arg, tc.DeepEquals, uri)
			c.Assert(params.Scope, tc.DeepEquals,
				secret.SecretAccessScope{Kind: secret.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, tc.DeepEquals,
				secret.SecretAccessor{Kind: secret.ApplicationAccessor, ID: "gitlab"})
			c.Assert(params.Role, tc.Equals, coresecrets.RoleView)
=======
	s.secretsState.EXPECT().ListSecrets(state.SecretsFilter{
		Labels:    []string{"my-secret"},
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
>>>>>>> 3.6
			return nil
		},
	)
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secret.SecretAccessParams) error {
			c.Assert(arg, tc.DeepEquals, uri)
			c.Assert(params.Scope, tc.DeepEquals,
				secret.SecretAccessScope{Kind: secret.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, tc.DeepEquals,
				secret.SecretAccessor{Kind: secret.ApplicationAccessor, ID: "mysql"})
			c.Assert(params.Role, tc.Equals, coresecrets.RoleView)
			return nil
		},
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	result, err := facade.GrantSecret(c.Context(), params.GrantRevokeUserSecretArg{
		Label: "my-secret",
		Applications: []string{
			"gitlab", "mysql",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}, {Error: nil}}})
}

func (s *SecretsSuite) TestGrantSecretPermissionDenied(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission),
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	_, err = facade.GrantSecret(c.Context(), params.GrantRevokeUserSecretArg{Label: "my-secret"})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestRevokeSecret(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().RevokeSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secret.SecretAccessParams) error {
			c.Assert(arg, tc.DeepEquals, uri)
			c.Assert(params.Scope, tc.DeepEquals,
				secret.SecretAccessScope{Kind: secret.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, tc.DeepEquals,
				secret.SecretAccessor{Kind: secret.ApplicationAccessor, ID: "gitlab"})
			c.Assert(params.Role, tc.Equals, coresecrets.RoleView)
			return nil
		},
	)
	s.secretService.EXPECT().RevokeSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secret.SecretAccessParams) error {
			c.Assert(arg, tc.DeepEquals, uri)
			c.Assert(params.Scope, tc.DeepEquals,
				secret.SecretAccessScope{Kind: secret.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, tc.DeepEquals,
				secret.SecretAccessor{Kind: secret.ApplicationAccessor, ID: "mysql"})
			c.Assert(params.Role, tc.Equals, coresecrets.RoleView)
			return nil
		},
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	result, err := facade.RevokeSecret(c.Context(), params.GrantRevokeUserSecretArg{
		URI: uri.String(),
		Applications: []string{
			"gitlab", "mysql",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}, {Error: nil}}})
}

func (s *SecretsSuite) TestRevokeSecretPermissionDenied(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission),
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService, s.modelName)
	c.Assert(err, tc.ErrorIsNil)

	_, err = facade.RevokeSecret(c.Context(), params.GrantRevokeUserSecretArg{Label: "my-secret"})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}
