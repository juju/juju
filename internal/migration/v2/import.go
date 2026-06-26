// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v2

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/agentbinary"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/semversion"
	accessservice "github.com/juju/juju/domain/access/service"
	accessstate "github.com/juju/juju/domain/access/state"
	cloudimagemetadataservice "github.com/juju/juju/domain/cloudimagemetadata/service"
	cloudimagemetadatastate "github.com/juju/juju/domain/cloudimagemetadata/state"
	credentialservice "github.com/juju/juju/domain/credential/service"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/export"
	"github.com/juju/juju/domain/export/types/latest"
	keymanagerservice "github.com/juju/juju/domain/keymanager/service"
	keymanagerstate "github.com/juju/juju/domain/keymanager/state"
	leaseservice "github.com/juju/juju/domain/lease/service"
	leasestate "github.com/juju/juju/domain/lease/state"
	domainmodel "github.com/juju/juju/domain/model"
	modelservice "github.com/juju/juju/domain/model/service"
	modelmigrationservice "github.com/juju/juju/domain/model/service/migration"
	modelstatecontroller "github.com/juju/juju/domain/model/state/controller"
	modelstatemodel "github.com/juju/juju/domain/model/state/model"
	migrationclaimservice "github.com/juju/juju/domain/modelmigration/service"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	"github.com/juju/juju/internal/errors"
)

// Deps bundles the database and ambient dependencies the v8 import
// orchestrator needs, supplied by the caller's migration scope.
type Deps struct {
	ControllerDB database.TxnRunnerFactory
	ModelDB      database.TxnRunnerFactory
	Clock        clock.Clock
	Logger       logger.Logger
}

// ImportModelArgs contains the data needed to perform a v8 model import: the
// target-portable controller-scoped snapshot and the transformed model-DB
// payload.
type ImportModelArgs struct {
	// SourceMigrationUUID is the source-side migration UUID recorded on the
	// target import claim.
	SourceMigrationUUID string

	// ControllerModelInfo is the semantic controller-database snapshot for the
	// model, decoded from the v8 import envelope by the apiserver facade.
	ControllerModelInfo coremodelmigration.ControllerModelInfo

	// ModelDBPayload is the model-DB export payload decoded from the envelope
	// and transformed up to the target schema version by the apiserver facade.
	// The controller-scoped import in this package does not use it; the model-DB
	// import operations driven by the migration package's coordinator consume
	// it. It is nil only for controller-scoped-only callers and tests.
	ModelDBPayload *latest.ModelExport
}

// ImportModel applies the v8 import's controller-scoped semantic data to the
// target controller: the durable model_migration_import claim, the
// target-local model bootstrap (controller model row + model DB in importing
// mode), and the users, credential, permissions, authorized keys, secret
// backend, leadership and cloud image metadata carried by info. Model-DB
// content import and activation are not part of this function.
//
// Each step calls the owning domain's service import method directly. The
// coordinator constructs the controller-scoped domain services once and
// orchestrates the call order FK-/dependency-safely.
//
// If a claim already exists for info.ModelInfo.UUID, the returned error
// wraps [coreerrors.AlreadyExists] (phase-specific wording is supplied by the
// modelmigration domain).
func ImportModel(
	ctx context.Context,
	deps Deps,
	args ImportModelArgs,
	view export.ProjectionView,
) error {
	return newImportCoordinator(deps, args, view).Import(ctx)
}

// importCoordinator wires the services and semantic input used by the v8
// import. Construction is separate from execution so the import flow below is
// only concerned with ordering and error handling.
type importCoordinator struct {
	deps                Deps
	services            importServices
	modelUUID           coremodel.UUID
	modelUUIDStr        string
	sourceMigrationUUID string
	info                coremodelmigration.ControllerModelInfo
	view                export.ProjectionView
}

func newImportCoordinator(
	deps Deps,
	args ImportModelArgs,
	view export.ProjectionView,
) importCoordinator {
	info := args.ControllerModelInfo
	modelUUIDStr := info.ModelInfo.UUID
	modelUUID := coremodel.UUID(modelUUIDStr)
	return importCoordinator{
		deps:                deps,
		services:            newImportServices(deps, modelUUID),
		modelUUID:           modelUUID,
		modelUUIDStr:        modelUUIDStr,
		sourceMigrationUUID: args.SourceMigrationUUID,
		info:                info,
		view:                view,
	}
}

