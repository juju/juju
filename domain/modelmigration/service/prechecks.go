// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
)

// PrecheckImport runs the environmental prechecks for a v8 model migration
// import against the target controller: the model's cloud and region must
// exist, every referenced user must be usable, the model credential must not
// be revoked, the secret backend must exist, and the model UUID/name must not
// collide with anything already on the controller. It performs no writes.
//
// These checks deliberately live in the modelmigration domain (reading the
// controller database directly) rather than in the migrationtarget facade, so
// the facade stays thin and the prechecks are a single domain concern.
func (s *Service) PrecheckImport(ctx context.Context, args modelmigration.ImportPrecheckArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.precheckCloudAndRegion(ctx, args); err != nil {
		return errors.Capture(err)
	}
	if err := s.precheckUsers(ctx, args.Users); err != nil {
		return errors.Capture(err)
	}
	if err := s.precheckCredential(ctx, args.Credential); err != nil {
		return errors.Capture(err)
	}
	if err := s.precheckSecretBackend(ctx, args.SecretBackend); err != nil {
		return errors.Capture(err)
	}
	if err := s.precheckModelCollisions(ctx, args); err != nil {
		return errors.Capture(err)
	}
	return nil
}

// precheckCloudAndRegion verifies the model's cloud exists on the target and,
// if set, that the cloud region is known to that cloud.
func (s *Service) precheckCloudAndRegion(ctx context.Context, args modelmigration.ImportPrecheckArgs) error {
	exists, err := s.controllerState.CloudExists(ctx, args.Cloud)
	if err != nil {
		return errors.Capture(err)
	}
	if !exists {
		return errors.Errorf("model's cloud %q not found on target controller", args.Cloud)
	}
	if args.CloudRegion == "" {
		return nil
	}
	regionExists, err := s.controllerState.CloudRegionExists(ctx, args.Cloud, args.CloudRegion)
	if err != nil {
		return errors.Capture(err)
	}
	if !regionExists {
		return errors.Errorf(
			"model's cloud region %q not valid for cloud %q on target controller",
			args.CloudRegion, args.Cloud)
	}
	return nil
}

// precheckUsers verifies that every model user can be applied on the target: a
// missing user is fine (it is recreated on import), but an existing user must
// not be disabled.
func (s *Service) precheckUsers(ctx context.Context, users []string) error {
	for _, u := range users {
		name, err := user.NewName(u)
		if err != nil {
			return errors.Errorf("model user name %q %w", u, coreerrors.NotValid)
		}
		disabled, exists, err := s.controllerState.IsUserDisabled(ctx, name.Name())
		if err != nil {
			return errors.Capture(err)
		}
		if exists && disabled {
			return errors.Errorf("model user %q is disabled on the target controller", u)
		}
	}
	return nil
}

// precheckCredential verifies that the model's cloud credential, when it
// already exists on the target, is not revoked there. Following 3.6 semantics,
// only the credential's existence (by natural key) and revoked status are
// checked; auth-type and attributes are not compared.
func (s *Service) precheckCredential(ctx context.Context, cred *modelmigration.ImportPrecheckCredential) error {
	if cred == nil {
		return nil
	}
	owner, err := user.NewName(cred.Owner)
	if err != nil {
		return errors.Errorf("model credential owner %q %w", cred.Owner, coreerrors.NotValid)
	}
	revoked, exists, err := s.controllerState.GetCredentialRevoked(ctx, cred.Cloud, owner.Name(), cred.Name)
	if err != nil {
		return errors.Capture(err)
	}
	if exists && revoked && !cred.Revoked {
		return errors.Errorf(
			"model credential %q/%q/%q is revoked on the target controller",
			cred.Cloud, cred.Owner, cred.Name)
	}
	return nil
}

// precheckSecretBackend verifies the model's secret backend exists on the
// target.
func (s *Service) precheckSecretBackend(ctx context.Context, backend string) error {
	if backend == "" {
		return nil
	}
	exists, err := s.controllerState.SecretBackendExists(ctx, backend)
	if err != nil {
		return errors.Capture(err)
	}
	if !exists {
		return errors.Errorf("model's secret backend %q not found on target controller", backend)
	}
	return nil
}

// precheckModelCollisions rejects imports that would collide with live rows on
// the target's shared-namespace tables (model, model_namespace) or with an
// existing import claim, and rejects model name/qualifier conflicts.
func (s *Service) precheckModelCollisions(ctx context.Context, args modelmigration.ImportPrecheckArgs) error {
	claim, err := s.controllerState.GetImportClaim(ctx, args.ModelUUID)
	if err == nil {
		return errors.Errorf(
			"model %q already has an import claim on this controller (phase %q, source migration %q, updated %s)",
			args.ModelUUID, claim.Phase, claim.SourceMigrationUUID,
			claim.UpdatedAt.Format("2006-01-02 15:04:05"))
	} else if !errors.Is(err, modelmigrationerrors.ErrImportNotFound) {
		return errors.Errorf("retrieving import claim for model %q: %w", args.ModelUUID, err)
	}

	// No import claim: any live row for this UUID is a hard collision.
	modelExists, err := s.controllerState.ModelExists(ctx, args.ModelUUID)
	if err != nil {
		return errors.Capture(err)
	}
	if modelExists {
		return errors.Errorf("model with same UUID already exists (%s)", args.ModelUUID)
	}

	nsExists, err := s.controllerState.ModelNamespaceExists(ctx, args.ModelUUID)
	if err != nil {
		return errors.Capture(err)
	}
	if nsExists {
		return errors.Errorf(
			"model database namespace for %q already exists on target controller", args.ModelUUID)
	}

	nameInUse, err := s.controllerState.ModelNameInUse(ctx, args.ModelName, args.ModelQualifier)
	if err != nil {
		return errors.Capture(err)
	}
	if nameInUse {
		return errors.Errorf("model named %q already exists", args.ModelName)
	}
	return nil
}
