// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag

import (
	"context"

	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher"
)

// ModelMigrationService supplies the migration phase this facade reports to
// agents.
//
// Both methods are direction-agnostic: they report a model being imported into
// this controller as migrating, exactly as they report one being exported from
// it. That is what keeps a migrating agent's own workers parked between the
// moment it starts talking to this controller and the moment the import is
// committed and the claim released.
type ModelMigrationService interface {
	// WatchMigrationActivity fires on export phase transitions and on import
	// claim changes, including the claim deletion that makes an imported model
	// usable.
	WatchMigrationActivity(ctx context.Context) (watcher.NotifyWatcher, error)

	// MigrationPhase reports the model's migration phase in either direction,
	// or migration.NONE when the model is not migrating.
	MigrationPhase(ctx context.Context) (migration.Phase, error)
}