func (c importCoordinator) Import(ctx context.Context) error {
	claimUUID, err := c.beginImport(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	inactiveUsers, err := c.importUsers(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	credKey, err := c.importCredential(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if err := c.bootstrapModel(ctx, credKey); err != nil {
		return errors.Capture(err)
	}

	if err := c.importExternalControllers(ctx, claimUUID); err != nil {
		return errors.Capture(err)
	}

	if err := c.importPermissions(ctx, claimUUID, inactiveUsers); err != nil {
		return errors.Capture(err)
	}

	if err := c.importAuthorizedKeys(ctx, inactiveUsers); err != nil {
		return errors.Capture(err)
	}

	if err := c.importSecretBackendReferences(ctx); err != nil {
		return errors.Capture(err)
	}

	if err := c.importLeadership(ctx); err != nil {
		return errors.Capture(err)
	}

	if err := c.importLastLogins(ctx, inactiveUsers); err != nil {
		return errors.Capture(err)
	}

	if err := c.importCloudImageMetadata(ctx); err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (c importCoordinator) beginImport(ctx context.Context) (string, error) {
	return c.services.claim.BeginImport(ctx, c.modelUUID, c.sourceMigrationUUID)
}

func (c importCoordinator) importUsers(ctx context.Context) (set.Strings, error) {
	inactiveUsers, err := c.services.access.ImportModelUsers(ctx, c.info.Users)
	if err != nil {
		return nil, errors.Errorf("resolving users for model %q import: %w", c.modelUUIDStr, err)
	}
	return inactiveUsers, nil
}

func (c importCoordinator) importCredential(ctx context.Context) (corecredential.Key, error) {
	if c.info.ModelCredential == nil {
		return corecredential.Key{}, nil
	}
	credKey, err := c.services.credential.ImportModelCredential(ctx, *c.info.ModelCredential)
	if err != nil {
		return corecredential.Key{}, errors.Errorf(
			"resolving credential for model %q import: %w", c.modelUUIDStr, err)
	}
	return credKey, nil
}

func (c importCoordinator) bootstrapModel(
	ctx context.Context, credKey corecredential.Key,
) error {
	var secretBackendName string
	if c.info.SecretBackend != nil {
		secretBackendName = c.info.SecretBackend.Name
	}
	agentStream := agentStreamFromModelConfig(c.view)
	if err := bootstrapImportedModel(
		ctx, c.deps, c.modelUUID, c.info.ModelInfo, credKey, secretBackendName,
		agentStream, c.view.AgentTargetVersion,
	); err != nil {
		return errors.Errorf("bootstrapping model %q: %w", c.modelUUIDStr, err)
	}
	return nil
}

func (c importCoordinator) importExternalControllers(
	ctx context.Context, claimUUID string,
) error {
	if err := c.services.claim.ImportExternalControllers(
		ctx, c.modelUUID, claimUUID, c.info.ExternalControllers,
	); err != nil {
		return errors.Errorf(
			"importing external controllers for model %q import: %w", c.modelUUIDStr, err)
	}
	return nil
}

func (c importCoordinator) importPermissions(
	ctx context.Context, claimUUID string, inactiveUsers set.Strings,
) error {
	offerUUIDs, err := c.services.access.ImportModelPermissions(
		ctx, c.info.Permissions, inactiveUsers,
	)
	if err != nil {
		return errors.Errorf("applying permissions for model %q import: %w", c.modelUUIDStr, err)
	}
	if err := c.services.claim.ImportOfferPermissions(
		ctx, c.modelUUID, claimUUID, offerUUIDs,
	); err != nil {
		return errors.Errorf(
			"recording offer permissions for model %q import: %w", c.modelUUIDStr, err)
	}
	return nil
}

func (c importCoordinator) importAuthorizedKeys(
	ctx context.Context, inactiveUsers set.Strings,
) error {
	if err := c.services.keymanager.ImportAuthorizedKeys(
		ctx, c.info.AuthorizedKeys, inactiveUsers, c.services.access.GetUserUUIDByName,
	); err != nil {
		return errors.Errorf(
			"applying authorized keys for model %q import: %w", c.modelUUIDStr, err)
	}
	return nil
}

func (c importCoordinator) importSecretBackendReferences(ctx context.Context) error {
	if err := c.services.secretBackend.ImportSecretBackendReferences(
		ctx, c.modelUUID, c.info.SecretBackendRefs,
	); err != nil {
		return errors.Errorf(
			"applying secret backend references for model %q import: %w", c.modelUUIDStr, err)
	}
	return nil
}

func (c importCoordinator) importLeadership(ctx context.Context) error {
	if err := c.services.lease.ImportApplicationLeadership(
		ctx, c.modelUUID, c.info.Leaders,
	); err != nil {
		return errors.Errorf(
			"claiming leadership leases for model %q import: %w", c.modelUUIDStr, err)
	}
	return nil
}

func (c importCoordinator) importLastLogins(
	ctx context.Context, inactiveUsers set.Strings,
) error {
	if err := c.services.access.ImportLastModelLogins(
		ctx, c.modelUUID, c.info.Users, inactiveUsers,
	); err != nil {
		return errors.Errorf(
			"applying last logins for model %q import: %w", c.modelUUIDStr, err)
	}
	return nil
}

func (c importCoordinator) importCloudImageMetadata(ctx context.Context) error {
	imageConflicts, err := c.services.cloudImage.ImportCloudImageMetadata(
		ctx, c.info.CloudImageMetadata,
	)
	if err != nil {
		return errors.Errorf(
			"applying cloud image metadata for model %q import: %w", c.modelUUIDStr, err)
	}
	for _, conflict := range imageConflicts {
		// Non-fatal: the target's custom image metadata is kept (target-wins);
		// the operator can reconcile via the normal custom-metadata path.
		c.deps.Logger.Warningf(ctx,
			"model %q import: keeping target custom cloud image metadata for %s/%s/%s/%s image %q, skipping source image %q",
			c.modelUUIDStr, conflict.Stream, conflict.Region, conflict.Version,
			conflict.Arch, conflict.ExistingImageID, conflict.IncomingImageID)
	}
	return nil
}

// importServices bundles the controller-scoped domain services the v8 import
// driver orchestrates. They are constructed once at the start of ImportModel
// and shared across the import steps.
type importServices struct {
	claim         *migrationclaimservice.Service
	access        *accessservice.Service
	credential    *credentialservice.Service
	keymanager    *keymanagerservice.Service
	secretBackend *secretbackendservice.Service
	lease         *leaseservice.Service
	cloudImage    *cloudimagemetadataservice.Service
}

// newImportServices constructs the controller-scoped domain services the v8
// import driver needs. Each service owns its state and is independent of the
// others; the import driver is responsible for calling them in FK-safe order.
func newImportServices(deps Deps, modelUUID coremodel.UUID) importServices {
	return importServices{
		claim: migrationclaimservice.NewImportService(
			migrationclaimstate.New(deps.ControllerDB, deps.Clock), deps.Logger,
		),
		access: accessservice.NewService(
			accessstate.NewState(deps.ControllerDB, deps.Clock, deps.Logger), deps.Clock,
		),
		credential: credentialservice.NewService(
			credentialstate.NewState(deps.ControllerDB), deps.Logger,
		),
		keymanager: keymanagerservice.NewService(
			modelUUID, keymanagerstate.NewState(deps.ControllerDB),
		),
		secretBackend: secretbackendservice.NewService(
			secretbackendstate.NewState(deps.ControllerDB, deps.Logger), deps.Logger,
		),
		lease: leaseservice.NewService(
			leasestate.NewState(deps.ControllerDB, deps.Logger),
		),
		cloudImage: cloudimagemetadataservice.NewService(
			cloudimagemetadatastate.NewState(deps.ControllerDB, deps.Clock, deps.Logger),
		),
	}
}

// bootstrapImportedModel creates the controller-database model row (claim-free:
// the v8 import claim is owned by the modelmigration domain, not this call) and
// then establishes the model database's read-only model info, marking it as
// importing so charm uploads during the migration are handled correctly. It is
// pure orchestration of two existing model-domain service methods.
func bootstrapImportedModel(
	ctx context.Context,
	deps Deps,
	modelUUID coremodel.UUID,
	identity coremodelmigration.ModelIdentityInfo,
	credKey corecredential.Key,
	secretBackendName string,
	agentStream agentbinary.AgentStream,
	agentTargetVersion semversion.Number,
) error {
	migrationSvc := modelmigrationservice.NewMigrationService(
		modelstatecontroller.NewState(deps.ControllerDB), deps.Logger,
	)
	modelSvc := modelservice.NewModelService(
		modelUUID,
		modelstatecontroller.NewState(deps.ControllerDB),
		modelstatemodel.NewState(deps.ModelDB, deps.Logger),
		modelservice.EnvironVersionProviderGetter(),
		modelservice.DefaultAgentBinaryFinder(),
	)

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

	if err := migrationSvc.ImportModelV2(ctx, args); err != nil {
		return errors.Errorf("creating model %q: %w", identity.Name, err)
	}
	if err := modelSvc.CreateImportingModelWithAgentVersionStream(ctx, agentTargetVersion, agentStream); err != nil {
		return errors.Errorf("creating model %q database: %w", identity.Name, err)
	}
	return nil
}

// agentStreamFromModelConfig reads the model's configured agent stream out of
// the projection view, defaulting to the released stream when unset.
func agentStreamFromModelConfig(view export.ProjectionView) agentbinary.AgentStream {
	if view.AgentStream != "" {
		return agentbinary.AgentStream(view.AgentStream)
	}
	return agentbinary.AgentStreamReleased
}
