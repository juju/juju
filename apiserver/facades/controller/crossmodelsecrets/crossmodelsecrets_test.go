// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&CrossModelSecretsSuite{})

type CrossModelSecretsSuite struct {
	coretesting.BaseSuite

	resources       *common.Resources
	secretsState    *mocks.MockSecretsState
	secretsConsumer *mocks.MockSecretsConsumer
	crossModelState *mocks.MockCrossModelState

	facade *crossmodelsecrets.CrossModelSecretsAPI
}

func (s *CrossModelSecretsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })
}

func (s *CrossModelSecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.secretsState = mocks.NewMockSecretsState(ctrl)
	s.secretsConsumer = mocks.NewMockSecretsConsumer(ctrl)
	s.crossModelState = mocks.NewMockCrossModelState(ctrl)

	secretsStateGetter := func(modelUUID string) (crossmodelsecrets.SecretsState, crossmodelsecrets.SecretsConsumer, func() bool, error) {
		return s.secretsState, s.secretsConsumer, func() bool { return false }, nil
	}
	backendConfigGetter := func(modelUUID string) (*provider.ModelBackendConfigInfo, error) {
		return &provider.ModelBackendConfigInfo{
			ActiveID: "active-id",
			Configs: map[string]provider.ModelBackendConfig{
				"backend-id": {
					ControllerUUID: coretesting.ControllerTag.Id(),
					ModelUUID:      modelUUID,
					ModelName:      "fred",
					BackendConfig: provider.BackendConfig{
						BackendType: "vault",
						Config:      map[string]interface{}{"foo": "bar"},
					},
				},
			},
		}, nil
	}
	var err error
	s.facade, err = crossmodelsecrets.NewCrossModelSecretsAPI(
		s.resources,
		coretesting.ModelTag.Id(),
		secretsStateGetter,
		backendConfigGetter,
		s.crossModelState,
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *CrossModelSecretsSuite) TestGetSecretContentInfo(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI().WithSource(coretesting.ModelTag.Id())
	app := names.NewApplicationTag("remote-app")
	consumer := names.NewUnitTag("remote-app/666")

	s.crossModelState.EXPECT().GetRemoteEntity("token").Return(app, nil)
	s.secretsConsumer.EXPECT().GetSecretConsumer(uri, consumer)
	s.secretsState.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		LatestRevision: 667,
	}, nil)
	s.secretsConsumer.EXPECT().SaveSecretConsumer(uri, consumer, &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 667,
		LatestRevision:  667,
	}).Return(nil)
	s.secretsConsumer.EXPECT().SecretAccess(uri, consumer).Return(coresecrets.RoleView, nil)
	s.secretsState.EXPECT().GetSecretValue(uri, 667).Return(
		nil,
		&coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}, nil,
	)

	badURI := coresecrets.NewURI()
	args := params.GetRemoteSecretContentArgs{
		Args: []params.GetRemoteSecretContentArg{{
			ApplicationToken: "token",
			UnitId:           666,
			BakeryVersion:    3,
			// TODO(cmr secrets)
			Macaroons: nil,
			GetSecretContentArg: params.GetSecretContentArg{
				URI:     uri.String(),
				Refresh: true,
				Peek:    false,
			},
		}, {
			GetSecretContentArg: params.GetSecretContentArg{
				URI: badURI.String(),
			},
		}},
	}
	results, err := s.facade.GetSecretContentInfo(args)
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
				Draining:       true,
				Config: params.SecretBackendConfig{
					BackendType: "vault",
					Params:      map[string]interface{}{"foo": "bar"},
				},
			},
		}, {
			Error: &params.Error{
				Code:    "not valid",
				Message: "secret URI with empty source UUID not valid",
			},
		}},
	})
}
