// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import "github.com/juju/juju/core/migration"

// MigrationStatus is the client side version of
// params.MigrationStatus.
type MigrationStatus struct {
	Attempt        int
	Phase          migration.Phase
	SourceAPIAddrs []string
	SourceCACert   string
	TargetAPIAddrs []string
	TargetCACert   string
}

// MigrationStatusWatcher describes a watcher that reports the latest
// status of a migration for a model.
type MigrationStatusWatcher interface {
	CoreWatcher
	Changes() <-chan MigrationStatus
}
