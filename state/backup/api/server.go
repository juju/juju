// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backup"
)

var (
	createBackup = backup.CreateBackup
	getDBInfo    = backup.NewDBInfo
	getStorage   = backup.NewBackupStorage
)

type backupServerAPI struct {
	st      *state.State
	storage backup.BackupStorage
}

func NewBackupServerAPI(st *state.State) (BackupAPI, error) {
	return newBackupServerAPI(st)
}

var newBackupServerAPI = func(st *state.State) (BackupAPI, error) {
	storage, err := getStorage(st, nil)
	if err != nil {
		return nil, fmt.Errorf("error opening backup storage: %v", err)
	}
	api := backupServerAPI{
		st:      st,
		storage: storage,
	}
	return &api, nil
}

func (ba *backupServerAPI) Create(name string) (*backup.BackupInfo, string, error) {
	dbinfo := getDBInfo(ba.st)
	info, err := createBackup(dbinfo, ba.storage, name, nil)
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
