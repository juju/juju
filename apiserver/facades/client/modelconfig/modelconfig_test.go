// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type modelconfigSuite struct {
	authorizer                    *facademocks.MockAuthorizer
	mockModelAgentService         *MockModelAgentService
	mockModelConfigService        *MockModelConfigService
	mockModelSecretBackendService *MockModelSecretBackendService
	mockModelService              *MockModelService
	mockBlockCommandService       *MockBlockCommandService

	modelUUID      coremodel.UUID
	controllerUUID string
}

func TestModelconfigSuite(t *testing.T) {
	tc.Run(t, &modelconfigSuite{})
}

func (s *modelconfigSuite) SetUpTest(c *tc.C) {
	s.controllerUUID = uuid.MustNewUUID().String()
	s.modelUUID = modeltesting.GenModelUUID(c)
}

func (s *modelconfigSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.mockModelAgentService = NewMockModelAgentService(ctrl)
	s.mockModelConfigService = NewMockModelConfigService(ctrl)
	s.mockModelSecretBackendService = NewMockModelSecretBackendService(ctrl)
	s.mockModelService = NewMockModelService(ctrl)
	s.mockBlockCommandService = NewMockBlockCommandService(ctrl)
	return ctrl
}

func (s *modelconfigSuite) getAPI(c *tc.C) *ModelConfigAPI {
	s.mockModelConfigService.EXPECT().ModelConfigValues(gomock.Any()).Return(
		config.ConfigValues{
			"type":          {Value: "dummy", Source: "model"},
			"agent-version": {Value: "1.2.3.4", Source: "model"},
			"ftp-proxy":     {Value: "http://proxy", Source: "model"},
			"charmhub-url":  {Value: "http://meshuggah.rocks", Source: "model"},
		}, nil,
	).AnyTimes()

	api := NewModelConfigAPI(
		s.authorizer,
		s.controllerUUID,
		s.modelUUID,
		s.mockModelAgentService,
		s.mockBlockCommandService,
		s.mockModelConfigService,
		s.mockModelSecretBackendService,
		s.mockModelService,
		loggertesting.WrapCheckLog(c),
	)
	return api
}

func (s *modelconfigSuite) expectModelReadAccess() {
	gomock.InOrder(
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).
			Return(errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission)),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, names.NewModelTag(s.modelUUID.String())).
			Return(errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission)),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, names.NewModelTag(s.modelUUID.String())),
	)
}

func (s *modelconfigSuite) expectModelWriteAccess() {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelUUID.String()))
}

func (s *modelconfigSuite) expectModelAdminAccess() {
	gomock.InOrder(
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).
			Return(errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission)),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, names.NewModelTag(s.modelUUID.String())),
	)
}

func (s *modelconfigSuite) expectNoModelAdminAccess() {
	gomock.InOrder(
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).
			Return(errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission)),
		s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.AdminAccess, names.NewModelTag(s.modelUUID.String())).
			Return(errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission)),
	)
}

func (s *modelconfigSuite) expectNoControllerAdminAccess() {
	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).
		Return(errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))
}

func (s *modelconfigSuite) expectNoBlocks() {
	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
}

func (s *modelconfigSuite) TestModelGetModelAdmin(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	// Check read access.
	s.expectModelReadAccess()
	// Chck admin access.
	s.expectModelAdminAccess()

	result, err := api.ModelGet(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Config, tc.DeepEquals, map[string]params.ConfigValue{
		"type":          {Value: "dummy", Source: "model"},
		"ftp-proxy":     {Value: "http://proxy", Source: "model"},
		"agent-version": {Value: "1.2.3.4", Source: "model"},
		"charmhub-url":  {Value: "http://meshuggah.rocks", Source: "model"},
	})
}

func (s *modelconfigSuite) TestModelGetControllerAdmin(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID)).Times(2)

	result, err := api.ModelGet(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Config, tc.DeepEquals, map[string]params.ConfigValue{
		"type":          {Value: "dummy", Source: "model"},
		"ftp-proxy":     {Value: "http://proxy", Source: "model"},
		"agent-version": {Value: "1.2.3.4", Source: "model"},
		"charmhub-url":  {Value: "http://meshuggah.rocks", Source: "model"},
	})
}

func (s *modelconfigSuite) TestModelGetReadAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	s.expectModelReadAccess()
	s.expectNoModelAdminAccess()

	result, err := api.ModelGet(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Config, tc.DeepEquals, map[string]params.ConfigValue{
		"type":          {Value: "dummy", Source: "model"},
		"ftp-proxy":     {Value: "http://proxy", Source: "model"},
		"agent-version": {Value: "1.2.3.4", Source: "model"},
		"charmhub-url":  {Value: "http://meshuggah.rocks", Source: "model"},
	})
}

