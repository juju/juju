// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/modelconfig"
	"github.com/juju/juju/apiserver/facades/client/modelconfig/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/constraints"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/featureflag"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type modelconfigSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite
	backend                       *mockBackend
	authorizer                    apiservertesting.FakeAuthorizer
	mockModelSecretBackendService *mocks.MockModelSecretBackendService
	mockModelConfigService        *mocks.MockModelConfigService
	mockModelService              *mocks.MockModelService
	mockBlockCommandService       *mocks.MockBlockCommandService
}

var _ = gc.Suite(&modelconfigSuite{})

func (s *modelconfigSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(featureflag.DeveloperMode)
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      names.NewUserTag("bruce@local"),
		AdminTag: names.NewUserTag("bruce@local"),
	}
	s.backend = &mockBackend{
		secretBackend: &coresecrets.SecretBackend{
			ID:          "backend-1",
			Name:        "backend-1",
			BackendType: "vault",
			Config: map[string]interface{}{
				"endpoint": "http://0.0.0.0:8200",
			},
		},
	}
}

func (s *modelconfigSuite) getAPI(c *gc.C) (*modelconfig.ModelConfigAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.mockModelSecretBackendService = mocks.NewMockModelSecretBackendService(ctrl)
	s.mockModelConfigService = mocks.NewMockModelConfigService(ctrl)
	s.mockBlockCommandService = mocks.NewMockBlockCommandService(ctrl)
	s.mockModelService = mocks.NewMockModelService(ctrl)

	s.mockModelConfigService.EXPECT().ModelConfigValues(gomock.Any()).Return(
		config.ConfigValues{
			"type":          {Value: "dummy", Source: "model"},
			"agent-version": {Value: "1.2.3.4", Source: "model"},
			"ftp-proxy":     {Value: "http://proxy", Source: "model"},
			"charmhub-url":  {Value: "http://meshuggah.rocks", Source: "model"},
		}, nil,
	).AnyTimes()

	modelID := modeltesting.GenModelUUID(c)
	controllerUUID := uuid.MustNewUUID().String()
	api, err := modelconfig.NewModelConfigAPI(
		modelID, controllerUUID, s.backend,
		s.mockModelSecretBackendService, s.mockModelConfigService, s.mockModelService,
		&s.authorizer, s.mockBlockCommandService,
	)
	c.Assert(err, jc.ErrorIsNil)
	return api, ctrl
}

func (s *modelconfigSuite) TestAdminModelGet(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	result, err := api.ModelGet(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config, jc.DeepEquals, map[string]params.ConfigValue{
		"type":          {Value: "dummy", Source: "model"},
		"ftp-proxy":     {Value: "http://proxy", Source: "model"},
		"agent-version": {Value: "1.2.3.4", Source: "model"},
		"charmhub-url":  {Value: "http://meshuggah.rocks", Source: "model"},
	})
}

func (s *modelconfigSuite) TestUserModelGet(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:         names.NewUserTag("bruce@local"),
		HasWriteTag: names.NewUserTag("bruce@local"),
		AdminTag:    names.NewUserTag("mary@local"),
	}
	result, err := api.ModelGet(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Config, jc.DeepEquals, map[string]params.ConfigValue{
		"type":          {Value: "dummy", Source: "model"},
		"ftp-proxy":     {Value: "http://proxy", Source: "model"},
		"agent-version": {Value: "1.2.3.4", Source: "model"},
		"charmhub-url":  {Value: "http://meshuggah.rocks", Source: "model"},
	})
}

func (s *modelconfigSuite) TestAdminModelSet(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	params := params.ModelSet{
		Config: map[string]interface{}{
			"some-key":  "value",
			"other-key": "other value",
		},
	}
	s.mockModelConfigService.EXPECT().UpdateModelConfig(
		gomock.Any(),
		map[string]any{
			"some-key":  "value",
			"other-key": "other value",
		},
		nil,
		gomock.Any(),
	).Return(nil)
	err := api.ModelSet(context.Background(), params)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) assertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), jc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *modelconfigSuite) assertModelSetBlocked(c *gc.C, args map[string]interface{}, msg string) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return(msg, nil)

	err := api.ModelSet(context.Background(), params.ModelSet{Config: args})
	s.assertBlocked(c, err, msg)
}

