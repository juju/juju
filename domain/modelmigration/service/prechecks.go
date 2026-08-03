// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/modelmigration"
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
	// Non-fatal: a custom cloud image metadata conflict is warned about, never
	// rejected (target-wins on import).
	s.precheckCloudImageMetadata(ctx, args.CloudImageMetadata)
	return nil
}

// precheckCloudImageMetadata emits a non-fatal warning for each custom cloud
// image metadata row whose natural key already exists on the target with a
// different image id. The existing target row is kept (target-wins) at import
// time, so this check never fails the migration.
func (s *Service) precheckCloudImageMetadata(ctx context.Context, rows []modelmigration.ImportPrecheckImageMetadata) {
	if len(rows) == 0 {
		return
	}
	conflicts, err := s.controllerState.GetConflictingCloudImageMetadata(ctx, rows)
	if err != nil {
		s.logger.Warningf(ctx, "checking custom cloud image metadata conflicts: %v", err)
		return
	}
	for _, c := range conflicts {
		s.logger.Warningf(ctx,
			"custom cloud image metadata conflict for %s/%s/%s/%s: keeping target image %q, source image %q will be skipped on import",
			c.Stream, c.Region, c.Version, c.Arch, c.ExistingImageID, c.ImageID)
	}
}

// precheckCloudAndRegion verifies the model's cloud exists on the target and,
// if set, that the cloud region is known to that cloud.
func (s *Service) precheckCloudAndRegion(ctx context.Context, args modelmigration.ImportPrecheckArgs) error {
	cloudExists, regionExists, err := s.controllerState.CheckCloudRegion(ctx, args.Cloud, args.CloudRegion)
	if err != nil {
		return errors.Capture(err)
	}
	if !cloudExists {
		return errors.Errorf("model's cloud %q not found on target controller", args.Cloud)
	}
	if args.CloudRegion != "" && !regionExists {
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
	names := make([]string, 0, len(users))
	for _, u := range users {
		name, err := user.NewName(u)
		if err != nil {
			return errors.Errorf("model user name %q %w", u, coreerrors.NotValid)
		}
		names = append(names, name.Name())
	}
	disabledUsers, err := s.controllerState.GetDisabledUsers(ctx, names)
	if err != nil {
		return errors.Capture(err)
	}
	if len(disabledUsers) >= 1 {
		return errors.Errorf("model users %q are disabled on the target controller",
			strings.Join(disabledUsers, ", "))
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
// existing import, and rejects model name/qualifier conflicts.
func (s *Service) precheckModelCollisions(ctx context.Context, args modelmigration.ImportPrecheckArgs) error {
	collision, err := s.controllerState.CheckImportModelCollision(
		ctx, args.ModelUUID, args.ModelName, args.ModelQualifier,
	)
	if err != nil {
		return errors.Capture(err)
	}
	if collision.Importing {
		return errors.Errorf("model %q already exists on this controller (currently importing)",
			args.ModelUUID)
	}
	if collision.ModelExists || collision.ModelNamespaceExists {
		return errors.Errorf("model %q already exists on this controller", args.ModelUUID)
	}
	if collision.ModelNameExists {
		return errors.Errorf("model named %q already exists", args.ModelName)
	}
	return nil
}
