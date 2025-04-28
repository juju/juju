// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"
	"time"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/user"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type undertakerSuite struct {
	coretesting.BaseSuite

	modelUUID coremodel.UUID

	secrets                  *mockSecrets
	mockSecretBackendService *MockSecretBackendService
	mockModelConfigService   *MockModelConfigService
	mockModelInfoService     *MockModelInfoService
	mockCloudSpecGetter      *MockModelProviderService
}

var _ = gc.Suite(&undertakerSuite{})

func (s *undertakerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *undertakerSuite) setupStateAndAPI(c *gc.C, isSystem bool, modelName string) (*mockState, *UndertakerAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.mockSecretBackendService = NewMockSecretBackendService(ctrl)
	s.mockModelConfigService = NewMockModelConfigService(ctrl)
	s.mockModelInfoService = NewMockModelInfoService(ctrl)
	s.mockCloudSpecGetter = NewMockModelProviderService(ctrl)

	machineNo := "1"
	if isSystem {
		machineNo = "0"
	}

	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag(machineNo),
		Controller: true,
	}

	st := newMockState(names.NewUserTag("admin"), modelName, isSystem)
	s.secrets = &mockSecrets{}
	s.PatchValue(&GetProvider, func(string) (provider.SecretBackendProvider, error) { return s.secrets, nil })

	s.modelUUID = modeltesting.GenModelUUID(c)
	api, err := newUndertakerAPI(
		s.modelUUID,
		st,
		nil,
		authorizer,
		s.mockCloudSpecGetter,
		s.mockSecretBackendService,
		s.mockModelConfigService,
		s.mockModelInfoService,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	return st, api, ctrl
}

func (s *undertakerSuite) TestNoPerms(c *gc.C) {
	for _, authorizer := range []apiservertesting.FakeAuthorizer{{
		Tag: names.NewMachineTag("0"),
	}, {
		Tag: names.NewUserTag("bob"),
	}} {
		st := newMockState(names.NewUserTag("admin"), "admin", true)
		_, err := newUndertakerAPI(
			modeltesting.GenModelUUID(c),
			st,
			nil,
			authorizer,
			nil,
			nil,
			nil,
			nil,
			nil,
		)
		c.Assert(err, gc.ErrorMatches, "permission denied")
	}
}

func (s *undertakerSuite) TestModelInfo(c *gc.C) {
	ctx := context.Background()
	name, err := user.NewName("user-admin")
	c.Assert(err, jc.ErrorIsNil)

	modelUUID := modeltesting.GenModelUUID(c)

	otherSt, hostedAPI, _ := s.setupStateAndAPI(c, false, "hostedmodel")
	s.mockModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		UUID:            modelUUID,
		Name:            "hostedmodel",
		CredentialOwner: name,
	}, nil).Times(2)

	st, api, _ := s.setupStateAndAPI(c, true, "admin")
	s.mockModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		UUID:            modelUUID,
		Name:            "admin",
		CredentialOwner: name,
	}, nil).Times(2)

	for _, test := range []struct {
		st        *mockState
		api       *UndertakerAPI
		isSystem  bool
		modelName string
	}{
		{otherSt, hostedAPI, false, "hostedmodel"},
		{st, api, true, "admin"},
	} {
		test.st.model.life = state.Dying
		test.st.model.forced = true
		minute := time.Minute
		test.st.model.timeout = &minute

		result, err := test.api.ModelInfo(ctx)
		c.Assert(err, jc.ErrorIsNil)

		info := result.Result
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Error, gc.IsNil)

		modelInfo, err := test.api.modelInfoService.GetModelInfo(ctx)
		c.Assert(err, jc.ErrorIsNil)

		c.Assert(info.UUID, gc.Equals, modelInfo.UUID.String())
		c.Assert(info.Name, gc.Equals, modelInfo.Name)
		c.Assert(info.IsSystem, gc.Equals, test.isSystem)
		c.Assert(info.Life, gc.Equals, life.Dying)
		c.Assert(info.ForceDestroyed, gc.Equals, true)
		c.Assert(info.DestroyTimeout, gc.NotNil)
		c.Assert(*info.DestroyTimeout, gc.Equals, time.Minute)
	}
}

