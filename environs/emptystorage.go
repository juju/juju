// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io"

	"github.com/juju/errors"

	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/utils"
)

// EmptyStorage holds a StorageReader object that contains no files and
// offers no URLs.
var EmptyStorage storage.StorageReader = emptyStorage{}

type emptyStorage struct{}

func (s emptyStorage) Get(name string) (io.ReadCloser, error) {
	return nil, errors.NotFoundf("file %q", name)
}

func (s emptyStorage) URL(name string) (string, error) {
	return "", fmt.Errorf("file %q not found", name)
}

func (s emptyStorage) List(prefix string) ([]string, error) {
	return nil, nil
}

// DefaultConsistencyStrategy is specified in the StorageReader interface.
func (s emptyStorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

// ShouldRetry is specified in the StorageReader interface.
func (s emptyStorage) ShouldRetry(err error) bool {
	return false
}
