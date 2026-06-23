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
	accessv2 "github.com/juju/juju/domain/access/modelmigration/v2"
	cloudimagemetadatav2 "github.com/juju/juju/domain/cloudimagemetadata/modelmigration/v2"
	credentialv2 "github.com/juju/juju/domain/credential/modelmigration/v2"
	"github.com/juju/juju/domain/export"
	keymanagerv2 "github.com/juju/juju/domain/keymanager/modelmigration/v2"
	leasev2 "github.com/juju/juju/domain/lease/modelmigration/v2"
	modelv2 "github.com/juju/juju/domain/model/modelmigration/v2"
	migrationclaimservice "github.com/juju/juju/domain/modelmigration/service"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	secretbackendv2 "github.com/juju/juju/domain/secretbackend/modelmigration/v2"
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
// Each step delegates to the owning domain's modelmigration/v2 package; this
// function only decodes the envelope once and orchestrates the call order
// FK-/dependency-safely. AssertImporting (called once, at the end) and the
// two atomic companion-write assertions inside the claim service are the
// only importing-phase guards; the per-domain writes in between are not
// individually guarded (see [migrationclaimservice.Service.AssertImporting]).
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

	claimUUID, err := claimSvc.BeginImport(ctx, modelUUID, envelope.ModelInfo.SourceMigrationUUID)
	if err != nil {
		return errors.Capture(err)
	}

	inactiveUsers, err := accessv2.ImportModelUsers(ctx, deps.ControllerDB, deps.Clock, deps.Logger, info.Users)
	if err != nil {
		return errors.Errorf("resolving users for model %q import: %w", modelUUIDStr, err)
	}

	var credKey corecredential.Key
	if info.ModelCredential != nil {
		credKey, err = credentialv2.ImportModelCredential(ctx, deps.ControllerDB, deps.Logger, *info.ModelCredential)
		if err != nil {
			return errors.Errorf("resolving credential for model %q import: %w", modelUUIDStr, err)
		}
	}

	var secretBackendName string
	if info.SecretBackend != nil {
		secretBackendName = info.SecretBackend.Name
	}
	agentStream := agentStreamFromModelConfig(view)
	if err := modelv2.BootstrapImportedModel(
		ctx, deps.ControllerDB, deps.ModelDB, deps.Logger, modelUUID,
		info.ModelInfo, credKey, secretBackendName, agentStream, view.AgentTargetVersion,
	); err != nil {
		return errors.Errorf("bootstrapping model %q: %w", modelUUIDStr, err)
	}

	if err := claimSvc.ImportExternalControllers(
		ctx, modelUUID, claimUUID, info.ExternalControllers,
	); err != nil {
		return errors.Errorf("importing external controllers for model %q: %w", modelUUIDStr, err)
	}

	offerUUIDs, err := accessv2.ImportModelPermissions(ctx, deps.ControllerDB, deps.Clock, deps.Logger, info.Permissions, inactiveUsers)
	if err != nil {
		return errors.Errorf("applying permissions for model %q import: %w", modelUUIDStr, err)
	}
	if err := claimSvc.ImportOfferPermissions(ctx, modelUUID, claimUUID, offerUUIDs); err != nil {
		return errors.Errorf("recording offer permissions for model %q import: %w", modelUUIDStr, err)
	}

	if err := keymanagerv2.ImportAuthorizedKeys(
		ctx, deps.ControllerDB, deps.Clock, modelUUID, info.AuthorizedKeys, inactiveUsers,
	); err != nil {
		return errors.Errorf("applying authorized keys for model %q import: %w", modelUUIDStr, err)
	}

	if err := secretbackendv2.ImportSecretBackendReferences(
		ctx, deps.ControllerDB, deps.Logger, modelUUID, info.SecretBackendRefs,
	); err != nil {
		return errors.Errorf("applying secret backend references for model %q import: %w", modelUUIDStr, err)
	}

	if err := leasev2.ImportApplicationLeadership(
		ctx, deps.ControllerDB, deps.Logger, modelUUID, info.Leaders,
	); err != nil {
		return errors.Errorf("claiming leadership leases for model %q import: %w", modelUUIDStr, err)
	}

	if err := accessv2.ImportLastModelLogins(ctx, deps.ControllerDB, deps.Clock, deps.Logger, modelUUID, info.Users, inactiveUsers); err != nil {
		return errors.Errorf("applying last logins for model %q import: %w", modelUUIDStr, err)
	}

	if err := cloudimagemetadatav2.ImportCloudImageMetadata(
		ctx, deps.ControllerDB, deps.Clock, deps.Logger, info.CloudImageMetadata,
	); err != nil {
		return errors.Errorf("applying cloud image metadata for model %q import: %w", modelUUIDStr, err)
	}

	if err := claimSvc.AssertImporting(ctx, modelUUID); err != nil {
		return errors.Errorf("model %q import interrupted: %w", modelUUIDStr, err)
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
