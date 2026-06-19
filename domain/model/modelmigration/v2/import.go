// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v2

import (
	"context"

	"github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/semversion"
	domainmodel "github.com/juju/juju/domain/model"
	modelservice "github.com/juju/juju/domain/model/service"
	modelmigrationservice "github.com/juju/juju/domain/model/service/migration"
	statecontroller "github.com/juju/juju/domain/model/state/controller"
	statemodel "github.com/juju/juju/domain/model/state/model"
	"github.com/juju/juju/internal/errors"
)

// BootstrapImportedModel creates the controller-database model row
// (claim-free: the v8 import claim is owned by the modelmigration domain,
// not this call) and then establishes the model database's read-only model
// info, marking it as importing so charm uploads during the migration are
// handled correctly.
func BootstrapImportedModel(
	ctx context.Context,
	controllerDB, modelDB database.TxnRunnerFactory,
	logger logger.Logger,
	modelUUID coremodel.UUID,
	identity coremodelmigration.ModelIdentityInfo,
	credKey credential.Key,
	secretBackendName string,
	agentStream agentbinary.AgentStream,
	agentTargetVersion semversion.Number,
) error {
	args := domainmodel.ModelImportArgs{
		UUID: modelUUID,
		GlobalModelCreationArgs: domainmodel.GlobalModelCreationArgs{
			Cloud:         identity.Cloud,
			CloudRegion:   identity.CloudRegion,
			Credential:    credKey,
			Name:          identity.Name,
			Qualifier:     coremodel.Qualifier(identity.Qualifier),
			SecretBackend: secretBackendName,
		},
	}

	bootstrapSvc := modelmigrationservice.NewMigrationService(statecontroller.NewState(controllerDB), logger)
	if err := bootstrapSvc.ImportModelV2(ctx, args); err != nil {
		return errors.Errorf("creating model %q: %w", identity.Name, err)
	}

	modelSvc := modelservice.NewModelService(
		modelUUID,
		statecontroller.NewState(controllerDB),
		statemodel.NewState(modelDB, logger),
		modelservice.EnvironVersionProviderGetter(),
		modelservice.DefaultAgentBinaryFinder(),
	)
	if err := modelSvc.CreateImportingModelWithAgentVersionStream(ctx, agentTargetVersion, agentStream); err != nil {
		return errors.Errorf("creating model %q database: %w", identity.Name, err)
	}
	return nil
}
