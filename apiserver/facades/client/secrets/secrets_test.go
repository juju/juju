// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apisecrets "github.com/juju/juju/apiserver/facades/client/secrets"
	"github.com/juju/juju/apiserver/facades/client/secrets/mocks"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type SecretsSuite struct {
	testing.IsolationSuite

	authorizer           *facademocks.MockAuthorizer
	authTag              names.Tag
	secretService        *mocks.MockSecretService
	secretBackendService *mocks.MockSecretBackendService
}

var _ = gc.Suite(&SecretsSuite{})

func (s *SecretsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.authTag = names.NewUserTag("foo")
}

func (s *SecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.secretService = mocks.NewMockSecretService(ctrl)
	s.secretBackendService = mocks.NewMockSecretBackendService(ctrl)
	return ctrl
}

func (s *SecretsSuite) expectAuthClient() {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *SecretsSuite) TestListSecrets(c *gc.C) {
	s.assertListSecrets(c, false)
}

func (s *SecretsSuite) TestListSecretsReveal(c *gc.C) {
	s.assertListSecrets(c, true)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) assertListSecrets(c *gc.C, reveal bool) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	if reveal {
		s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)
	} else {
		s.authorizer.EXPECT().HasPermission(permission.ReadAccess, coretesting.ModelTag).Return(nil)
	}

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	now := time.Now()
	uri := coresecrets.NewURI()
	metadata := []*coresecrets.SecretMetadata{{
		URI:              uri,
		Version:          1,
		Owner:            coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mysql"},
		RotatePolicy:     coresecrets.RotateHourly,
		LatestRevision:   2,
		LatestExpireTime: ptr(now),
		NextRotateTime:   ptr(now.Add(time.Hour)),
		Description:      "shhh",
		Label:            "foobar",
		CreateTime:       now,
		UpdateTime:       now.Add(time.Second),
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

	results, err := facade.ListSecrets(context.Background(), params.ListSecretsArgs{ShowSecrets: reveal})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ListSecretResults{
		Results: []params.ListSecretResult{{
			URI:              uri.String(),
			Version:          1,
			OwnerTag:         "application-mysql",
			RotatePolicy:     string(coresecrets.RotateHourly),
			LatestExpireTime: ptr(now),
			NextRotateTime:   ptr(now.Add(time.Hour)),
			Description:      "shhh",
			Label:            "foobar",
			LatestRevision:   2,
			CreateTime:       now,
			UpdateTime:       now.Add(time.Second),
			Value:            valueResult,
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

func (s *SecretsSuite) TestListSecretsPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.ReadAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.ListSecrets(context.Background(), params.ListSecretsArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestListSecretsPermissionDeniedShow(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))
	s.authorizer.EXPECT().HasPermission(permission.AdminAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.ListSecrets(context.Background(), params.ListSecretsArgs{ShowSecrets: true})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestCreateSecretsPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.CreateSecrets(context.Background(), params.CreateSecretArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestCreateSecretsEmptyData(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	uriStrPtr := ptr(uri.String())

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.CreateSecrets(context.Background(), params.CreateSecretArgs{
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

func (s *SecretsSuite) TestCreateSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	uriStrPtr := ptr(uri.String())
	s.secretService.EXPECT().CreateUserSecret(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, arg1 *coresecrets.URI, params secretservice.CreateUserSecretParams) error {
		c.Assert(arg1, gc.DeepEquals, uri)
		c.Assert(params.Version, gc.Equals, 1)
		c.Assert(params.UpdateUserSecretParams.Description, gc.DeepEquals, ptr("this is a user secret."))
		c.Assert(params.UpdateUserSecretParams.Label, gc.DeepEquals, ptr("label"))
		c.Assert(params.UpdateUserSecretParams.Data, gc.DeepEquals, coresecrets.SecretData(map[string]string{"foo": "bar"}))
		return nil
	})
	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.CreateSecrets(context.Background(), params.CreateSecretArgs{
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
	c.Assert(result.Results[0], gc.DeepEquals, params.StringResult{Result: uri.String()})
}

func (s *SecretsSuite) assertUpdateSecrets(c *gc.C, uri *coresecrets.URI) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

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
		c.Assert(arg1, gc.DeepEquals, uri)
		c.Assert(params.Description, gc.DeepEquals, ptr("this is a user secret."))
		c.Assert(params.Label, gc.DeepEquals, ptr("label"))
		c.Assert(params.AutoPrune, gc.DeepEquals, ptr(true))
		c.Assert(params.Data, gc.DeepEquals, coresecrets.SecretData(map[string]string{"foo": "bar"}))
		return nil
	})
	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.UpdateSecrets(context.Background(), params.UpdateUserSecretArgs{
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
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *SecretsSuite) TestUpdateSecrets(c *gc.C) {
	s.assertUpdateSecrets(c, coresecrets.NewURI())
}

func (s *SecretsSuite) TestUpdateSecretsByName(c *gc.C) {
	s.assertUpdateSecrets(c, nil)
}

func (s *SecretsSuite) TestRemoveSecrets(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectAuthClient()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		Accessor:  secretservice.SecretAccessor{Kind: secretservice.ModelAccessor, ID: coretesting.ModelTag.Id()},
		Revisions: []int{666},
	}).Return(nil)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)
	results, err := facade.RemoveSecrets(context.Background(), params.DeleteSecretArgs{
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

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)
	_, err = facade.RemoveSecrets(context.Background(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestRemoveSecretRevision(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectAuthClient()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		Accessor:  secretservice.SecretAccessor{Kind: secretservice.ModelAccessor, ID: coretesting.ModelTag.Id()},
		Revisions: []int{666},
	}).Return(nil)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)
	results, err := facade.RemoveSecrets(context.Background(), params.DeleteSecretArgs{
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
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		Accessor:  secretservice.SecretAccessor{Kind: secretservice.ModelAccessor, ID: coretesting.ModelTag.Id()},
		Revisions: []int{666},
	}).Return(secreterrors.SecretNotFound)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)
	results, err := facade.RemoveSecrets(context.Background(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, jc.Satisfies, params.IsCodeSecretNotFound)
}

func (s *SecretsSuite) TestGrantSecret(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secretservice.SecretAccessParams) error {
			c.Assert(arg, gc.DeepEquals, uri)
			c.Assert(params.Scope, jc.DeepEquals,
				secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, jc.DeepEquals,
				secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: "gitlab"})
			c.Assert(params.Role, gc.Equals, coresecrets.RoleView)
			return nil
		},
	)
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secretservice.SecretAccessParams) error {
			c.Assert(arg, gc.DeepEquals, uri)
			c.Assert(params.Scope, jc.DeepEquals,
				secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, jc.DeepEquals,
				secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: "mysql"})
			c.Assert(params.Role, gc.Equals, coresecrets.RoleView)
			return nil
		},
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.GrantSecret(context.Background(), params.GrantRevokeUserSecretArg{
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
	s.secretService.EXPECT().GetUserSecretURIByLabel(gomock.Any(), "my-secret").Return(uri, nil)
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secretservice.SecretAccessParams) error {
			c.Assert(arg, gc.DeepEquals, uri)
			c.Assert(params.Scope, jc.DeepEquals,
				secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, jc.DeepEquals,
				secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: "gitlab"})
			c.Assert(params.Role, gc.Equals, coresecrets.RoleView)
			return nil
		},
	)
	s.secretService.EXPECT().GrantSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secretservice.SecretAccessParams) error {
			c.Assert(arg, gc.DeepEquals, uri)
			c.Assert(params.Scope, jc.DeepEquals,
				secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, jc.DeepEquals,
				secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: "mysql"})
			c.Assert(params.Role, gc.Equals, coresecrets.RoleView)
			return nil
		},
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.GrantSecret(context.Background(), params.GrantRevokeUserSecretArg{
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

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.GrantSecret(context.Background(), params.GrantRevokeUserSecretArg{Label: "my-secret"})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestRevokeSecret(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().RevokeSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secretservice.SecretAccessParams) error {
			c.Assert(arg, gc.DeepEquals, uri)
			c.Assert(params.Scope, jc.DeepEquals,
				secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, jc.DeepEquals,
				secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: "gitlab"})
			c.Assert(params.Role, gc.Equals, coresecrets.RoleView)
			return nil
		},
	)
	s.secretService.EXPECT().RevokeSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arg *coresecrets.URI, params secretservice.SecretAccessParams) error {
			c.Assert(arg, gc.DeepEquals, uri)
			c.Assert(params.Scope, jc.DeepEquals,
				secretservice.SecretAccessScope{Kind: secretservice.ModelAccessScope, ID: coretesting.ModelTag.Id()})
			c.Assert(params.Subject, jc.DeepEquals,
				secretservice.SecretAccessor{Kind: secretservice.ApplicationAccessor, ID: "mysql"})
			c.Assert(params.Role, gc.Equals, coresecrets.RoleView)
			return nil
		},
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.RevokeSecret(context.Background(), params.GrantRevokeUserSecretArg{
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

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.RevokeSecret(context.Background(), params.GrantRevokeUserSecretArg{Label: "my-secret"})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestGetModelSecretBackendPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.ReadAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission),
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.GetModelSecretBackend(context.Background())
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestGetModelSecretBackendFailed(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.ReadAccess, coretesting.ModelTag).Return(nil)
	s.secretBackendService.EXPECT().GetModelSecretBackend(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id())).Return("", errors.New("boom"))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.GetModelSecretBackend(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.ErrorMatches, "boom")
}

func (s *SecretsSuite) TestGetModelSecretBackend(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.ReadAccess, coretesting.ModelTag).Return(nil)
	s.secretBackendService.EXPECT().GetModelSecretBackend(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id())).Return("myvault", nil)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.GetModelSecretBackend(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.Equals, "myvault")
}

func (s *SecretsSuite) TestSetModelSecretBackendPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission),
	)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.SetModelSecretBackend(context.Background(), params.SetModelSecretBackendArg{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestSetModelSecretBackendFailed(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)
	s.secretBackendService.EXPECT().SetModelSecretBackend(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id()), "myvault").Return(errors.New("boom"))

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.SetModelSecretBackend(context.Background(), params.SetModelSecretBackendArg{
		SecretBackendName: "myvault",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.ErrorMatches, "boom")
}

func (s *SecretsSuite) TestSetModelSecretBackend(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.WriteAccess, coretesting.ModelTag).Return(nil)
	s.secretBackendService.EXPECT().SetModelSecretBackend(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id()), "myvault").Return(nil)

	facade, err := apisecrets.NewTestAPI(s.authTag, s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	result, err := facade.SetModelSecretBackend(context.Background(), params.SetModelSecretBackendArg{
		SecretBackendName: "myvault",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}
