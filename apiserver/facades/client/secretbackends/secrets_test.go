// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func ptr[T any](v T) *T {
	return &v
}

type SecretsSuite struct {
	testhelpers.IsolationSuite

	authorizer         *facademocks.MockAuthorizer
	mockBackendService *MockSecretBackendService
}

var _ = tc.Suite(&SecretsSuite{})

func (s *SecretsSuite) setup(c *tc.C) (*SecretBackendsAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().AuthClient().Return(true)
	s.mockBackendService = NewMockSecretBackendService(ctrl)
	api, err := NewTestAPI(s.authorizer, s.mockBackendService)
	c.Assert(err, tc.ErrorIsNil)
	return api, ctrl
}

func (s *SecretsSuite) TestAddSecretBackends(c *tc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)
	addedConfig := map[string]interface{}{
		"endpoint": "http://vault",
	}
	s.mockBackendService.EXPECT().CreateSecretBackend(gomock.Any(), secrets.SecretBackend{
		ID:                  "backend-id",
		Name:                "myvault",
		BackendType:         "vault",
		TokenRotateInterval: ptr(200 * time.Minute),
		Config:              addedConfig,
	}).Return(nil)
	s.mockBackendService.EXPECT().CreateSecretBackend(gomock.Any(), secrets.SecretBackend{
		ID:          "existing-id",
		Name:        "myvault2",
		BackendType: "vault",
		Config:      addedConfig,
	}).Return(secretbackenderrors.AlreadyExists)

	results, err := facade.AddSecretBackends(c.Context(), params.AddSecretBackendArgs{
		Args: []params.AddSecretBackendArg{{
			ID: "backend-id",
			SecretBackend: params.SecretBackend{
				Name:                "myvault",
				BackendType:         "vault",
				TokenRotateInterval: ptr(200 * time.Minute),
				Config:              map[string]interface{}{"endpoint": "http://vault"},
			},
		}, {
			ID: "existing-id",
			SecretBackend: params.SecretBackend{
				Name:        "myvault2",
				BackendType: "vault",
				Config:      map[string]interface{}{"endpoint": "http://vault"},
			},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.DeepEquals, []params.ErrorResult{
		{},
		{Error: &params.Error{
			Code:    "secret backend already exists",
			Message: `secret backend already exists`}},
	})
}

func (s *SecretsSuite) TestAddSecretBackendsPermissionDenied(c *tc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	_, err := facade.AddSecretBackends(c.Context(), params.AddSecretBackendArgs{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestListSecretBackends(c *tc.C) {
	s.assertListSecretBackends(c, false)
}

func (s *SecretsSuite) TestListSecretBackendsReveal(c *tc.C) {
	s.assertListSecretBackends(c, true)
}

func (s *SecretsSuite) assertListSecretBackends(c *tc.C, reveal bool) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	if reveal {
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)
	}
	s.mockBackendService.EXPECT().BackendSummaryInfo(gomock.Any(), reveal, "myvault").
		Return([]*secretbackendservice.SecretBackendInfo{
			{
				SecretBackend: secrets.SecretBackend{
					ID:                  "backend-id",
					Name:                "myvault",
					BackendType:         "vault",
					TokenRotateInterval: ptr(666 * time.Minute),
					Config: map[string]any{
						"endpoint": "http://vault",
						"token":    "s.ajehjdee",
					},
				},
				NumSecrets: 3,
				Status:     "error",
				Message:    "ping error",
			},
			{
				SecretBackend: secrets.SecretBackend{
					ID:          coretesting.ControllerTag.Id(),
					Name:        "internal",
					BackendType: "controller",
					Config:      map[string]any{},
				},
				NumSecrets: 1,
				Status:     "active",
			},
		}, nil)

	results, err := facade.ListSecretBackends(c.Context(),
		params.ListSecretBackendsArgs{
			Names: []string{"myvault"}, Reveal: reveal,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ListSecretBackendsResults{
		Results: []params.SecretBackendResult{
			{
				Result: params.SecretBackend{
					Name:                "myvault",
					BackendType:         "vault",
					TokenRotateInterval: ptr(666 * time.Minute),
					Config: map[string]any{
						"endpoint": "http://vault",
						"token":    "s.ajehjdee",
					},
				},
				ID:         "backend-id",
				NumSecrets: 3,
				Status:     "error",
				Message:    "ping error",
			},
			{
				Result: params.SecretBackend{
					Name:        "internal",
					BackendType: "controller",
					Config:      map[string]interface{}{},
				},
				ID:         coretesting.ControllerTag.Id(),
				Status:     "active",
				NumSecrets: 1,
			},
		},
	})
}

func (s *SecretsSuite) TestListSecretBackendsPermissionDeniedReveal(c *tc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	_, err := facade.ListSecretBackends(c.Context(), params.ListSecretBackendsArgs{Reveal: true})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestUpdateSecretBackends(c *tc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)

	s.mockBackendService.EXPECT().UpdateSecretBackend(gomock.Any(),
		secretbackendservice.UpdateSecretBackendParams{
			UpdateSecretBackendParams: secretbackend.UpdateSecretBackendParams{
				BackendIdentifier:   secretbackend.BackendIdentifier{Name: "myvault"},
				NewName:             ptr("new-name"),
				TokenRotateInterval: ptr(200 * time.Minute),
				Config: map[string]string{
					"endpoint":        "http://vault",
					"namespace":       "foo",
					"tls-server-name": "server-name",
				},
			},
			Reset:    []string{"namespace"},
			SkipPing: true,
		},
	).Return(nil)
	s.mockBackendService.EXPECT().UpdateSecretBackend(gomock.Any(),
		secretbackendservice.UpdateSecretBackendParams{
			UpdateSecretBackendParams: secretbackend.UpdateSecretBackendParams{
				BackendIdentifier: secretbackend.BackendIdentifier{Name: "not-existing-name"},
			},
		},
	).Return(secretbackenderrors.NotFound)

	results, err := facade.UpdateSecretBackends(c.Context(), params.UpdateSecretBackendArgs{
		Args: []params.UpdateSecretBackendArg{{
			Name:                "myvault",
			NameChange:          ptr("new-name"),
			TokenRotateInterval: ptr(200 * time.Minute),
			Config: map[string]interface{}{
				"endpoint":        "http://vault",
				"namespace":       "foo",
				"tls-server-name": "server-name",
			},
			Reset: []string{"namespace"},
			Force: true,
		}, {
			Name: "not-existing-name",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.DeepEquals, []params.ErrorResult{
		{},
		{Error: &params.Error{
			Code:    "secret backend not found",
			Message: `secret backend not found`}},
	})
}

func (s *SecretsSuite) TestUpdateSecretBackendsPermissionDenied(c *tc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	_, err := facade.UpdateSecretBackends(c.Context(), params.UpdateSecretBackendArgs{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestRemoveSecretBackends(c *tc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, coretesting.ControllerTag).Return(nil)

	gomock.InOrder(
		s.mockBackendService.EXPECT().DeleteSecretBackend(gomock.Any(),
			secretbackendservice.DeleteSecretBackendParams{
				BackendIdentifier: secretbackend.BackendIdentifier{Name: "myvault"},
				DeleteInUse:       true,
			}).Return(nil),
		s.mockBackendService.EXPECT().DeleteSecretBackend(gomock.Any(),
			secretbackendservice.DeleteSecretBackendParams{
				BackendIdentifier: secretbackend.BackendIdentifier{Name: "myvault2"},
				DeleteInUse:       false,
			}).Return(errors.NotSupportedf("remove with revisions")),
	)

	results, err := facade.RemoveSecretBackends(c.Context(), params.RemoveSecretBackendArgs{
		Args: []params.RemoveSecretBackendArg{{
			Name:  "myvault",
			Force: true,
		}, {
			Name: "myvault2",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.DeepEquals, []params.ErrorResult{
		{},
		{Error: &params.Error{
			Code:    "not supported",
			Message: `remove with revisions not supported`}},
	})
}

func (s *SecretsSuite) TestRemoveSecretBackendsPermissionDenied(c *tc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, coretesting.ControllerTag).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	_, err := facade.RemoveSecretBackends(c.Context(), params.RemoveSecretBackendArgs{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}
