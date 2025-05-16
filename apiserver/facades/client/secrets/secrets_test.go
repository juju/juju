// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"context"
	stdtesting "testing"
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
}

func TestSecretsSuite(t *stdtesting.T) { tc.Run(t, &SecretsSuite{}) }
func (s *SecretsSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.authTag = names.NewUserTag("foo")
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

func (s *SecretsSuite) TestListSecretsReveal(c *tc.C) {
	s.assertListSecrets(c, true)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) assertListSecrets(c *tc.C, reveal bool) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	if reveal {
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)
	} else {
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, coretesting.ModelTag).Return(nil)
	}

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
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
		LatestExpireTime:       ptr(now),
		NextRotateTime:         ptr(now.Add(time.Hour)),
		Description:            "shhh",
		Label:                  "foobar",
		CreateTime:             now,
		UpdateTime:             now.Add(time.Second),
	}}
	revisions := [][]*coresecrets.SecretRevisionMetadata{
		{{
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
		}},
	}

	s.secretService.EXPECT().ListSecrets(gomock.Any(), nil, secret.NilRevision, secret.NilLabels).Return(
		metadata, revisions, nil,
	)
	s.secretService.EXPECT().GetSecretGrants(gomock.Any(), uri, coresecrets.RoleView).Return([]secretservice.SecretAccess{
		{
			Scope: secretservice.SecretAccessScope{
				Kind: secretservice.RelationAccessScope,
				ID:   "gitlab:server mysql:db",
			},
			Subject: secretservice.SecretAccessor{
				Kind: secretservice.ApplicationAccessor,
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

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
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

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, tc.ErrorIsNil)

	_, err = facade.ListSecrets(c.Context(), params.ListSecretsArgs{ShowSecrets: true})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestCreateSecretsPermissionDenied(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, tc.ErrorIsNil)

	_, err = facade.CreateSecrets(c.Context(), params.CreateSecretArgs{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestCreateSecretsEmptyData(c *tc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	uriStrPtr := ptr(uri.String())

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
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

	uri := coresecrets.NewURI()
	uriStrPtr := ptr(uri.String())
	s.secretService.EXPECT().CreateUserSecret(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg1 *coresecrets.URI, params secretservice.CreateUserSecretParams) error {
		c.Assert(arg1, tc.DeepEquals, uri)
		c.Assert(params.Version, tc.Equals, 1)
		c.Assert(params.UpdateUserSecretParams.Description, tc.DeepEquals, ptr("this is a user secret."))
		c.Assert(params.UpdateUserSecretParams.Label, tc.DeepEquals, ptr("label"))
		c.Assert(params.UpdateUserSecretParams.Data, tc.DeepEquals, coresecrets.SecretData(map[string]string{"foo": "bar"}))
		c.Assert(params.UpdateUserSecretParams.Checksum, tc.Equals, "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b")
		return nil
	})
	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, tc.ErrorIsNil)

	result, err := facade.CreateSecrets(c.Context(), params.CreateSecretArgs{
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
		s.secretService.EXPECT().GetUserSecretURIByLabel(gomock.Any(), "my-secret").Return(uri, nil)
	} else {
		uriString = uri.String()
	}
	s.secretService.EXPECT().UpdateUserSecret(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg1 *coresecrets.URI, params secretservice.UpdateUserSecretParams) error {
		c.Assert(arg1, tc.DeepEquals, uri)
		c.Assert(params.Description, tc.DeepEquals, ptr("this is a user secret."))
		c.Assert(params.Label, tc.DeepEquals, ptr("label"))
		c.Assert(params.AutoPrune, tc.DeepEquals, ptr(true))
		c.Assert(params.Data, tc.DeepEquals, coresecrets.SecretData(map[string]string{"foo": "bar"}))
		c.Assert(params.Checksum, tc.Equals, "7a38bf81f383f69433ad6e900d35b3e2385593f76a7b7ab5d4355b8ba41ee24b")
		return nil
	})
	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, tc.ErrorIsNil)

	result, err := facade.UpdateSecrets(c.Context(), params.UpdateUserSecretArgs{
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
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		Accessor:  secretservice.SecretAccessor{Kind: secretservice.ModelAccessor, ID: coretesting.ModelTag.Id()},
		Revisions: []int{666},
	}).Return(nil)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
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

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
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
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		Accessor:  secretservice.SecretAccessor{Kind: secretservice.ModelAccessor, ID: coretesting.ModelTag.Id()},
		Revisions: []int{666},
	}).Return(nil)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
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
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		Accessor:  secretservice.SecretAccessor{Kind: secretservice.ModelAccessor, ID: coretesting.ModelTag.Id()},
		Revisions: []int{666},
	}).Return(secreterrors.SecretNotFound)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
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
		func(_ context.Context, arg *coresecrets.URI, params secretservice.SecretAccessParams) error {
			c.Assert(arg, tc.DeepEquals, uri)
			c.Assert(params.Scope, tc.DeepEquals,
				secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, tc.DeepEquals,
				secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: "gitlab"})
			c.Assert(params.Role, tc.Equals, coresecrets.RoleView)
			return nil
		},
	)
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secretservice.SecretAccessParams) error {
			c.Assert(arg, tc.DeepEquals, uri)
			c.Assert(params.Scope, tc.DeepEquals,
				secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, tc.DeepEquals,
				secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: "mysql"})
			c.Assert(params.Role, tc.Equals, coresecrets.RoleView)
			return nil
		},
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
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
	s.secretService.EXPECT().GetUserSecretURIByLabel(gomock.Any(), "my-secret").Return(uri, nil)
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secretservice.SecretAccessParams) error {
			c.Assert(arg, tc.DeepEquals, uri)
			c.Assert(params.Scope, tc.DeepEquals,
				secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, tc.DeepEquals,
				secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: "gitlab"})
			c.Assert(params.Role, tc.Equals, coresecrets.RoleView)
			return nil
		},
	)
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secretservice.SecretAccessParams) error {
			c.Assert(arg, tc.DeepEquals, uri)
			c.Assert(params.Scope, tc.DeepEquals,
				secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, tc.DeepEquals,
				secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: "mysql"})
			c.Assert(params.Role, tc.Equals, coresecrets.RoleView)
			return nil
		},
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
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

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
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
		func(_ context.Context, arg *coresecrets.URI, params secretservice.SecretAccessParams) error {
			c.Assert(arg, tc.DeepEquals, uri)
			c.Assert(params.Scope, tc.DeepEquals,
				secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, tc.DeepEquals,
				secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: "gitlab"})
			c.Assert(params.Role, tc.Equals, coresecrets.RoleView)
			return nil
		},
	)
	s.secretService.EXPECT().RevokeSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secretservice.SecretAccessParams) error {
			c.Assert(arg, tc.DeepEquals, uri)
			c.Assert(params.Scope, tc.DeepEquals,
				secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, tc.DeepEquals,
				secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: "mysql"})
			c.Assert(params.Role, tc.Equals, coresecrets.RoleView)
			return nil
		},
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
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

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, tc.ErrorIsNil)

	_, err = facade.RevokeSecret(c.Context(), params.GrantRevokeUserSecretArg{Label: "my-secret"})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}
