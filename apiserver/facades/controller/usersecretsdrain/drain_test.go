// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain_test

import (
	"context"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/usersecretsdrain"
	"github.com/juju/juju/apiserver/facades/controller/usersecretsdrain/mocks"
	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type drainSuite struct {
	testing.IsolationSuite

	authorizer           *facademocks.MockAuthorizer
	secretService        *mocks.MockSecretService
	secretBackendService *mocks.MockSecretBackendService
	facade               *usersecretsdrain.SecretsDrainAPI
}

var _ = tc.Suite(&drainSuite{})

func (s *drainSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().AuthController().Return(true)
	s.secretService = mocks.NewMockSecretService(ctrl)
	s.secretBackendService = mocks.NewMockSecretBackendService(ctrl)

	var err error
	s.facade, err = usersecretsdrain.NewTestAPI(s.authorizer, s.secretService, s.secretBackendService)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

type backendConfigParamsMatcher struct {
	c        *tc.C
	expected any
}

func (m backendConfigParamsMatcher) Matches(x interface{}) bool {
	if obtained, ok := x.(secretbackendservice.BackendConfigParams); ok {
		m.c.Assert(obtained.GrantedSecretsGetter, tc.NotNil)
		obtained.GrantedSecretsGetter = nil
		m.c.Assert(obtained, jc.DeepEquals, m.expected)
		return true
	}
	obtained, ok := x.(secretbackendservice.DrainBackendConfigParams)
	if !ok {
		return false
	}
	m.c.Assert(obtained.GrantedSecretsGetter, tc.NotNil)
	obtained.GrantedSecretsGetter = nil
	m.c.Assert(obtained, jc.DeepEquals, m.expected)
	return true
}

func (m backendConfigParamsMatcher) String() string {
	return "Match the contents of BackendConfigParams"
}

func (s *drainSuite) TestGetSecretBackendConfigs(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretBackendService.EXPECT().DrainBackendConfigInfo(gomock.Any(), backendConfigParamsMatcher{c: c,
		expected: secretbackendservice.DrainBackendConfigParams{
			Accessor: secretservice.SecretAccessor{
				Kind: secretservice.ModelAccessor,
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
					Config:      map[string]interface{}{"foo": "admin"},
				},
			},
		},
	}, nil)

	result, err := s.facade.GetSecretBackendConfigs(context.Background(), params.SecretBackendArgs{
		BackendIDs: []string{"backend-id"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.SecretBackendConfigResults{
		ActiveID: "backend-id",
		Results: map[string]params.SecretBackendConfigResult{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				Draining:       true,
				Config: params.SecretBackendConfig{
					BackendType: "some-backend",
					Params:      map[string]interface{}{"foo": "admin"},
				},
			},
		},
	})
}

func (s *drainSuite) TestGetSecretContentInvalidArg(c *tc.C) {
	defer s.setup(c).Finish()

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{{}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `empty URI`)
}

func (s *drainSuite) TestGetSecretContentInternal(c *tc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecret(gomock.Any(), uri).Return(&coresecrets.SecretMetadata{URI: uri, LatestRevision: 668}, nil)
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, secretservice.SecretAccessor{
		Kind: secretservice.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String()},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *drainSuite) TestGetSecretContentExternal(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecret(gomock.Any(), uri).Return(&coresecrets.SecretMetadata{URI: uri, LatestRevision: 668}, nil)
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, secretservice.SecretAccessor{
		Kind: secretservice.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}).Return(
		nil, &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}, nil,
	)
	s.secretBackendService.EXPECT().BackendConfigInfo(gomock.Any(), backendConfigParamsMatcher{c: c,
		expected: secretbackendservice.BackendConfigParams{
			Accessor: secretservice.SecretAccessor{
				Kind: secretservice.ModelAccessor,
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
					Config:      map[string]interface{}{"foo": "bar"},
				},
			},
		},
	}, nil)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String()},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
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
					Params:      map[string]interface{}{"foo": "bar"},
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
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 666, secretservice.SecretAccessor{
		Kind: secretservice.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretRevisionContentInfo(context.Background(), params.SecretRevisionArg{
		URI:       uri.String(),
		Revisions: []int{666},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *drainSuite) TestGetSecretRevisionContentInfoExternal(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 666, secretservice.SecretAccessor{
		Kind: secretservice.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}).Return(
		nil, &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}, nil,
	)
	s.secretBackendService.EXPECT().BackendConfigInfo(gomock.Any(), backendConfigParamsMatcher{c: c,
		expected: secretbackendservice.BackendConfigParams{
			Accessor: secretservice.SecretAccessor{
				Kind: secretservice.ModelAccessor,
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
					Config:      map[string]interface{}{"foo": "bar"},
				},
			},
		},
	}, nil)

	results, err := s.facade.GetSecretRevisionContentInfo(context.Background(), params.SecretRevisionArg{
		URI:       uri.String(),
		Revisions: []int{666},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
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
					Params:      map[string]interface{}{"foo": "bar"},
				},
			},
		}},
	})
}
