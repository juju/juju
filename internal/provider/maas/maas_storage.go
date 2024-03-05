// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/environs/storage"
)

type maasStorage struct {
	// The Environ that this Storage is for.
	environ *maasEnviron

	// The Controller for the MAAS 2 cluster.
	maasController gomaasapi.Controller
}

var _ storage.Storage = (*maasStorage)(nil)

func (stor *maasStorage) prefixWithPrivateNamespace(name string) string {
	return prefixWithPrivateNamespace(stor.environ, name)
}

// All filenames need to be namespaced so they are private to this environment.
// This prevents different environments from interfering with each other.
// We're using the agent name UUID here.
func prefixWithPrivateNamespace(env *maasEnviron, name string) string {
	return env.uuid + "-" + name
}

// Get implements storage.StorageReader
func (stor *maasStorage) Get(name string) (io.ReadCloser, error) {
	name = stor.prefixWithPrivateNamespace(name)
	file, err := stor.maasController.GetFile(name)
	if err != nil {
		if gomaasapi.IsNoMatchError(err) {
			return nil, fmt.Errorf("%s %w", name, errors.NotFound)
		}
		return nil, errors.Trace(err)
	}
	contents, err := file.ReadAll()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return io.NopCloser(bytes.NewReader(contents)), nil
}

// List implements storage.StorageReader
func (stor *maasStorage) List(prefix string) ([]string, error) {
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
func (stor *maasStorage) URL(name string) (string, error) {
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
func (stor *maasStorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// ShouldRetry implements storage.StorageReader
func (stor *maasStorage) ShouldRetry(err error) bool {
	return false
}

// Put implements storage.StorageWriter
func (stor *maasStorage) Put(name string, r io.Reader, length int64) error {
	name = stor.prefixWithPrivateNamespace(name)
	args := gomaasapi.AddFileArgs{Filename: name, Reader: r, Length: length}
	return stor.maasController.AddFile(args)
}

// Remove implements storage.StorageWriter
func (stor *maasStorage) Remove(name string) error {
	name = stor.prefixWithPrivateNamespace(name)
	file, err := stor.maasController.GetFile(name)
	if err != nil {
		return errors.Trace(err)
	}
	return file.Delete()
}

// RemoveAll implements storage.StorageWriter
func (stor *maasStorage) RemoveAll() error {
	return removeAll(stor)
}
