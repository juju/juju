// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain_test

import (
	"fmt"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/usersecretsdrain"
	"github.com/juju/juju/apiserver/facades/controller/usersecretsdrain/mocks"
	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type drainSuite struct {
	testhelpers.IsolationSuite

	authorizer           *facademocks.MockAuthorizer
	secretService        *mocks.MockSecretService
	secretBackendService *mocks.MockSecretBackendService
	facade               *usersecretsdrain.SecretsDrainAPI
}

func TestDrainSuite(t *testing.T) {
	tc.Run(t, &drainSuite{})
}

func (s *drainSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().AuthController().Return(true)
	s.secretService = mocks.NewMockSecretService(ctrl)
	s.secretBackendService = mocks.NewMockSecretBackendService(ctrl)

	var err error
	s.facade, err = usersecretsdrain.NewTestAPI(s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, tc.ErrorIsNil)

	return ctrl
}

type backendConfigParamsMatcher struct {
	c        *tc.C
	expected any
}

func (m backendConfigParamsMatcher) Matches(x any) bool {
	if obtained, ok := x.(secretbackendservice.BackendConfigParams); ok {
		m.c.Assert(obtained.GrantedSecretsGetter, tc.NotNil)
		obtained.GrantedSecretsGetter = nil
		m.c.Assert(obtained, tc.DeepEquals, m.expected)
		return true
	}
	obtained, ok := x.(secretbackendservice.DrainBackendConfigParams)
	if !ok {
		return false
	}
	m.c.Assert(obtained.GrantedSecretsGetter, tc.NotNil)
	obtained.GrantedSecretsGetter = nil
	m.c.Assert(obtained, tc.DeepEquals, m.expected)
	return true
}

func (m backendConfigParamsMatcher) String() string {
	return "Match the contents of BackendConfigParams"
}

func (s *drainSuite) TestGetSecretBackendConfigs(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretBackendService.EXPECT().DrainBackendConfigInfo(gomock.Any(), backendConfigParamsMatcher{c: c,
		expected: secretbackendservice.DrainBackendConfigParams{
			Accessor: secret.SecretAccessor{
				Kind: secret.ModelAccessor,
				ID:   coretesting.ModelTag.Id(),
			},
			ModelUUID: model.UUID(coretesting.ModelTag.Id()),
			BackendID: "backend-id",
		}}).Return(&provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
					Config:      map[string]any{"foo": "admin"},
				},
			},
		},
	}, nil)

	result, err := s.facade.GetSecretBackendConfigs(c.Context(), params.SecretBackendArgs{
		BackendIDs: []string{"backend-id"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.SecretBackendConfigResults{
		ActiveID: "backend-id",
		Results: map[string]params.SecretBackendConfigResult{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				Draining:       true,
				Config: params.SecretBackendConfig{
					BackendType: "some-backend",
					Params:      map[string]any{"foo": "admin"},
				},
			},
		},
	})
}

func (s *drainSuite) TestGetSecretContentInvalidArg(c *tc.C) {
	defer s.setup(c).Finish()

	results, err := s.facade.GetSecretContentInfo(c.Context(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{{}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `empty URI`)
}

func (s *drainSuite) TestGetSecretContentInternal(c *tc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecret(gomock.Any(), uri).Return(&coresecrets.SecretMetadata{URI: uri, LatestRevision: 668}, nil)
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(c.Context(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *drainSuite) TestGetSecretContentExternal(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecret(gomock.Any(), uri).Return(&coresecrets.SecretMetadata{URI: uri, LatestRevision: 668}, nil)
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}).Return(
		nil, &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}, nil,
	)
	s.secretBackendService.EXPECT().BackendConfigInfo(gomock.Any(), backendConfigParamsMatcher{c: c,
		expected: secretbackendservice.BackendConfigParams{
			Accessor: secret.SecretAccessor{
				Kind: secret.ModelAccessor,
				ID:   coretesting.ModelTag.Id(),
			},
			ModelUUID:      model.UUID(coretesting.ModelTag.Id()),
			BackendIDs:     []string{"backend-id"},
			SameController: true,
		}}).Return(&provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
					Config:      map[string]any{"foo": "bar"},
				},
			},
		},
	}, nil)

	results, err := s.facade.GetSecretContentInfo(c.Context(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{
				ValueRef: &params.SecretValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				},
			},
			BackendConfig: &params.SecretBackendConfigResult{
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				Draining:       false,
				Config: params.SecretBackendConfig{
					BackendType: "some-backend",
					Params:      map[string]any{"foo": "bar"},
				},
			},
		}},
	})
}

