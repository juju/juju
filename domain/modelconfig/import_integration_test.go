// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig_test

import (
	"context"
	"testing"

	"github.com/juju/description/v11"
	"github.com/juju/tc"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	"github.com/juju/juju/domain/modelconfig/modelmigration"
	"github.com/juju/juju/domain/modelconfig/service"
	"github.com/juju/juju/domain/modelconfig/state"
	modeldefaultsservice "github.com/juju/juju/domain/modeldefaults/service"
	modeldefaultsstate "github.com/juju/juju/domain/modeldefaults/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	_ "github.com/juju/juju/internal/provider/dummy"
)

type importSuite struct {
	schematesting.ControllerModelSuite

	coordinator *coremodelmigration.Coordinator
	scope       coremodelmigration.Scope
	svc         *service.Service
	modelUUID   model.UUID
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) SetUpTest(c *tc.C) {
	s.ControllerModelSuite.SetUpTest(c)

	controllerTxnFactory := func(context.Context) (coredatabase.TxnRunner, error) {
		return s.ControllerTxnRunner(), nil
	}
	s.modelUUID = modeltesting.CreateTestModel(c, controllerTxnFactory, "foo")

	modelTxnFactory := func(context.Context) (coredatabase.TxnRunner, error) {
		return s.ModelTxnRunner(c, s.modelUUID.String()), nil
	}

	s.coordinator = coremodelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	s.scope = coremodelmigration.NewScope(controllerTxnFactory, modelTxnFactory, nil, s.modelUUID)

	defaultsProvider := modeldefaultsservice.NewService(
		modeldefaultsservice.ProviderModelConfigGetter(),
		modeldefaultsstate.NewState(controllerTxnFactory),
	).ModelDefaultsProvider(s.modelUUID)

	modelmigration.RegisterImport(s.coordinator, defaultsProvider, loggertesting.WrapCheckLog(c))

	st := state.NewState(modelTxnFactory)
	s.svc = service.NewService(
		defaultsProvider,
		config.ModelValidator(),
		service.ProviderModelConfigGetter(),
		st,
	)

	// Initialise required model config entries.
	err := st.UpdateModelConfig(c.Context(), map[string]string{
		"name": "foo",
		"uuid": s.modelUUID.String(),
		"type": "dummy",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	c.Cleanup(func() {
		s.coordinator = nil
		s.svc = nil
		s.scope = coremodelmigration.Scope{}
	})
}

func (s *importSuite) TestImportModelConfig(c *tc.C) {
	cfg, err := config.New(config.NoDefaults, map[string]any{
		"name":          "foo",
		"uuid":          s.modelUUID.String(),
		"type":          "dummy",
		"nonsense":      42,
		"image-stream":  "beta",
		"default-base":  "ubuntu@20.04",
		"agent-version": "7.8.9",
		"agent-stream":  "candidate",
	})
	c.Assert(err, tc.ErrorIsNil)

	desc := description.NewModel(description.ModelArgs{
		Config: cfg.AllAttrs(),
	})

	err = s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	storedConfig, err := s.svc.ModelConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	name := storedConfig.Name()
	c.Assert(name, tc.Equals, "foo")

	uuid := storedConfig.UUID()
	c.Assert(uuid, tc.Equals, s.modelUUID.String())

	modelType := storedConfig.Type()
	c.Assert(modelType, tc.Equals, "dummy")

	imageStream := storedConfig.ImageStream()
	c.Assert(imageStream, tc.Equals, "beta")

	defaultBase, _ := storedConfig.DefaultBase()
	c.Check(defaultBase, tc.Equals, "ubuntu@20.04")

	agentVersion, _ := storedConfig.AgentVersion()
	c.Check(agentVersion.String(), tc.Equals, "7.8.9")

	agentStream := storedConfig.AgentStream()
	c.Check(agentStream, tc.Equals, "candidate")

	defaults := config.ConfigDefaults()

	lxdSnapChannel := storedConfig.LXDSnapChannel()
	c.Check(lxdSnapChannel, tc.DeepEquals, defaults[config.LXDSnapChannel])
}
