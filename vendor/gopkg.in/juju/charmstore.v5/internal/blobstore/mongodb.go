// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blobstore // import "gopkg.in/juju/charmstore.v5/internal/blobstore"

import (
	"fmt"
	"io"

	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"
)

type mongoBackend struct {
	fs *mgo.GridFS
}

// NewMongoBackend returns a backend implementation which stores
// data in the given MongoDB database, using prefix as a prefix for
// the collections created.
func NewMongoBackend(db *mgo.Database, prefix string) Backend {
	return &mongoBackend{
		fs: db.GridFS(prefix),
	}
}

func (m *mongoBackend) Get(name string) (ReadSeekCloser, int64, error) {
	f, err := m.fs.Open(name)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, 0, errgo.WithCausef(err, ErrNotFound, "backend blob not found")
		}
		return nil, 0, errgo.Mask(err)
	}
	return mongoBackendReader{f}, f.Size(), nil
}

func (m *mongoBackend) Put(name string, r io.Reader, size int64, hash string) error {
	f, err := m.fs.Create(name)
	if err != nil {
		return errgo.Mask(err)
	}
	if err := copyAndCheckHash(f, r, hash); err != nil {
		f.Abort()
		f.Close()
		return errgo.Mask(err)
	}
	if err := f.Close(); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (m *mongoBackend) Remove(name string) error {
	if err := m.fs.Remove(name); err != nil && err != mgo.ErrNotFound {
		return errgo.Notef(err, "cannot delete %q", name)
	}
	return nil
}

func copyAndCheckHash(w io.Writer, r io.Reader, hash string) error {
	hasher := NewHash()
	if _, err := io.Copy(io.MultiWriter(w, hasher), r); err != nil {
		return err
	}
	actualHash := fmt.Sprintf("%x", hasher.Sum(nil))
	if actualHash != hash {
		return errgo.New("hash mismatch")
	}
	return nil
}

// mongoBackendReader translates not-found errors as
// produced by mgo's GridFS into not-found errors as expected
// by the Backend.Get interface contract.
type mongoBackendReader struct {
	ReadSeekCloser
}

func (r mongoBackendReader) Read(buf []byte) (int, error) {
	n, err := r.ReadSeekCloser.Read(buf)
	if err == nil || err == io.EOF {
		return n, err
	}
	if errgo.Cause(err) == mgo.ErrNotFound {
		return n, errgo.WithCausef(err, ErrNotFound, "")
	}
	return n, errgo.Mask(err)
}
