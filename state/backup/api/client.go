// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/state/backup"
)

type backupClientAPI struct {
	client apiClient
}

func NewBackupClientAPI(client apiClient) (BackupAPI, error) {
	return newBackupClientAPI(client)
}

func newBackupClientAPI(client apiClient) (BackupAPI, error) {
	api := backupClientAPI{
		client: client,
	}
	return &api, nil
}

func (ba *backupClientAPI) Create(name string) (*backup.BackupInfo, string, error) {
	args := BackupArgs{
		Action: ActionCreate,
		Name:   name,
	}
	result, err := ba.client.Backup(args)
	if err != nil {
		return nil, "", err
	}
	return &result.Info, result.URL, nil
}