func (s *modelconfigSuite) TestModelSetModelAdmin(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	s.expectModelWriteAccess()
	s.expectModelAdminAccess()
	s.expectNoControllerAdminAccess()
	s.expectNoBlocks()

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
	)
	err := api.ModelSet(c.Context(), params)
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetModelConfigAgentStream tests that the agent stream can be set via
// model config and the value is correctly abstracted from config and removed.
func (s *modelconfigSuite) TestSetModelConfigAgentStream(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	s.expectModelWriteAccess()
	s.expectModelAdminAccess()
	s.expectNoControllerAdminAccess()
	s.expectNoBlocks()

	s.mockModelAgentService.EXPECT().SetModelAgentStream(
		gomock.Any(),
		coreagentbinary.AgentStreamReleased,
	).Return(nil)
	s.mockModelConfigService.EXPECT().UpdateModelConfig(
		gomock.Any(),
		map[string]any{},
		nil,
		gomock.Any(),
	).Return(nil)

	err := api.ModelSet(c.Context(), params.ModelSet{
		Config: map[string]any{
			"agent-stream": "released",
		},
	})
	c.Check(err, tc.ErrorIsNil)
}

// TestSetModelConfigAgentStreamInvalid tests that an invalid agent stream
// resultes in an error of not valid.
func (s *modelconfigSuite) TestSetModelConfigAgentStreamInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	s.expectModelWriteAccess()
	s.expectModelAdminAccess()
	s.expectNoControllerAdminAccess()
	s.expectNoBlocks()

	s.mockModelAgentService.EXPECT().SetModelAgentStream(
		gomock.Any(),
		coreagentbinary.AgentStream("invalid"),
	).Return(coreerrors.NotValid)

	err := api.ModelSet(c.Context(), params.ModelSet{
		Config: map[string]any{
			"agent-stream": "invalid",
		},
	})
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *modelconfigSuite) assertBlocked(c *tc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue, tc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), tc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *modelconfigSuite) assertModelSetBlocked(c *tc.C, args map[string]interface{}, msg string) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	s.expectModelWriteAccess()
	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return(msg, nil)

	err := api.ModelSet(c.Context(), params.ModelSet{Config: args})
	s.assertBlocked(c, err, msg)
}

func (s *modelconfigSuite) TestBlockChangesModelSet(c *tc.C) {
	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("TestBlockChangesModelSet", nil)
	args := map[string]interface{}{"some-key": "value"}
	s.assertModelSetBlocked(c, args, "TestBlockChangesModelSet")
}

func (s *modelconfigSuite) TestAdminCanSetLogTrace(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	oldConfig, err := config.New(config.NoDefaults, map[string]any{
		config.UUIDKey:   modelUUID.String(),
		config.NameKey:   "test-model",
		config.TypeKey:   "caas",
		"logging-config": "<root>=DEBUG",
	})
	c.Assert(err, tc.ErrorIsNil)

	newConfig, err := config.New(config.NoDefaults, map[string]any{
		config.UUIDKey:   modelUUID.String(),
		config.NameKey:   "test-model",
		config.TypeKey:   "caas",
		"logging-config": "<root>=DEBUG;somepackage=TRACE",
	})
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := LogTracingValidator(true)(c.Context(), newConfig, oldConfig)
	c.Check(err, tc.ErrorIsNil)
	c.Check(cfg.AllAttrs(), tc.DeepEquals, newConfig.AllAttrs())
}

func (s *modelconfigSuite) TestUserCanSetLogNoTrace(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	oldConfig, err := config.New(config.NoDefaults, map[string]any{
		config.UUIDKey:   modelUUID.String(),
		config.NameKey:   "test-model",
		config.TypeKey:   "caas",
		"logging-config": "<root>=DEBUG",
	})
	c.Assert(err, tc.ErrorIsNil)

	newConfig, err := config.New(config.NoDefaults, map[string]any{
		config.UUIDKey:   modelUUID.String(),
		config.NameKey:   "test-model",
		config.TypeKey:   "caas",
		"logging-config": "<root>=DEBUG;somepackage=ERROR",
	})
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := LogTracingValidator(true)(c.Context(), newConfig, oldConfig)
	c.Check(err, tc.ErrorIsNil)
	c.Check(cfg.AllAttrs(), tc.DeepEquals, newConfig.AllAttrs())
}

