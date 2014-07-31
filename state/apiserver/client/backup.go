// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"io"
	"path"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backup"
	"github.com/juju/juju/state/backup/api"
)

func newDBInfo(st *state.State) *backup.DBConnInfo {
	mgoInfo := st.MongoConnectionInfo()

	dbinfo := backup.DBConnInfo{
		Hostname: mgoInfo.Addrs[0],
		Password: mgoInfo.Password,
	}
	// TODO(dfc) Backup should take a Tag
	if mgoInfo.Tag != nil {
		dbinfo.Username = mgoInfo.Tag.String()
	}
	return &dbinfo
}

type envFileStorage struct {
	envStor storage.Storage
	root    string
}

func newFileStorage(envStor storage.Storage, root string) backup.FileStorage {
	stor := envFileStorage{
		envStor: envStor,
		root:    root,
	}
	return &stor
}

func (s *envFileStorage) path(id string) string {
	// Use of path.Join instead of filepath.Join is intentional - this
	// is an environment storage path not a filesystem path.
	return path.Join(s.root, id)
}

func (s *envFileStorage) AddFile(id string, file io.Reader, size int64) error {
	return s.envStor.Put(s.path(id), file, size)
}

func (s *envFileStorage) File(id string) (io.ReadCloser, error) {
	return s.envStor.Get(s.path(id))
}

func (s *envFileStorage) URL(id string) (string, error) {
	return s.envStor.URL(s.path(id))
}

func newBackupStorage(st *state.State) (backup.BackupStorage, error) {
	envStor, err := environs.GetStorage(st)
	if err != nil {
		return nil, fmt.Errorf("error getting environment storage: %v", err)
	}
	docs := state.NewStateStorage(st, "backups") // state.backupsC
	files := newFileStorage(envStor, backup.StorageRoot)
	return backup.NewBackupStorage(docs, files), nil
}

func (c *Client) Backup(args api.BackupArgs) (p api.BackupResult, err error) {
	dbinfo := newDBInfo(c.api.state)
	stor, err := newBackupStorage(c.api.state)
	if err != nil {
		return p, err
	}
	backupAPI := api.NewBackupServerAPI(dbinfo, stor)

	result, err := api.HandleRequest(backupAPI, &args)
	if err != nil {
		return p, err
	}
	p = *result
	return p, nil
}
