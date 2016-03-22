// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import "github.com/juju/juju/core/migration"

// MigrationMasterWatcher describes a watcher that reports the target
// controller details for an active model migration.
type MigrationMasterWatcher interface {
	CoreWatcher
	Changes() <-chan migration.TargetInfo
}
