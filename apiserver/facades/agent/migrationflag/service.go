// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag

import (
	"context"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/modelmigration"
)

type ModelMigrationService interface {
	WatchMigrationPhase(ctx context.Context) (watcher.NotifyWatcher, error)
	Migration(ctx context.Context) (modelmigration.Migration, error)
}
