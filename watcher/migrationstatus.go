// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import "github.com/juju/juju/apiserver/params"

// MigrationStatusWatcher describes a watcher that reports the latest
// status of a migration for a model.
type MigrationStatusWatcher interface {
	CoreWatcher
	Changes() <-chan params.MigrationStatus
}
