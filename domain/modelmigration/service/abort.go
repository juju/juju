// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/errors"
)

// These methods are the controller-scoped pass-throughs the v8 abort driver
// (internal/migration.AbortModelImport) and the abort reconciler call to tear
// down a partially imported model. They mirror the activation-driver methods in
// activate.go: input validation and tracing live here, the SQL lives in state.

// SetImportPhaseAborting transitions the model's import claim from the
// importing phase to the aborting phase. It is idempotent when the claim is
// already aborting, and returns
// [modelmigrationerrors.ErrAbortActivating] when the claim has crossed the
// activation point of no return.
func (s *Service) SetImportPhaseAborting(ctx context.Context, modelUUID coremodel.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("validating model uuid: %w", err)
	}

	return s.controllerState.SetImportPhaseAborting(ctx, modelUUID.String())
}

// FinalizeAbortedImport deletes the model's import claim, its FK-dependent
// companion rows, and its namespace registration once abort cleanup is provably
// complete. It returns [modelmigrationerrors.ErrAbortNotFinalizable] when
// cleanup is not yet provable, and is idempotent when no claim exists.
func (s *Service) FinalizeAbortedImport(ctx context.Context, modelUUID coremodel.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("validating model uuid: %w", err)
	}

	return s.controllerState.FinalizeAbortedImport(ctx, modelUUID.String())
}

// StageAbortedModelDatabaseDeletion hands the aborted model's dqlite database
// off to the undertaker's model-database deleter (removing its namespace
// registration and staging the deletion), so the database is dropped out of
// band before the claim is finalized. It is idempotent.
func (s *Service) StageAbortedModelDatabaseDeletion(ctx context.Context, modelUUID coremodel.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("validating model uuid: %w", err)
	}

	return s.controllerState.StageAbortedModelDatabaseDeletion(ctx, modelUUID.String())
}

// IsModelDying reports whether the model row exists and has left the alive
// state (it is dying or dead), meaning the generic removal undertaker is
// already tearing it down, as happens after a v7/legacy abort marked it dead.
// The v8 abort driver uses this to stand aside from such a model rather than
// re-driving its own compensation over the undertaker's teardown.
func (s *Service) IsModelDying(ctx context.Context, modelUUID coremodel.UUID) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return false, errors.Errorf("validating model uuid: %w", err)
	}

	return s.controllerState.IsModelDying(ctx, modelUUID.String())
}

// GetAllImportClaims returns a snapshot of every outstanding import claim, for
// the abort reconciler to scan.
func (s *Service) GetAllImportClaims(ctx context.Context) ([]modelmigration.ImportClaimStatus, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.controllerState.GetAllImportClaims(ctx)
}

// IsImportNamespaceRegistered reports whether the model's dqlite namespace is
// still registered, i.e. whether the model database may still need dropping
// before abort finalization.
func (s *Service) IsImportNamespaceRegistered(ctx context.Context, modelUUID coremodel.UUID) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return false, errors.Errorf("validating model uuid: %w", err)
	}

	return s.controllerState.IsImportNamespaceRegistered(ctx, modelUUID.String())
}
