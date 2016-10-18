// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"io"
	"io/ioutil"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/storage"
)

type maas2Storage struct {
	// The Environ that this Storage is for.
	environ *maasEnviron

	// The Controller for the MAAS 2 cluster.
	maasController gomaasapi.Controller
}

var _ storage.Storage = (*maas2Storage)(nil)

func (stor *maas2Storage) prefixWithPrivateNamespace(name string) string {
	return prefixWithPrivateNamespace(stor.environ, name)
}

// Get implements storage.StorageReader
func (stor *maas2Storage) Get(name string) (io.ReadCloser, error) {
	name = stor.prefixWithPrivateNamespace(name)
	file, err := stor.maasController.GetFile(name)
	if err != nil {
		if gomaasapi.IsNoMatchError(err) {
			return nil, errors.Wrap(err, errors.NotFoundf(name))
		}
		return nil, errors.Trace(err)
	}
	contents, err := file.ReadAll()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ioutil.NopCloser(bytes.NewReader(contents)), nil
}

// List implements storage.StorageReader
func (stor *maas2Storage) List(prefix string) ([]string, error) {
	prefix = stor.prefixWithPrivateNamespace(prefix)
	files, err := stor.maasController.Files(prefix)
	if err != nil {
		return nil, errors.Trace(err)
	}
	privatePrefix := stor.prefixWithPrivateNamespace("")
	names := make([]string, len(files))
	for i, file := range files {
		// Remove the private namespacing.
		names[i] = strings.TrimPrefix(file.Filename(), privatePrefix)
	}
	sort.Strings(names)
	return names, nil
}

// URL implements storage.StorageReader
func (stor *maas2Storage) URL(name string) (string, error) {
	name = stor.prefixWithPrivateNamespace(name)
	file, err := stor.maasController.GetFile(name)
	if err != nil {
		return "", errors.Trace(err)
	}
	return file.AnonymousURL(), nil
}

// DefaultConsistencyStrategy implements storage.StorageReader
//
// TODO(katco): 2016-08-09: lp:1611427
func (stor *maas2Storage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// ShouldRetry implements storage.StorageReader
func (stor *maas2Storage) ShouldRetry(err error) bool {
	return false
}

// Put implements storage.StorageWriter
func (stor *maas2Storage) Put(name string, r io.Reader, length int64) error {
	name = stor.prefixWithPrivateNamespace(name)
	args := gomaasapi.AddFileArgs{Filename: name, Reader: r, Length: length}
	return stor.maasController.AddFile(args)
}

// Remove implements storage.StorageWriter
func (stor *maas2Storage) Remove(name string) error {
	name = stor.prefixWithPrivateNamespace(name)
	file, err := stor.maasController.GetFile(name)
	if err != nil {
		return errors.Trace(err)
	}
	return file.Delete()
}

// RemoveAll implements storage.StorageWriter
func (stor *maas2Storage) RemoveAll() error {
	return removeAll(stor)
}