func (s *modelconfigSuite) TestModelSetNoWriteAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelUUID.String())).
		Return(errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission))

	err := api.ModelSet(c.Context(), params.ModelSet{})
	c.Assert(errors.Cause(err), tc.ErrorMatches, "permission denied")
}

func (s *modelconfigSuite) TestUserCannotSetLogTrace(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	oldConfig, err := config.New(config.NoDefaults, map[string]any{
		config.UUIDKey:   modelUUID.String(),
		config.NameKey:   "test-model",
		config.TypeKey:   "caas",
		"logging-config": "<root>=DEBUG",
	})
	c.Assert(err, tc.ErrorIsNil)

	newConfig, err := config.New(config.NoDefaults, map[string]any{
		config.UUIDKey:   modelUUID.String(),
		config.NameKey:   "test-model",
		config.TypeKey:   "caas",
		"logging-config": "<root>=DEBUG;somepackage=TRACE",
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = LogTracingValidator(false)(c.Context(), newConfig, oldConfig)
	var validationErr *config.ValidationError
	c.Check(errors.As(err, &validationErr), tc.IsTrue)
	c.Check(*validationErr, tc.DeepEquals, config.ValidationError{
		InvalidAttrs: []string{config.LoggingConfigKey},
		Reason:       "only controller admins can set a model's logging level to TRACE",
	})
}

func (s *modelconfigSuite) TestModelUnset(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	s.expectModelWriteAccess()
	s.expectNoBlocks()

	s.mockModelConfigService.EXPECT().UpdateModelConfig(
		gomock.Any(),
		nil,
		[]string{"abc"},
		gomock.Any(),
	)

	args := params.ModelUnset{Keys: []string{"abc"}}
	err := api.ModelUnset(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelconfigSuite) TestBlockModelUnset(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	s.expectModelWriteAccess()
	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("TestBlockModelUnset", nil)

	args := params.ModelUnset{Keys: []string{"abc"}}
	err := api.ModelUnset(c.Context(), args)
	s.assertBlocked(c, err, "TestBlockModelUnset")
}

func (s *modelconfigSuite) TestModelUnsetMissing(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	s.expectModelWriteAccess()
	s.expectNoBlocks()

	// It's okay to unset a non-existent attribute.
	s.mockModelConfigService.EXPECT().UpdateModelConfig(
		gomock.Any(),
		nil,
		[]string{"not_there"},
		gomock.Any(),
	)
	args := params.ModelUnset{Keys: []string{"not_there"}}
	err := api.ModelUnset(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelconfigSuite) TestClientSetModelConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, tc.ErrorIsNil)

	s.expectModelWriteAccess()
	s.expectNoBlocks()
	s.mockModelService.EXPECT().SetModelConstraints(gomock.Any(), cons)

	err = api.SetModelConstraints(c.Context(), params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelconfigSuite) TestClientSetModelConstraintsFailedModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, tc.ErrorIsNil)

	s.expectModelWriteAccess()
	s.expectNoBlocks()
	s.mockModelService.EXPECT().SetModelConstraints(gomock.Any(), cons).Return(modelerrors.NotFound)

	err = api.SetModelConstraints(c.Context(), params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	c.Assert(err, tc.Satisfies, params.IsCodeModelNotFound)
}

func (s *modelconfigSuite) TestClientSetModelConstraintsFailedSpaceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, tc.ErrorIsNil)

	s.expectModelWriteAccess()
	s.expectNoBlocks()
	s.mockModelService.EXPECT().SetModelConstraints(gomock.Any(), cons).Return(networkerrors.SpaceNotFound)

	err = api.SetModelConstraints(c.Context(), params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	c.Assert(err, tc.Satisfies, params.IsCodeNotFound)
}

func (s *modelconfigSuite) TestClientSetModelConstraintsFailedInvalidContainerType(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, tc.ErrorIsNil)

	s.expectModelWriteAccess()
	s.expectNoBlocks()
	s.mockModelService.EXPECT().SetModelConstraints(gomock.Any(), cons).Return(machineerrors.InvalidContainerType)

	err = api.SetModelConstraints(c.Context(), params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	c.Assert(params.ErrCode(err), tc.Equals, params.CodeNotValid)
}

func (s *modelconfigSuite) assertSetModelConstraintsBlocked(c *tc.C, msg string) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, tc.ErrorIsNil)

	s.expectModelWriteAccess()
	s.mockBlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("TestBlockChangesClientSetModelConstraints", nil)

	err = api.SetModelConstraints(c.Context(), params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	s.assertBlocked(c, err, msg)
}

func (s *modelconfigSuite) TestBlockChangesClientSetModelConstraints(c *tc.C) {
	s.assertSetModelConstraintsBlocked(c, "TestBlockChangesClientSetModelConstraints")
}

