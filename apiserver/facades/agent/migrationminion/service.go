// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"context"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
)

type ModelMigrationService interface {
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