func (s *modelconfigSuite) TestBlockChangesModelSet(c *gc.C) {
	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("TestBlockChangesModelSet", nil)
	args := map[string]interface{}{"some-key": "value"}
	s.assertModelSetBlocked(c, args, "TestBlockChangesModelSet")
}

func (s *modelconfigSuite) TestAdminCanSetLogTrace(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	oldConfig, err := config.New(config.NoDefaults, map[string]any{
		config.UUIDKey:   modelUUID.String(),
		config.NameKey:   "test-model",
		config.TypeKey:   "caas",
		"logging-config": "<root>=DEBUG",
	})
	c.Assert(err, jc.ErrorIsNil)

	newConfig, err := config.New(config.NoDefaults, map[string]any{
		config.UUIDKey:   modelUUID.String(),
		config.NameKey:   "test-model",
		config.TypeKey:   "caas",
		"logging-config": "<root>=DEBUG;somepackage=TRACE",
	})
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := modelconfig.LogTracingValidator(true)(context.Background(), newConfig, oldConfig)
	c.Check(err, jc.ErrorIsNil)
	c.Check(cfg.AllAttrs(), jc.DeepEquals, newConfig.AllAttrs())
}

func (s *modelconfigSuite) TestUserCanSetLogNoTrace(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	oldConfig, err := config.New(config.NoDefaults, map[string]any{
		config.UUIDKey:   modelUUID.String(),
		config.NameKey:   "test-model",
		config.TypeKey:   "caas",
		"logging-config": "<root>=DEBUG",
	})
	c.Assert(err, jc.ErrorIsNil)

	newConfig, err := config.New(config.NoDefaults, map[string]any{
		config.UUIDKey:   modelUUID.String(),
		config.NameKey:   "test-model",
		config.TypeKey:   "caas",
		"logging-config": "<root>=DEBUG;somepackage=ERROR",
	})
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := modelconfig.LogTracingValidator(true)(context.Background(), newConfig, oldConfig)
	c.Check(err, jc.ErrorIsNil)
	c.Check(cfg.AllAttrs(), jc.DeepEquals, newConfig.AllAttrs())
}