func (s *undertakerSuite) TestProcessDyingModel(c *gc.C) {
	ctx := context.Background()
	otherSt, hostedAPI, _ := s.setupStateAndAPI(c, false, "hostedmodel")
	model, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = hostedAPI.ProcessDyingModel(ctx)
	c.Assert(err, gc.ErrorMatches, "model is not dying")
	c.Assert(model.Life(), gc.Equals, state.Alive)

	otherSt.model.life = state.Dying
	err = hostedAPI.ProcessDyingModel(ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(model.Life(), gc.Equals, state.Dead)
}

func (s *undertakerSuite) TestRemoveAliveModel(c *gc.C) {
	ctx := context.Background()
	_, hostedAPI, ctrl := s.setupStateAndAPI(c, false, "hostedmodel")
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		UUID: modelUUID,
	}, nil).Times(2)

	modelInfo, err := hostedAPI.modelInfoService.GetModelInfo(ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.mockSecretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), modelInfo.UUID).Return(&provider.ModelBackendConfigInfo{}, nil)

	err = hostedAPI.RemoveModel(ctx)
	c.Assert(err, gc.ErrorMatches, "model not dying or dead")
}

func (s *undertakerSuite) TestRemoveDyingModel(c *gc.C) {
	ctx := context.Background()
	otherSt, hostedAPI, ctrl := s.setupStateAndAPI(c, false, "hostedmodel")
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		UUID: modelUUID,
	}, nil).Times(2)

	modelInfo, err := hostedAPI.modelInfoService.GetModelInfo(ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.mockSecretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), modelInfo.UUID).Return(&provider.ModelBackendConfigInfo{}, nil)
	// Set model to dying
	otherSt.model.life = state.Dying

	c.Assert(hostedAPI.RemoveModel(ctx), jc.ErrorIsNil)
}

func (s *undertakerSuite) TestDeadRemoveModel(c *gc.C) {
	ctx := context.Background()
	otherSt, hostedAPI, ctrl := s.setupStateAndAPI(c, false, "hostedmodel")
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		UUID: modelUUID,
	}, nil).Times(2)

	modelInfo, err := hostedAPI.modelInfoService.GetModelInfo(ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.mockSecretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), modelInfo.UUID).Return(&provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ModelUUID: modelUUID.String(),
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
				},
			},
		},
	}, nil)

	// Set model to dead
	otherSt.model.life = state.Dying
	err = hostedAPI.ProcessDyingModel(ctx)
	c.Assert(err, gc.IsNil)

	err = hostedAPI.RemoveModel(ctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(otherSt.removed, jc.IsTrue)

	c.Assert(s.secrets.cleanedUUID, gc.Equals, modelInfo.UUID.String())
}

func (s *undertakerSuite) TestDeadRemoveModelSecretsConfigNotFound(c *gc.C) {
	ctx := context.Background()
	otherSt, hostedAPI, ctrl := s.setupStateAndAPI(c, false, "hostedmodel")
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		UUID: modelUUID,
	}, nil).Times(2)

	modelInfo, err := hostedAPI.modelInfoService.GetModelInfo(ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.mockSecretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), modelInfo.UUID).Return(nil, secretbackenderrors.NotFound)
	// Set model to dead
	otherSt.model.life = state.Dying
	err = hostedAPI.ProcessDyingModel(ctx)
	c.Assert(err, gc.IsNil)

	err = hostedAPI.RemoveModel(ctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(otherSt.removed, jc.IsTrue)
	c.Assert(s.secrets.cleanedUUID, gc.Equals, "")
}

func (s *undertakerSuite) TestModelConfig(c *gc.C) {
	ctx := context.Background()
	_, hostedAPI, _ := s.setupStateAndAPI(c, false, "hostedmodel")

	expectedCfg, err := config.New(false, coretesting.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)
	s.mockModelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(expectedCfg, nil)

	cfg, err := hostedAPI.ModelConfig(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, gc.NotNil)
}

func (s *undertakerSuite) TestCloudSpec(c *gc.C) {
	ctx := context.Background()
	_, hostedAPI, _ := s.setupStateAndAPI(c, false, "hostedmodel")

	cred := cloud.NewCredential("userpass", map[string]string{"user": "fred", "password": "secret"})
	result := environscloudspec.CloudSpec{
		Name:       "cloud",
		Credential: &cred,
	}
	s.mockCloudSpecGetter.EXPECT().GetCloudSpec(gomock.Any()).Return(result, nil)

	got, err := hostedAPI.CloudSpec(ctx, params.Entities{Entities: []params.Entity{{
		Tag: names.NewModelTag(s.modelUUID.String()).String(),
	}, {
		Tag: names.NewModelTag(modeltesting.GenModelUUID(c).String()).String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, params.CloudSpecResults{
		Results: []params.CloudSpecResult{{
			Result: &params.CloudSpec{
				Name: "cloud",
				Credential: &params.CloudCredential{
					AuthType:   "userpass",
					Attributes: map[string]string{"user": "fred", "password": "secret"},
				},
			},
		}, {
			Error: &params.Error{
				Code:    params.CodeUnauthorized,
				Message: "permission denied",
			},
		}}})
}
