// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"labix.org/v2/mgo"
)

var logger = loggo.GetLogger("juju.storage")

type gridFSStorage struct {
	namespace string
	session   *mgo.Session
}

var _ ResourceStorage = (*gridFSStorage)(nil)

// NewGridFS returns a ResourceStorage instance backed by a mongo GridFS.
// namespace is used to segregate different sets of data.
func NewGridFS(namespace string, session *mgo.Session) ResourceStorage {
	return &gridFSStorage{
		namespace: namespace,
		session:   session,
	}
}

func (g *gridFSStorage) db() *mgo.Database {
	return g.session.DB("juju")
}

func (g *gridFSStorage) gridFS() *mgo.GridFS {
	return g.db().GridFS(g.namespace)
}

// Get is defined on ResourceStorage.
func (g *gridFSStorage) Get(path string) (io.ReadCloser, error) {
	file, err := g.gridFS().Open(path)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to open GridFS file %q", path)
	}
	return file, nil
}

// Put is defined on ResourceStorage.
func (g *gridFSStorage) Put(path string, r io.Reader, length int64) (checksum string, error error) {
	file, err := g.gridFS().Create(path)
	if err != nil {
		return "", errors.Annotatef(err, "failed to create GridFS file %q", path)
	}
	defer func() {
		if err != nil {
			file.Close()
			if err := g.Remove(path); err != nil {
				logger.Warningf("error cleaning up after failed write: %v", err)
			}
		}
	}()
	num, err := io.Copy(file, r)
	if err != nil {
		return "", errors.Annotatef(err, "failed to write data")
	}
	if num != length {
		return "", errors.Errorf("expected to write %d bytes, only wrote %d", length, num)
	}
	err = file.Close()
	if err != nil {
		return "", errors.Annotatef(err, "failed to flush data")
	}
	return file.MD5(), nil
}

// Remove is defined on ResourceStorage.
func (g *gridFSStorage) Remove(path string) error {
	return g.gridFS().Remove(path)
}
