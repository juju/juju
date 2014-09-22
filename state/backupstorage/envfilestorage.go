// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupstorage

import (
	"io"
	"path"

	"github.com/juju/utils/filestorage"

	"github.com/juju/juju/environs/storage"
)

const envStorageRoot = "/"

// Ensure we satisfy the interface.
var _ filestorage.RawFileStorage = (*envFileStorage)(nil)

type envFileStorage struct {
	envStor storage.Storage
	root    string
}

func newEnvFileStorage(envStor storage.Storage, root string) filestorage.RawFileStorage {
	// Due to circular imports we cannot simply get the storage from
	// state.State using environs.GetStorage().
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

func (s *envFileStorage) File(id string) (io.ReadCloser, error) {
	return s.envStor.Get(s.path(id))
}

func (s *envFileStorage) AddFile(id string, file io.Reader, size int64) error {
	return s.envStor.Put(s.path(id), file, size)
}

func (s *envFileStorage) RemoveFile(id string) error {
	return s.envStor.Remove(s.path(id))
}

// Close implements io.Closer.Close.
func (s *envFileStorage) Close() error {
	return nil
}
