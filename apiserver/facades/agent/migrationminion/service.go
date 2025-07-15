// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/modelmigration"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package migrationminion_test -destination mocks_test.go github.com/juju/juju/apiserver/facades/agent/migrationminion ModelMigrationService,ControllerNodeService,ControllerConfigService

// ModelMigrationService defines migration functionality required for the minion.
type ModelMigrationService interface {
	// Migration returns status about migration of this model.
	Migration(ctx context.Context) (modelmigration.Migration, error)
	// WatchForMigration returns a notification watcher that fires when this model
	// undergoes migration.
	WatchForMigration(ctx context.Context) (watcher.NotifyWatcher, error)
	// ReportFromUnit accepts a phase report from a migration minion for a unit
	// agent.
	ReportFromUnit(ctx context.Context, unitName unit.Name, phase migration.Phase) error
	// ReportFromMachine accepts a phase report from a migration minion for a
	// machine agent.
	ReportFromMachine(ctx context.Context, machineName machine.Name, phase migration.Phase) error
}

// ControllerNodeService defines API address functionality required by the
// migration watchers.
type ControllerNodeService interface {
	// GetAllAPIAddressesForClients returns a string slice of api
	// addresses available for agents.
	GetAllAPIAddressesForClients(ctx context.Context) ([]string, error)
}

// ControllerConfigService defines the methods required to get the controller
// configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (controller.Config, error)
}
