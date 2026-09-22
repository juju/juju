// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/version"
	accessstate "github.com/juju/juju/domain/access/state"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	domainmodel "github.com/juju/juju/domain/model"
	modelcontrollerstate "github.com/juju/juju/domain/model/state/controller"
	modelstate "github.com/juju/juju/domain/model/state/model"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/domain/modelmigration"
	schematesting "github.com/juju/juju/domain/schema/testing"
	secretbootstrap "github.com/juju/juju/domain/secretbackend/bootstrap"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	_ "github.com/juju/juju/internal/provider/dummy"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type legacyImportSuite struct {
	schematesting.ControllerModelSuite
}

func TestLegacyImportSuite(t *testing.T) {
	tc.Run(t, &legacyImportSuite{})
}

// TestEmptyModel establishes that the production coordinator can finish with
// this fixture. Regression cases below must fail on their additional exported
// state, rather than missing controller prerequisites or provider configuration.
func (s *legacyImportSuite) TestEmptyModel(c *tc.C) {
	desc, coordinator, scope := s.setupImport(c)
	c.Assert(coordinator.Perform(c.Context(), scope, desc), tc.ErrorIsNil)
}

// TestModelSpaceConstraints checks that a constraint can resolve a named space
// from the same 3.6 description. It runs the production operation registration
// order against real databases, so a mock cannot accept the constraint before
// the space actually exists.
func (s *legacyImportSuite) TestModelSpaceConstraints(c *tc.C) {
	desc, coordinator, scope := s.setupImport(c)
	desc.AddSpace(description.SpaceArgs{Id: "1", Name: "database", ProviderID: "space-1"})
	desc.SetConstraints(description.ConstraintsArgs{Spaces: []string{"database"}})
	c.Assert(coordinator.Perform(c.Context(), scope, desc), tc.ErrorIsNil)
	cons, err := modelstate.NewState(scope.ModelDB(), loggertesting.WrapCheckLog(c)).GetModelConstraints(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons.Spaces, tc.NotNil)
	c.Assert(*cons.Spaces, tc.HasLen, 1)
	c.Check((*cons.Spaces)[0].SpaceName, tc.Equals, "database")
}

// setupImport creates the target controller prerequisites and an empty source
// description. All import operations are registered through the production
// entry point; none are mocked, skipped, or reordered by this fixture.
func (s *legacyImportSuite) setupImport(c *tc.C) (description.Model, *coremodelmigration.Coordinator, coremodelmigration.Scope) {
	logger := loggertesting.WrapCheckLog(c)
	controllerFactory := s.TxnRunnerFactory()
	access := accessstate.NewState(controllerFactory, clock.WallClock, logger)
	owner, err := user.NewName("admin")
	c.Assert(err, tc.ErrorIsNil)
	ownerUUID := tc.Must(c, user.NewUUID)
	c.Assert(access.AddUser(c.Context(), ownerUUID, owner, "Admin", false, ownerUUID), tc.ErrorIsNil)
	everyone, err := user.NewName("everyone@external")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access.AddUser(c.Context(), tc.Must(c, user.NewUUID), everyone, "Everyone", true, ownerUUID), tc.ErrorIsNil)
	// The dummy provider is deliberately absent from the production enum.
	c.Assert(s.ControllerTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO cloud_type (id, type) VALUES (999, 'dummy')")
		return err
	}), tc.ErrorIsNil)
	cloud := cloud.Cloud{Name: "dummy", Type: "dummy", AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType}, Regions: []cloud.Region{{Name: "dummy-region"}}}
	c.Assert(cloudstate.NewState(controllerFactory).CreateCloud(c.Context(), owner, tc.Must(c, uuid.NewUUID).String(), cloud), tc.ErrorIsNil)
	c.Assert(secretbootstrap.CreateDefaultBackends(model.IAAS)(c.Context(), s.ControllerTxnRunner(), nil), tc.ErrorIsNil)
	controllerModelUUID := tc.Must(c, model.NewUUID)
	controllerState := modelcontrollerstate.NewState(controllerFactory)
	err = s.ControllerTxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		if err := modelcontrollerstate.Create(ctx, controllerState, tx, controllerModelUUID, model.IAAS, domainmodel.GlobalModelCreationArgs{
			Name: "controller", Qualifier: "admin", Cloud: "dummy", CloudRegion: "dummy-region", AdminUsers: []user.UUID{ownerUUID}, SecretBackend: "internal",
		}); err != nil {
			return err
		}
		return modelcontrollerstate.GetActivator()(ctx, controllerState, tx, controllerModelUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	s.SeedControllerTable(c, controllerModelUUID)

	modelUUID := tc.Must(c, model.NewUUID)
	runner := s.ModelTxnRunner(c, modelUUID.String())
	modelFactory := func(context.Context) (database.TxnRunner, error) { return runner, nil }
	attrs := jujutesting.FakeConfig().Merge(jujutesting.Attrs{
		"uuid": modelUUID.String(), "agent-version": version.Current.String(),
	})
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)
	// Agent version is already selected by migration before domain import.
	desc := description.NewModel(description.ModelArgs{
		Type: model.IAAS.String(), Owner: "admin", Cloud: "dummy", CloudRegion: "dummy-region", Config: cfg.AllAttrs(),
	})
	provider := legacyImportProvider{cfg: cfg}
	scope := coremodelmigration.NewScope(controllerFactory, modelFactory, nil, provider, modelUUID)
	coordinator := coremodelmigration.NewCoordinator(logger)
	modelmigration.ImportOperations(coordinator, legacyImportDefaults{}, provider, clock.WallClock, logger)
	return desc, coordinator, scope
}

type legacyImportDefaults struct{}

func (legacyImportDefaults) ModelDefaults(context.Context) (modeldefaults.Defaults, error) {
	return modeldefaults.Defaults{}, nil
}

// legacyImportProvider supplies the existing dummy provider for storage pool
// defaults. No live provider or controller is contacted by these tests.
type legacyImportProvider struct{ cfg *config.Config }

func (p legacyImportProvider) GetEphemeralProviderConfig(context.Context) (providertracker.EphemeralProviderConfig, error) {
	return providertracker.EphemeralProviderConfig{ModelType: model.IAAS, ModelConfig: p.cfg, CloudSpec: jujutesting.FakeCloudSpec()}, nil
}

func (legacyImportProvider) EphemeralProviderFromConfig(ctx context.Context, cfg providertracker.EphemeralProviderConfig) (providertracker.Provider, error) {
	provider, err := environs.Provider("dummy")
	if err != nil {
		return nil, err
	}
	return environs.Open(ctx, provider, environs.OpenParams{Config: cfg.ModelConfig, Cloud: cfg.CloudSpec}, nil)
}