func (s *drainSuite) TestGetSecretRevisionContentInfoInternal(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 666, secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretRevisionContentInfo(c.Context(), params.SecretRevisionArg{
		URI:       uri.String(),
		Revisions: []int{666},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *drainSuite) TestGetSecretRevisionContentInfoExternal(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 666, secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}).Return(
		nil, &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}, nil,
	)
	s.secretBackendService.EXPECT().BackendConfigInfo(gomock.Any(), backendConfigParamsMatcher{c: c,
		expected: secretbackendservice.BackendConfigParams{
			Accessor: secret.SecretAccessor{
				Kind: secret.ModelAccessor,
				ID:   coretesting.ModelTag.Id(),
			},
			ModelUUID:      model.UUID(coretesting.ModelTag.Id()),
			BackendIDs:     []string{"backend-id"},
			SameController: true,
		}}).Return(&provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
					Config:      map[string]any{"foo": "bar"},
				},
			},
		},
	}, nil)

	results, err := s.facade.GetSecretRevisionContentInfo(c.Context(), params.SecretRevisionArg{
		URI:       uri.String(),
		Revisions: []int{666},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{
				ValueRef: &params.SecretValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				},
			},
			BackendConfig: &params.SecretBackendConfigResult{
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				Draining:       false,
				Config: params.SecretBackendConfig{
					BackendType: "some-backend",
					Params:      map[string]any{"foo": "bar"},
				},
			},
		}},
	})
}

func (s *drainSuite) TestGetSecretBackendConfigsNotFound(c *tc.C) {
	defer s.setup(c).Finish()

	// Arrange:
	s.secretBackendService.EXPECT().DrainBackendConfigInfo(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("boom: %w", secretbackenderrors.NotFound))

	// Act:
	_, err := s.facade.GetSecretBackendConfigs(c.Context(), params.SecretBackendArgs{
		BackendIDs: []string{"backend-id"},
	})

	// Assert:
	pErr, ok := err.(*params.Error)
	c.Assert(ok, tc.IsTrue)
	c.Check(pErr.Code, tc.Equals, params.CodeSecretBackendNotFound)
}

