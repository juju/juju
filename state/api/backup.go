// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/state/backup"
	"github.com/juju/juju/state/backup/api"
)

var (
	newBackupAPI = api.NewBackupClientAPI
)

func (c *Client) Backup(args api.BackupArgs) (api.BackupResult, error) {
	var result api.BackupResult
	err := c.call("Backup", args, &result)
	return result, err
}

func (c *Client) BackupCreate(name string) (*backup.BackupInfo, string, error) {
	backupAPI := newBackupAPI(c)
	return backupAPI.Create(name)
}
