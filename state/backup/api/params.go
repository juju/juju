// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/state/backup"
)

// Backup holds the args to backup-related API calls.
type Backup struct {
	Action string
	Name   string
}

// BackupResult holds the result of backup-related API calls.
type BackupResult struct {
	Info backup.BackupInfo
	URL  string
}

// BackupListResult holds the result of a BackupList API call.
type BackupListResult struct {
	Backups map[string]backup.BackupInfo
}
