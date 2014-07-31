// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"

	"github.com/juju/juju/state/backup"
)

var (
	createBackup = backup.CreateBackup
)

type backupServerAPI struct {
	dbinfo  *backup.DBConnInfo
	storage backup.BackupStorage
}

func NewBackupServerAPI(
	dbinfo *backup.DBConnInfo, stor backup.BackupStorage,
) BackupAPI {
	return newBackupServerAPI(dbinfo, stor)
}

var newBackupServerAPI = func(
	dbinfo *backup.DBConnInfo, stor backup.BackupStorage,
) BackupAPI {
	api := backupServerAPI{
		dbinfo:  dbinfo,
		storage: stor,
	}
	return &api
}

func (ba *backupServerAPI) Create(name string) (*backup.BackupInfo, string, error) {
	info, err := createBackup(ba.dbinfo, ba.storage, name, nil)
	if err != nil {
		return nil, "", fmt.Errorf("error creating backup: %v", err)
	}
	// We have to wait, so we might as well include the URL.
	URL, err := ba.storage.URL(info.Name)
	if err != nil {
		return info, "", nil
	}
	return info, URL, nil
}