func (s *modelconfigSuite) TestClientGetModelConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	s.expectModelReadAccess()

	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, tc.ErrorIsNil)

	s.mockModelService.EXPECT().GetModelConstraints(gomock.Any()).Return(cons, nil)

	obtained, err := api.GetModelConstraints(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained.Constraints, tc.DeepEquals, cons)
}

func (s *modelconfigSuite) TestClientGetModelConstraintsFailedModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.getAPI(c)

	s.expectModelReadAccess()

	s.mockModelService.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Value{}, modelerrors.NotFound)

	_, err := api.GetModelConstraints(c.Context())
	c.Assert(err, tc.Satisfies, params.IsCodeModelNotFound)
}

func (s *modelconfigSuite) TestGetModelSecretBackendFailedPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()
	facade := s.getAPI(c)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, names.NewModelTag(s.modelUUID.String())).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission),
	)

	_, err := facade.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(err, tc.ErrorIs, authentication.ErrorEntityMissingPermission)
}

func (s *modelconfigSuite) TestGetModelSecretBackendFailedModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	facade := s.getAPI(c)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, names.NewModelTag(s.modelUUID.String()))
	s.mockModelSecretBackendService.EXPECT().GetModelSecretBackend(gomock.Any()).Return("", modelerrors.NotFound)

	result, err := facade.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.ErrorMatches, "model not found")
	c.Assert(result.Error.Code, tc.Equals, params.CodeModelNotFound)
}

func (s *modelconfigSuite) TestGetModelSecretBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()
	facade := s.getAPI(c)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.ReadAccess, names.NewModelTag(s.modelUUID.String()))
	s.mockModelSecretBackendService.EXPECT().GetModelSecretBackend(gomock.Any()).Return("myvault", nil)

	result, err := facade.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Result, tc.Equals, "myvault")
}

func (s *modelconfigSuite) TestSetModelSecretBackendFailedPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()
	facade := s.getAPI(c)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelUUID.String())).Return(
		errors.WithType(apiservererrors.ErrPerm, authentication.ErrorEntityMissingPermission),
	)

	_, err := facade.SetModelSecretBackend(c.Context(), params.SetModelSecretBackendArg{})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(err, tc.ErrorIs, authentication.ErrorEntityMissingPermission)
}

func (s *modelconfigSuite) TestSetModelSecretBackendFailedModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	facade := s.getAPI(c)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelUUID.String()))
	s.mockModelSecretBackendService.EXPECT().SetModelSecretBackend(gomock.Any(), "myvault").Return(modelerrors.NotFound)

	result, err := facade.SetModelSecretBackend(c.Context(), params.SetModelSecretBackendArg{
		SecretBackendName: "myvault",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.ErrorMatches, "model not found")
	c.Assert(result.Error.Code, tc.Equals, params.CodeModelNotFound)
}

func (s *modelconfigSuite) TestSetModelSecretBackendFailedSecretBackendNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	facade := s.getAPI(c)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelUUID.String()))
	s.mockModelSecretBackendService.EXPECT().SetModelSecretBackend(gomock.Any(), "myvault").Return(secretbackenderrors.NotFound)

	result, err := facade.SetModelSecretBackend(c.Context(), params.SetModelSecretBackendArg{
		SecretBackendName: "myvault",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.ErrorMatches, "secret backend not found")
	c.Assert(result.Error.Code, tc.Equals, params.CodeSecretBackendNotFound)
}

func (s *modelconfigSuite) TestSetModelSecretBackendFailedSecretBackendNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	facade := s.getAPI(c)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelUUID.String()))
	s.mockModelSecretBackendService.EXPECT().SetModelSecretBackend(gomock.Any(), "myvault").Return(secretbackenderrors.NotValid)

	result, err := facade.SetModelSecretBackend(c.Context(), params.SetModelSecretBackendArg{
		SecretBackendName: "myvault",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.ErrorMatches, "secret backend not valid")
	c.Assert(result.Error.Code, tc.Equals, params.CodeSecretBackendNotValid)
}

func (s *modelconfigSuite) TestSetModelSecretBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()
	facade := s.getAPI(c)

	s.authorizer.EXPECT().HasPermission(gomock.Any(), permission.WriteAccess, names.NewModelTag(s.modelUUID.String()))
	s.mockModelSecretBackendService.EXPECT().SetModelSecretBackend(gomock.Any(), "myvault")

	result, err := facade.SetModelSecretBackend(c.Context(), params.SetModelSecretBackendArg{
		SecretBackendName: "myvault",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
}
