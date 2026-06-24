// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v2

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/agentbinary"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	corelease "github.com/juju/juju/core/lease"
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
	"github.com/juju/juju/rpc/params"
)

// Deps bundles the database and ambient dependencies the v8 import
// orchestrator needs, supplied by the caller's migration scope.
type Deps struct {
	ControllerDB database.TxnRunnerFactory
	ModelDB      database.TxnRunnerFactory
	Clock        clock.Clock
	Logger       logger.Logger
}

// ImportModel applies the v8 import envelope's controller-scoped semantic
// data to the target controller: the durable model_migration_import claim,
// the target-local model bootstrap (controller model row + model DB in
// importing mode), and the users, credential, permissions, authorized keys,
// secret backend, leadership and cloud image metadata carried by envelope.
// Model-DB content import and activation are not part of this function.
//
// Each step calls the owning domain's service import method directly (the
// controller-DB facts have no transformer to run and no per-domain coordinator
// ownership to honour); this function constructs the controller-scoped domain
// services once and orchestrates the call order FK-/dependency-safely.
// AssertImporting (called once, at the end) and the two atomic companion-write
// assertions inside the claim service are the only importing-phase guards; the
// per-domain writes in between are not individually guarded (see
// [migrationclaimservice.Service.AssertImporting]).
//
// If a claim already exists for envelope.ModelInfo.UUID, the returned error
// wraps [coreerrors.AlreadyExists] (phase-specific wording is supplied by the
// modelmigration domain).
func ImportModel(
	ctx context.Context, deps Deps, envelope params.SerializedModelV2, view export.ProjectionView,
) error {
	modelUUIDStr := envelope.ModelInfo.UUID
	modelUUID := coremodel.UUID(modelUUIDStr)
	info := controllerModelInfoFromEnvelope(envelope)

	claimSvc := migrationclaimservice.NewImportService(migrationclaimstate.New(deps.ControllerDB, deps.Clock))
	accessSvc := accessservice.NewService(accessstate.NewState(deps.ControllerDB, deps.Clock, deps.Logger), deps.Clock)
	credentialSvc := credentialservice.NewService(credentialstate.NewState(deps.ControllerDB), deps.Logger)
	keymanagerSvc := keymanagerservice.NewService(modelUUID, keymanagerstate.NewState(deps.ControllerDB))
	secretBackendSvc := secretbackendservice.NewService(secretbackendstate.NewState(deps.ControllerDB, deps.Logger), deps.Logger)
	leaseSvc := leaseservice.NewService(leasestate.NewState(deps.ControllerDB, deps.Logger))
	cloudImageSvc := cloudimagemetadataservice.NewService(cloudimagemetadatastate.NewState(deps.ControllerDB, deps.Clock, deps.Logger))

	claimUUID, err := claimSvc.BeginImport(ctx, modelUUID, envelope.ModelInfo.SourceMigrationUUID)
	if err != nil {
		return errors.Capture(err)
	}

	inactiveUsers, err := accessSvc.ImportModelUsers(ctx, info.Users)
	if err != nil {
		return errors.Errorf("resolving users for model %q import: %w", modelUUIDStr, err)
	}

	var credKey corecredential.Key
	if info.ModelCredential != nil {
		credKey, err = credentialSvc.ImportModelCredential(ctx, *info.ModelCredential)
		if err != nil {
			return errors.Errorf("resolving credential for model %q import: %w", modelUUIDStr, err)
		}
	}

	var secretBackendName string
	if info.SecretBackend != nil {
		secretBackendName = info.SecretBackend.Name
	}
	agentStream := agentStreamFromModelConfig(view)
	if err := bootstrapImportedModel(
		ctx, deps, modelUUID, info.ModelInfo, credKey, secretBackendName,
		agentStream, view.AgentTargetVersion,
	); err != nil {
		return errors.Errorf("bootstrapping model %q: %w", modelUUIDStr, err)
	}

	if err := claimSvc.ImportExternalControllers(
		ctx, modelUUID, claimUUID, info.ExternalControllers,
	); err != nil {
		return errors.Errorf("importing external controllers for model %q import: %w", modelUUIDStr, err)
	}

	offerUUIDs, err := accessSvc.ImportModelPermissions(ctx, info.Permissions, inactiveUsers)
	if err != nil {
		return errors.Errorf("applying permissions for model %q import: %w", modelUUIDStr, err)
	}
	if err := claimSvc.ImportOfferPermissions(ctx, modelUUID, claimUUID, offerUUIDs); err != nil {
		return errors.Errorf("recording offer permissions for model %q import: %w", modelUUIDStr, err)
	}

	if err := keymanagerSvc.ImportAuthorizedKeys(
		ctx, info.AuthorizedKeys, inactiveUsers, accessSvc.GetUserUUIDByName,
	); err != nil {
		return errors.Errorf("applying authorized keys for model %q import: %w", modelUUIDStr, err)
	}

	if err := secretBackendSvc.ImportSecretBackendReferences(
		ctx, modelUUID, info.SecretBackendRefs,
	); err != nil {
		return errors.Errorf("applying secret backend references for model %q import: %w", modelUUIDStr, err)
	}

	if err := leaseSvc.ImportApplicationLeadership(ctx, modelUUID, info.Leaders); err != nil {
		return errors.Errorf("claiming leadership leases for model %q import: %w", modelUUIDStr, err)
	}

	if err := accessSvc.ImportLastModelLogins(ctx, modelUUID, info.Users, inactiveUsers); err != nil {
		return errors.Errorf("applying last logins for model %q import: %w", modelUUIDStr, err)
	}

	imageConflicts, err := cloudImageSvc.ImportCloudImageMetadata(ctx, info.CloudImageMetadata)
	if err != nil {
		return errors.Errorf("applying cloud image metadata for model %q import: %w", modelUUIDStr, err)
	}
	for _, c := range imageConflicts {
		// Non-fatal: the target's custom image metadata is kept (target-wins);
		// the operator can reconcile via the normal custom-metadata path.
		deps.Logger.Warningf(ctx,
			"model %q import: keeping target custom cloud image metadata for %s/%s/%s/%s image %q, skipping source image %q",
			modelUUIDStr, c.Stream, c.Region, c.Version, c.Arch, c.ExistingImageID, c.IncomingImageID)
	}

	if err := claimSvc.AssertImporting(ctx, modelUUID); err != nil {
		return errors.Errorf("model %q import interrupted: %w", modelUUIDStr, err)
	}

	return nil
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

// controllerModelInfoFromEnvelope decodes a v8 wire envelope's
// controller-scoped semantic fields into their target-portable domain form.
// It is the inverse of the source side's envelopeFromControllerModelInfo
// (internal/worker/migrationmaster/envelope.go), and is run once at the start
// of import so every step below works from the same typed snapshot.
func controllerModelInfoFromEnvelope(envelope params.SerializedModelV2) coremodelmigration.ControllerModelInfo {
	info := coremodelmigration.ControllerModelInfo{
		ModelInfo: coremodelmigration.ModelIdentityInfo{
			UUID:            envelope.ModelInfo.UUID,
			Name:            envelope.ModelInfo.Name,
			Qualifier:       envelope.ModelInfo.Qualifier,
			Type:            envelope.ModelInfo.Type,
			Cloud:           envelope.ModelInfo.Cloud,
			CloudRegion:     envelope.ModelInfo.CloudRegion,
			CredentialName:  envelope.ModelInfo.CredentialName,
			CredentialOwner: envelope.ModelInfo.CredentialOwner,
			Life:            envelope.ModelInfo.Life,
		},
	}
	if n := len(envelope.Users); n > 0 {
		info.Users = make([]coremodelmigration.ModelUser, 0, n)
	}
	for _, u := range envelope.Users {
		info.Users = append(info.Users, coremodelmigration.ModelUser{
			Name:        u.Name,
			DisplayName: u.DisplayName,
			CreatedBy:   u.CreatedBy,
			CreatedAt:   u.CreatedAt,
			Removed:     u.Removed,
			External:    u.External,
			LastLogin:   u.LastLogin,
		})
	}
	if cred := envelope.ModelCredential; cred != nil {
		info.ModelCredential = &coremodelmigration.ModelCloudCredential{
			Cloud:         cred.Cloud,
			Owner:         cred.Owner,
			Name:          cred.Name,
			AuthType:      cred.AuthType,
			Attributes:    cred.Attributes,
			Revoked:       cred.Revoked,
			Invalid:       cred.Invalid,
			InvalidReason: cred.InvalidReason,
		}
	}
	if n := len(envelope.Permissions); n > 0 {
		info.Permissions = make([]coremodelmigration.ModelPermission, 0, n)
	}
	for _, p := range envelope.Permissions {
		info.Permissions = append(info.Permissions, coremodelmigration.ModelPermission{
			ObjectType:  p.ObjectType,
			GrantOn:     p.GrantOn,
			SubjectName: p.SubjectName,
			Access:      p.Access,
		})
	}
	if n := len(envelope.AuthorizedKeys); n > 0 {
		info.AuthorizedKeys = make([]coremodelmigration.ModelAuthorizedKey, 0, n)
	}
	for _, k := range envelope.AuthorizedKeys {
		info.AuthorizedKeys = append(info.AuthorizedKeys, coremodelmigration.ModelAuthorizedKey{
			Username:  k.Username,
			PublicKey: k.PublicKey,
		})
	}
	if backend := envelope.SecretBackend; backend != nil {
		info.SecretBackend = &coremodelmigration.ModelSecretBackend{
			Name:        backend.Name,
			BackendType: backend.BackendType,
		}
	}
	if n := len(envelope.SecretBackendRefs); n > 0 {
		info.SecretBackendRefs = make([]coremodelmigration.SecretBackendReference, 0, n)
	}
	for _, ref := range envelope.SecretBackendRefs {
		info.SecretBackendRefs = append(info.SecretBackendRefs, coremodelmigration.SecretBackendReference{
			BackendName:        ref.BackendName,
			SecretRevisionUUID: ref.SecretRevisionUUID,
			SecretID:           ref.SecretID,
		})
	}
	leaderCount := 0
	for _, l := range envelope.Leases {
		if l.Type == corelease.ApplicationLeadershipNamespace {
			leaderCount++
		}
	}
	if leaderCount > 0 {
		info.Leaders = make([]coremodelmigration.ApplicationLeadership, 0, leaderCount)
	}
	for _, l := range envelope.Leases {
		if l.Type != corelease.ApplicationLeadershipNamespace {
			continue
		}
		info.Leaders = append(info.Leaders, coremodelmigration.ApplicationLeadership{
			Application: l.Name,
			Leader:      l.Holder,
		})
	}
	if n := len(envelope.CloudImageMetadata); n > 0 {
		info.CloudImageMetadata = make([]coremodelmigration.CloudImageMetadata, 0, n)
	}
	for _, m := range envelope.CloudImageMetadata {
		info.CloudImageMetadata = append(info.CloudImageMetadata, coremodelmigration.CloudImageMetadata{
			Stream:          m.Stream,
			Region:          m.Region,
			Version:         m.Version,
			Arch:            m.Arch,
			VirtType:        m.VirtType,
			RootStorageType: m.RootStorageType,
			RootStorageSize: m.RootStorageSize,
			Source:          m.Source,
			Priority:        m.Priority,
			ImageID:         m.ImageId,
			CreatedAt:       m.CreatedAt,
		})
	}
	if n := len(envelope.ExternalControllers); n > 0 {
		info.ExternalControllers = make([]coremodelmigration.ExternalController, 0, n)
	}
	for _, c := range envelope.ExternalControllers {
		info.ExternalControllers = append(info.ExternalControllers, coremodelmigration.ExternalController{
			UUID:           c.UUID,
			Alias:          c.Alias,
			CACert:         c.CACert,
			Addresses:      c.Addresses,
			ConsumedModels: c.ConsumedModels,
		})
	}
	return info
}
