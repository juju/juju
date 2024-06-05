// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/modelconfig"
	"github.com/juju/juju/apiserver/facades/client/modelconfig/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/constraints"
	modeltesting "github.com/juju/juju/core/model/testing"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/featureflag"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type modelconfigSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite
	backend                  *mockBackend
	authorizer               apiservertesting.FakeAuthorizer
	mockSecretBackendService *mocks.MockSecretBackendService
	mockModelConfigService   *mocks.MockModelConfigService
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
		cfg: config.ConfigValues{
			"type":            {Value: "dummy", Source: "model"},
			"agent-version":   {Value: "1.2.3.4", Source: "model"},
			"ftp-proxy":       {Value: "http://proxy", Source: "model"},
			"authorized-keys": {Value: coretesting.FakeAuthKeys, Source: "model"},
			"charmhub-url":    {Value: "http://meshuggah.rocks", Source: "model"},
		},
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

func (s *modelconfigSuite) getAPI(c *gc.C) (*modelconfig.ModelConfigAPIV3, *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.mockSecretBackendService = mocks.NewMockSecretBackendService(ctrl)
	s.mockModelConfigService = mocks.NewMockModelConfigService(ctrl)

	s.mockModelConfigService.EXPECT().ModelConfigValues(gomock.Any()).Return(
		config.ConfigValues{
			"type":            {Value: "dummy", Source: "model"},
			"agent-version":   {Value: "1.2.3.4", Source: "model"},
			"ftp-proxy":       {Value: "http://proxy", Source: "model"},
			"authorized-keys": {Value: coretesting.FakeAuthKeys, Source: "model"},
			"charmhub-url":    {Value: "http://meshuggah.rocks", Source: "model"},
		}, nil,
	).AnyTimes()

	api, err := modelconfig.NewModelConfigAPI(s.backend, s.mockSecretBackendService, s.mockModelConfigService, &s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	return api, ctrl
}

func (s *modelconfigSuite) TestAdminModelGet(c *gc.C) {
	api, _ := s.getAPI(c)

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
	api, _ := s.getAPI(c)
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
	api, _ := s.getAPI(c)
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

func (s *modelconfigSuite) blockAllChanges(msg string) {
	s.backend.msg = msg
	s.backend.b = state.ChangeBlock
}

func (s *modelconfigSuite) assertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), jc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *modelconfigSuite) assertModelSetBlocked(c *gc.C, args map[string]interface{}, msg string) {
	api, _ := s.getAPI(c)
	err := api.ModelSet(context.Background(), params.ModelSet{Config: args})
	s.assertBlocked(c, err, msg)
}

func (s *modelconfigSuite) TestBlockChangesModelSet(c *gc.C) {
	s.blockAllChanges("TestBlockChangesModelSet")
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
	api, _ := s.getAPI(c)
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
		Cause:        errors.ConstError("only controller admins can set a model's logging level to TRACE"),
	})
}

func (s *modelconfigSuite) TestModelUnset(c *gc.C) {
	api, _ := s.getAPI(c)

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
	api, _ := s.getAPI(c)
	s.blockAllChanges("TestBlockModelUnset")

	args := params.ModelUnset{Keys: []string{"abc"}}
	err := api.ModelUnset(context.Background(), args)
	s.assertBlocked(c, err, "TestBlockModelUnset")
}

func (s *modelconfigSuite) TestModelUnsetMissing(c *gc.C) {
	api, _ := s.getAPI(c)
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
	api, _ := s.getAPI(c)
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = api.SetModelConstraints(context.Background(), params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.backend.cons, gc.DeepEquals, cons)
}

func (s *modelconfigSuite) assertSetModelConstraintsBlocked(c *gc.C, msg string) {
	api, _ := s.getAPI(c)
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = api.SetModelConstraints(context.Background(), params.SetConstraints{
		ApplicationName: "app",
		Constraints:     cons,
	})
	s.assertBlocked(c, err, msg)
}

func (s *modelconfigSuite) TestBlockChangesClientSetModelConstraints(c *gc.C) {
	s.blockAllChanges("TestBlockChangesClientSetModelConstraints")
	s.assertSetModelConstraintsBlocked(c, "TestBlockChangesClientSetModelConstraints")
}

func (s *modelconfigSuite) TestClientGetModelConstraints(c *gc.C) {
	api, _ := s.getAPI(c)
	// Set constraints for the model.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	s.backend.cons = cons
	obtained, err := api.GetModelConstraints(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained.Constraints, gc.DeepEquals, cons)
}

type mockBackend struct {
	cfg           config.ConfigValues
	old           *config.Config
	b             state.BlockType
	msg           string
	cons          constraints.Value
	secretBackend *coresecrets.SecretBackend
}

func (m *mockBackend) SetModelConstraints(value constraints.Value) error {
	m.cons = value
	return nil
}

func (m *mockBackend) ModelConstraints() (constraints.Value, error) {
	return m.cons, nil
}

func (m *mockBackend) ModelConfigValues() (config.ConfigValues, error) {
	return m.cfg, nil
}

func (m *mockBackend) Sequences() (map[string]int, error) {
	return nil, nil
}

func (m *mockBackend) UpdateModelConfig(update map[string]interface{}, remove []string,
	validate ...state.ValidateConfigFunc) error {
	for _, validateFunc := range validate {
		if err := validateFunc(update, remove, m.old); err != nil {
			return err
		}
	}
	for k, v := range update {
		m.cfg[k] = config.ConfigValue{Value: v, Source: "model"}
	}
	for _, n := range remove {
		delete(m.cfg, n)
	}
	return nil
}

func (m *mockBackend) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	if m.b == t {
		return &mockBlock{t: t, m: m.msg}, true, nil
	} else {
		return nil, false, nil
	}
}

func (m *mockBackend) ModelTag() names.ModelTag {
	return names.NewModelTag("deadbeef-2f18-4fd2-967d-db9663db7bea")
}

func (m *mockBackend) ControllerTag() names.ControllerTag {
	return names.NewControllerTag("deadbeef-babe-4fd2-967d-db9663db7bea")
}

func (m *mockBackend) SpaceByName(string) error {
	return nil
}

func (m *mockBackend) GetSecretBackend(name string) (*coresecrets.SecretBackend, error) {
	if name == "invalid" {
		return nil, errors.NotFoundf("invalid")
	}
	return m.secretBackend, nil
}

type mockBlock struct {
	state.Block
	t state.BlockType
	m string
}

func (m mockBlock) Id() string { return "" }

func (m mockBlock) Tag() (names.Tag, error) { return names.NewModelTag("mocktesting"), nil }

func (m mockBlock) Type() state.BlockType { return m.t }

func (m mockBlock) Message() string { return m.m }

func (m mockBlock) ModelUUID() string { return "" }
