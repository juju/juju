// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/passwords"
)

// MigrationState is the state required for migrating passwords.
type MigrationState interface {
	// GetAllUnitPasswordHashes returns a map of unit names to password hashes.
	GetAllUnitPasswordHashes(context.Context) (map[string]map[unit.Name]passwords.PasswordHash, error)
}

// MigrationService provides the API for migrating passwords.
type MigrationService struct {
	st MigrationState
}

// NewMigrationService returns a new service reference wrapping the input state.
func NewMigrationService(
	st MigrationState,
) *MigrationService {
	return &MigrationService{
		st: st,
	}
}

// GetAllUnitPasswordHashes returns a map of unit names to password hashes.
func (s *MigrationService) GetAllUnitPasswordHashes(ctx context.Context) (map[string]map[unit.Name]passwords.PasswordHash, error) {
	return s.st.GetAllUnitPasswordHashes(ctx)
}