func (s *modelconfigSuite) TestUserReadAccess(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	apiUser := names.NewUserTag("read")
	s.authorizer.Tag = apiUser

	_, err := api.ModelGet(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = api.ModelSet(context.Background(), params.ModelSet{})
	c.Assert(errors.Cause(err), gc.ErrorMatches, "permission denied")
}

func (s *modelconfigSuite) TestUserCannotSetLogTrace(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	oldConfig, err := config.New(config.NoDefaults, map[string]any{
		config.UUIDKey:   modelUUID.String(),
		config.NameKey:   "test-model",
		config.TypeKey:   "caas",
		"logging-config": "<root>=DEBUG",
	})
	c.Assert(err, jc.ErrorIsNil)

	newConfig, err := config.New(config.NoDefaults, map[string]any{
		config.UUIDKey:   modelUUID.String(),
		config.NameKey:   "test-model",
		config.TypeKey:   "caas",
		"logging-config": "<root>=DEBUG;somepackage=TRACE",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = modelconfig.LogTracingValidator(false)(context.Background(), newConfig, oldConfig)
	var validationErr *config.ValidationError
	c.Check(errors.As(err, &validationErr), jc.IsTrue)
	c.Check(*validationErr, jc.DeepEquals, config.ValidationError{
		InvalidAttrs: []string{config.LoggingConfigKey},
		Reason:       "only controller admins can set a model's logging level to TRACE",
	})
}

func (s *modelconfigSuite) TestModelUnset(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	s.mockModelConfigService.EXPECT().UpdateModelConfig(
		gomock.Any(),
		nil,
		[]string{"abc"},
		gomock.Any(),
	).Return(nil)

	args := params.ModelUnset{Keys: []string{"abc"}}
	err := api.ModelUnset(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestBlockModelUnset(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("TestBlockModelUnset", nil)

	args := params.ModelUnset{Keys: []string{"abc"}}
	err := api.ModelUnset(context.Background(), args)
	s.assertBlocked(c, err, "TestBlockModelUnset")
}

func (s *modelconfigSuite) TestModelUnsetMissing(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	// It's okay to unset a non-existent attribute.
	s.mockModelConfigService.EXPECT().UpdateModelConfig(
		gomock.Any(),
		nil,
		[]string{"not_there"},
		gomock.Any(),
	)
	args := params.ModelUnset{Keys: []string{"not_there"}}
	err := api.ModelUnset(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestClientSetModelConstraints(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)

	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
	s.mockModelService.EXPECT().SetModelConstraints(gomock.Any(), cons).Return(nil)

	err = api.SetModelConstraints(context.Background(), params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelconfigSuite) TestClientSetModelConstraintsFailedModelNotFound(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)

	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
	s.mockModelService.EXPECT().SetModelConstraints(gomock.Any(), cons).Return(modelerrors.NotFound)

	err = api.SetModelConstraints(context.Background(), params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	c.Assert(err, jc.Satisfies, params.IsCodeModelNotFound)
}

func (s *modelconfigSuite) TestClientSetModelConstraintsFailedSpaceNotFound(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)

	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
	s.mockModelService.EXPECT().SetModelConstraints(gomock.Any(), cons).Return(networkerrors.SpaceNotFound)

	err = api.SetModelConstraints(context.Background(), params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
}

func (s *modelconfigSuite) TestClientSetModelConstraintsFailedInvalidContainerType(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)

	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
	s.mockModelService.EXPECT().SetModelConstraints(gomock.Any(), cons).Return(machineerrors.InvalidContainerType)

	err = api.SetModelConstraints(context.Background(), params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	c.Assert(params.ErrCode(err), gc.Equals, params.CodeNotValid)
}

func (s *modelconfigSuite) assertSetModelConstraintsBlocked(c *gc.C, msg string) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)

	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("TestBlockChangesClientSetModelConstraints", nil)

	err = api.SetModelConstraints(context.Background(), params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	s.assertBlocked(c, err, msg)
}

func (s *modelconfigSuite) TestBlockChangesClientSetModelConstraints(c *gc.C) {
	s.assertSetModelConstraintsBlocked(c, "TestBlockChangesClientSetModelConstraints")
}

func (s *modelconfigSuite) TestClientGetModelConstraints(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)

	s.mockModelService.EXPECT().GetModelConstraints(gomock.Any()).Return(cons, nil)

	obtained, err := api.GetModelConstraints(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained.Constraints, gc.DeepEquals, cons)
}

func (s *modelconfigSuite) TestClientGetModelConstraintsFailedModelNotFound(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	s.mockModelService.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Value{}, modelerrors.NotFound)

	_, err := api.GetModelConstraints(context.Background())
	c.Assert(err, jc.Satisfies, params.IsCodeModelNotFound)
}

type modelSecretBackendSuite struct {
	testing.IsolationSuite

	authorizer                    *facademocks.MockAuthorizer
	mockModelSecretBackendService *mocks.MockModelSecretBackendService
	mockBlockCommandService       *mocks.MockBlockCommandService
	modelID                       coremodel.UUID
	controllerUUID                string
}

var _ = gc.Suite(&modelSecretBackendSuite{})

func (s *modelSecretBackendSuite) setup(c *gc.C) (*modelconfig.ModelConfigAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.authorizer.EXPECT().AuthClient().Return(true)
	s.mockModelSecretBackendService = mocks.NewMockModelSecretBackendService(ctrl)
	s.mockBlockCommandService = mocks.NewMockBlockCommandService(ctrl)
	s.modelID = modeltesting.GenModelUUID(c)
	s.controllerUUID = uuid.MustNewUUID().String()

	api, err := modelconfig.NewModelConfigAPI(s.modelID, s.controllerUUID, nil, s.mockModelSecretBackendService, nil, nil, s.authorizer, s.mockBlockCommandService)
	c.Assert(err, jc.ErrorIsNil)
	return api, ctrl
}

func (s *modelSecretBackendSuite) TestGetModelSecretBackendFailedPermissionDenied(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, names.NewModelTag(s.modelID.String())).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission),
	)

	_, err := facade.GetModelSecretBackend(context.Background())
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.ErrorIs, authentication.ErrorEntityMissingPermission)
}