// TestGetSecretContentInfoErrorCodes checks every sentinel reachable through
// getSecretContent: the metadata lookup can fail with SecretNotFound, the
// revision read with SecretNotFound, SecretRevisionNotFound or
// PermissionDenied, and the backend lookup with secretbackenderrors.NotFound.
func (s *drainSuite) TestGetSecretContentInfoErrorCodes(c *tc.C) {
	accessor := secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}
	for _, t := range []struct {
		about  string
		expect func(*tc.C, *coresecrets.URI, error)
		err    error
		code   string
	}{{
		about: "secret metadata not found",
		expect: func(c *tc.C, uri *coresecrets.URI, err error) {
			s.secretService.EXPECT().GetSecret(gomock.Any(), uri).Return(nil, err)
		},
		err:  secreterrors.SecretNotFound,
		code: params.CodeSecretNotFound,
	}, {
		about: "revision not found",
		expect: func(c *tc.C, uri *coresecrets.URI, err error) {
			s.secretService.EXPECT().GetSecret(gomock.Any(), uri).
				Return(&coresecrets.SecretMetadata{URI: uri, LatestRevision: 668}, nil)
			s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, accessor).Return(nil, nil, err)
		},
		err:  secreterrors.SecretRevisionNotFound,
		code: params.CodeSecretRevisionNotFound,
	}, {
		about: "read access denied",
		expect: func(c *tc.C, uri *coresecrets.URI, err error) {
			s.secretService.EXPECT().GetSecret(gomock.Any(), uri).
				Return(&coresecrets.SecretMetadata{URI: uri, LatestRevision: 668}, nil)
			s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, accessor).Return(nil, nil, err)
		},
		err:  secreterrors.PermissionDenied,
		code: params.CodeUnauthorized,
	}, {
		about: "backend not found",
		expect: func(c *tc.C, uri *coresecrets.URI, err error) {
			s.secretService.EXPECT().GetSecret(gomock.Any(), uri).
				Return(&coresecrets.SecretMetadata{URI: uri, LatestRevision: 668}, nil)
			s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, accessor).
				Return(nil, &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "rev-id"}, nil)
			s.secretBackendService.EXPECT().BackendConfigInfo(gomock.Any(), gomock.Any()).Return(nil, err)
		},
		err:  secretbackenderrors.NotFound,
		code: params.CodeSecretBackendNotFound,
	}} {
		c.Logf("%s", t.about)
		func() {
			defer s.setup(c).Finish()

			// Arrange:
			uri := coresecrets.NewURI()
			t.expect(c, uri, fmt.Errorf("boom: %w", t.err))

			// Act:
			results, err := s.facade.GetSecretContentInfo(c.Context(), params.GetSecretContentArgs{
				Args: []params.GetSecretContentArg{{URI: uri.String()}},
			})

			// Assert:
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(results.Results, tc.HasLen, 1)
			c.Assert(results.Results[0].Error, tc.NotNil)
			c.Check(results.Results[0].Error.Code, tc.Equals, t.code)
		}()
	}
}

// TestGetSecretRevisionContentInfoErrorCodes checks every sentinel reachable
// through GetSecretValue plus the backend lookup for external revisions.
func (s *drainSuite) TestGetSecretRevisionContentInfoErrorCodes(c *tc.C) {
	accessor := secret.SecretAccessor{
		Kind: secret.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}
	for _, t := range []struct {
		about  string
		expect func(*tc.C, *coresecrets.URI, error)
		err    error
		code   string
	}{{
		about: "secret not found",
		expect: func(c *tc.C, uri *coresecrets.URI, err error) {
			s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 666, accessor).Return(nil, nil, err)
		},
		err:  secreterrors.SecretNotFound,
		code: params.CodeSecretNotFound,
	}, {
		about: "revision not found",
		expect: func(c *tc.C, uri *coresecrets.URI, err error) {
			s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 666, accessor).Return(nil, nil, err)
		},
		err:  secreterrors.SecretRevisionNotFound,
		code: params.CodeSecretRevisionNotFound,
	}, {
		about: "read access denied",
		expect: func(c *tc.C, uri *coresecrets.URI, err error) {
			s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 666, accessor).Return(nil, nil, err)
		},
		err:  secreterrors.PermissionDenied,
		code: params.CodeUnauthorized,
	}, {
		about: "backend not found",
		expect: func(c *tc.C, uri *coresecrets.URI, err error) {
			s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 666, accessor).
				Return(nil, &coresecrets.ValueRef{BackendID: "backend-id", RevisionID: "rev-id"}, nil)
			s.secretBackendService.EXPECT().BackendConfigInfo(gomock.Any(), gomock.Any()).Return(nil, err)
		},
		err:  secretbackenderrors.NotFound,
		code: params.CodeSecretBackendNotFound,
	}} {
		c.Logf("%s", t.about)
		func() {
			defer s.setup(c).Finish()

			// Arrange:
			uri := coresecrets.NewURI()
			t.expect(c, uri, fmt.Errorf("boom: %w", t.err))

			// Act:
			results, err := s.facade.GetSecretRevisionContentInfo(c.Context(), params.SecretRevisionArg{
				URI:       uri.String(),
				Revisions: []int{666},
			})

			// Assert:
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(results.Results, tc.HasLen, 1)
			c.Assert(results.Results[0].Error, tc.NotNil)
			c.Check(results.Results[0].Error.Code, tc.Equals, t.code)
		}()
	}
}
