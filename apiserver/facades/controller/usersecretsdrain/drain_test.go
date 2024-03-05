// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/usersecretsdrain"
	"github.com/juju/juju/apiserver/facades/controller/usersecretsdrain/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type drainSuite struct {
	testing.IsolationSuite

	authorizer   *facademocks.MockAuthorizer
	secretsState *mocks.MockSecretsState
	facade       *usersecretsdrain.SecretsDrainAPI
}

var _ = gc.Suite(&drainSuite{})

func (s *drainSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().AuthController().Return(true)
	s.secretsState = mocks.NewMockSecretsState(ctrl)

	backendConfigGetter := func(_ context.Context, backendIds []string, wantAll bool) (*provider.ModelBackendConfigInfo, error) {
		// wantAll is for 3.1 compatibility only.
		if wantAll {
			return nil, errors.NotSupportedf("wantAll")
		}
		return &provider.ModelBackendConfigInfo{
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
		}, nil
	}

	drainConfigGetter := func(_ context.Context, backendID string) (*provider.ModelBackendConfigInfo, error) {
		return &provider.ModelBackendConfigInfo{
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
		}, nil
	}

	var err error
	s.facade, err = usersecretsdrain.NewTestAPI(s.authorizer, s.secretsState, backendConfigGetter, drainConfigGetter)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *drainSuite) TestGetSecretBackendConfigs(c *gc.C) {
	defer s.setup(c).Finish()

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

func (s *drainSuite) TestGetSecretContentInvalidArg(c *gc.C) {
	defer s.setup(c).Finish()

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{{}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `empty URI`)
}

func (s *drainSuite) TestGetSecretContentInternal(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()
	s.secretsState.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{URI: uri, LatestRevision: 668}, nil)
	s.secretsState.EXPECT().GetSecretValue(uri, 668).Return(
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

func (s *drainSuite) TestGetSecretContentExternal(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsState.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{URI: uri, LatestRevision: 668}, nil)
	s.secretsState.EXPECT().GetSecretValue(uri, 668).Return(
		nil, &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}, nil,
	)

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

func (s *drainSuite) TestGetSecretRevisionContentInfoInternal(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	s.secretsState.EXPECT().GetSecretValue(uri, 666).Return(
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

func (s *drainSuite) TestGetSecretRevisionContentInfoExternal(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsState.EXPECT().GetSecretValue(uri, 666).Return(
		nil, &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}, nil,
	)

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