func (s *modelSecretBackendSuite) TestGetModelSecretBackendFailedModelNotFound(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, names.NewModelTag(s.modelID.String())).Return(nil)
	s.mockModelSecretBackendService.EXPECT().GetModelSecretBackend(gomock.Any()).Return("", modelerrors.NotFound)

	result, err := facade.GetModelSecretBackend(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.ErrorMatches, "model not found")
	c.Assert(result.Error.Code, gc.Equals, params.CodeModelNotFound)
}

func (s *modelSecretBackendSuite) TestGetModelSecretBackend(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, names.NewModelTag(s.modelID.String())).Return(nil)
	s.mockModelSecretBackendService.EXPECT().GetModelSecretBackend(gomock.Any()).Return("myvault", nil)

	result, err := facade.GetModelSecretBackend(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.Equals, "myvault")
}

func (s *modelSecretBackendSuite) TestSetModelSecretBackendFailedPermissionDenied(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelID.String())).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission),
	)

	_, err := facade.SetModelSecretBackend(context.Background(), params.SetModelSecretBackendArg{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.ErrorIs, authentication.ErrorEntityMissingPermission)
}

func (s *modelSecretBackendSuite) TestSetModelSecretBackendFailedModelNotFound(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelID.String())).Return(nil)
	s.mockModelSecretBackendService.EXPECT().SetModelSecretBackend(gomock.Any(), "myvault").Return(modelerrors.NotFound)

	result, err := facade.SetModelSecretBackend(context.Background(), params.SetModelSecretBackendArg{
		SecretBackendName: "myvault",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.ErrorMatches, "model not found")
	c.Assert(result.Error.Code, gc.Equals, params.CodeModelNotFound)
}

func (s *modelSecretBackendSuite) TestSetModelSecretBackendFailedSecretBackendNotFound(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelID.String())).Return(nil)
	s.mockModelSecretBackendService.EXPECT().SetModelSecretBackend(gomock.Any(), "myvault").Return(secretbackenderrors.NotFound)

	result, err := facade.SetModelSecretBackend(context.Background(), params.SetModelSecretBackendArg{
		SecretBackendName: "myvault",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.ErrorMatches, "secret backend not found")
	c.Assert(result.Error.Code, gc.Equals, params.CodeSecretBackendNotFound)
}

func (s *modelSecretBackendSuite) TestSetModelSecretBackendFailedSecretBackendNotValid(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelID.String())).Return(nil)
	s.mockModelSecretBackendService.EXPECT().SetModelSecretBackend(gomock.Any(), "myvault").Return(secretbackenderrors.NotValid)

	result, err := facade.SetModelSecretBackend(context.Background(), params.SetModelSecretBackendArg{
		SecretBackendName: "myvault",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.ErrorMatches, "secret backend not valid")
	c.Assert(result.Error.Code, gc.Equals, params.CodeSecretBackendNotValid)
}

func (s *modelSecretBackendSuite) TestSetModelSecretBackend(c *gc.C) {
	facade, ctrl := s.setup(c)
	defer ctrl.Finish()

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelID.String())).Return(nil)
	s.mockModelSecretBackendService.EXPECT().SetModelSecretBackend(gomock.Any(), "myvault").Return(nil)

	result, err := facade.SetModelSecretBackend(context.Background(), params.SetModelSecretBackendArg{
		SecretBackendName: "myvault",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

type mockBackend struct {
	secretBackend *coresecrets.SecretBackend
}

func (m *mockBackend) Sequences() (map[string]int, error) {
	return nil, nil
}
