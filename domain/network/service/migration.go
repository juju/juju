// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network/internal"
)

// MigrationState describes methods required
// for migrating machine network configuration.
type MigrationState interface {
	// AllMachinesAndNetNodes returns all machine names mapped to their
	// net mode UUIDs in the model.
	AllMachinesAndNetNodes(ctx context.Context) (map[string]string, error)

	// DeleteImportedLinkLayerDevices deletes all data added via the ImportLinkLayerDevices
	// method.
	DeleteImportedLinkLayerDevices(ctx context.Context) error

	// ImportLinkLayerDevices adds link layer devices into the model as part
	// of the migration import process.
	ImportLinkLayerDevices(ctx context.Context, input []internal.ImportLinkLayerDevice) error

	// GetAllSubnets returns all known subnets in the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
}

// MigrationService provides the API for model migration actions within
// the network domain.
type MigrationService struct {
	st     MigrationState
	logger logger.Logger
}

// NewMigrationService returns a new migration service reference wrapping
// the input state. These methods are specific to migration only and not
// intended to be used outside the domain.
func NewMigrationService(st MigrationState, logger logger.Logger) *MigrationService {
	return &MigrationService{
		st:     st,
		logger: logger,
	}
}
