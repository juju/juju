// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/state/backup"
)

// Backup holds the args to backup-related API calls.
//
// Action must always be set.  Otherwise which fields must be set
// depends on the action.
type BackupArgs struct {
	Action string
	Name   string
}

// BackupResult holds the result of backup-related API calls.
//
// Not all fields will be set for any given result.
type BackupResult struct {
	Info backup.BackupInfo
	URL  string
}
