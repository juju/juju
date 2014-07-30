// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

// This is a separate package due to import cycles with state.

import (
	"fmt"
	"io"
	"path"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/state"
)

const storageRoot = "/backup"

// BackupStorage is the API for the storage used by backups.
type BackupStorage interface {
	Add(info *BackupInfo, archive io.Reader) error
	Info(name string) (*BackupInfo, error)
	// In common with storage.StorageReader:
	URL(name string) (string, error)
}

// NewBackupStorage returns a new backup storage based on the state.
func NewBackupStorage(
	st *state.State, stor storage.Storage,
) (BackupStorage, error) {
	var err error
	if stor == nil {
		stor, err = environs.GetStorage(st)
		if err != nil {
			return nil, fmt.Errorf("error getting environment storage: %v", err)
		}
	}

	bstor := backupStorage{
		st:   st,
		stor: stor,
	}
	return &bstor, nil
}

type backupStorage struct {
	st   *state.State
	stor storage.Storage
}

func (s *backupStorage) archivePath(name string) string {
	// Use of path.Join instead of filepath.Join is intentional - this
	// is an environment storage path not a filesystem path.
	return path.Join(storageRoot, name)
}

func (s *backupStorage) Info(name string) (*BackupInfo, error) {
	// XXX Pull from state.
	return nil, fmt.Errorf("not finished")
}

func (s *backupStorage) URL(name string) (string, error) {
	path := s.archivePath(name)
	return s.stor.URL(path)
}

func (s *backupStorage) Add(info *BackupInfo, archive io.Reader) error {
	// XXX Add to state.

	path := s.archivePath(info.Name)
	err := s.stor.Put(path, archive, info.Size)
	if err != nil {
		return err
	}
	return nil
}
