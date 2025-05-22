// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

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

func TestUndertakerSuite(t *testing.T) {
	tc.Run(t, &undertakerSuite{})
}

func (s *undertakerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *undertakerSuite) setupStateAndAPI(c *tc.C, isSystem bool, modelName string) (*mockState, *UndertakerAPI, *gomock.Controller) {
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
	c.Assert(err, tc.ErrorIsNil)
	return st, api, ctrl
}

func (s *undertakerSuite) TestNoPerms(c *tc.C) {
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
		c.Assert(err, tc.ErrorMatches, "permission denied")
	}
}

func (s *undertakerSuite) TestModelInfo(c *tc.C) {
	ctx := c.Context()
	name, err := user.NewName("user-admin")
	c.Assert(err, tc.ErrorIsNil)

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
		c.Assert(err, tc.ErrorIsNil)

		info := result.Result
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(result.Error, tc.IsNil)

		modelInfo, err := test.api.modelInfoService.GetModelInfo(ctx)
		c.Assert(err, tc.ErrorIsNil)

		c.Assert(info.UUID, tc.Equals, modelInfo.UUID.String())
		c.Assert(info.Name, tc.Equals, modelInfo.Name)
		c.Assert(info.IsSystem, tc.Equals, test.isSystem)
		c.Assert(info.Life, tc.Equals, life.Dying)
		c.Assert(info.ForceDestroyed, tc.Equals, true)
		c.Assert(info.DestroyTimeout, tc.NotNil)
		c.Assert(*info.DestroyTimeout, tc.Equals, time.Minute)
	}
}

func (s *undertakerSuite) TestProcessDyingModel(c *tc.C) {
	ctx := c.Context()
	otherSt, hostedAPI, _ := s.setupStateAndAPI(c, false, "hostedmodel")
	model, err := otherSt.Model()
	c.Assert(err, tc.ErrorIsNil)

	err = hostedAPI.ProcessDyingModel(ctx)
	c.Assert(err, tc.ErrorMatches, "model is not dying")
	c.Assert(model.Life(), tc.Equals, state.Alive)

	otherSt.model.life = state.Dying
	err = hostedAPI.ProcessDyingModel(ctx)
	c.Assert(err, tc.IsNil)
	c.Assert(model.Life(), tc.Equals, state.Dead)
}

func (s *undertakerSuite) TestRemoveAliveModel(c *tc.C) {
	ctx := c.Context()
	_, hostedAPI, ctrl := s.setupStateAndAPI(c, false, "hostedmodel")
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		UUID: modelUUID,
	}, nil).Times(2)

	modelInfo, err := hostedAPI.modelInfoService.GetModelInfo(ctx)
	c.Assert(err, tc.ErrorIsNil)

	s.mockSecretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), modelInfo.UUID).Return(&provider.ModelBackendConfigInfo{}, nil)

	err = hostedAPI.RemoveModel(ctx)
	c.Assert(err, tc.ErrorMatches, "model not dying or dead")
}

func (s *undertakerSuite) TestRemoveDyingModel(c *tc.C) {
	ctx := c.Context()
	otherSt, hostedAPI, ctrl := s.setupStateAndAPI(c, false, "hostedmodel")
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		UUID: modelUUID,
	}, nil).Times(2)

	modelInfo, err := hostedAPI.modelInfoService.GetModelInfo(ctx)
	c.Assert(err, tc.ErrorIsNil)

	s.mockSecretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), modelInfo.UUID).Return(&provider.ModelBackendConfigInfo{}, nil)
	// Set model to dying
	otherSt.model.life = state.Dying

	c.Assert(hostedAPI.RemoveModel(ctx), tc.ErrorIsNil)
}

func (s *undertakerSuite) TestDeadRemoveModel(c *tc.C) {
	ctx := c.Context()
	otherSt, hostedAPI, ctrl := s.setupStateAndAPI(c, false, "hostedmodel")
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		UUID: modelUUID,
	}, nil).Times(2)

	modelInfo, err := hostedAPI.modelInfoService.GetModelInfo(ctx)
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.IsNil)

	err = hostedAPI.RemoveModel(ctx)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(otherSt.removed, tc.IsTrue)

	c.Assert(s.secrets.cleanedUUID, tc.Equals, modelInfo.UUID.String())
}

func (s *undertakerSuite) TestDeadRemoveModelSecretsConfigNotFound(c *tc.C) {
	ctx := c.Context()
	otherSt, hostedAPI, ctrl := s.setupStateAndAPI(c, false, "hostedmodel")
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		UUID: modelUUID,
	}, nil).Times(2)

	modelInfo, err := hostedAPI.modelInfoService.GetModelInfo(ctx)
	c.Assert(err, tc.ErrorIsNil)

	s.mockSecretBackendService.EXPECT().GetSecretBackendConfigForAdmin(gomock.Any(), modelInfo.UUID).Return(nil, secretbackenderrors.NotFound)
	// Set model to dead
	otherSt.model.life = state.Dying
	err = hostedAPI.ProcessDyingModel(ctx)
	c.Assert(err, tc.IsNil)

	err = hostedAPI.RemoveModel(ctx)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(otherSt.removed, tc.IsTrue)
	c.Assert(s.secrets.cleanedUUID, tc.Equals, "")
}

func (s *undertakerSuite) TestModelConfig(c *tc.C) {
	ctx := c.Context()
	_, hostedAPI, _ := s.setupStateAndAPI(c, false, "hostedmodel")

	expectedCfg, err := config.New(false, coretesting.FakeConfig())
	c.Assert(err, tc.ErrorIsNil)
	s.mockModelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(expectedCfg, nil)

	cfg, err := hostedAPI.ModelConfig(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg, tc.NotNil)
}

func (s *undertakerSuite) TestCloudSpec(c *tc.C) {
	ctx := c.Context()
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, params.CloudSpecResults{
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
