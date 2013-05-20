// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io"
)

// EmptyStorage holds a StorageReader object that contains no files and
// offers no URLs.
var EmptyStorage StorageReader = emptyStorage{}

type emptyStorage struct{}

func (s emptyStorage) Get(name string) (io.ReadCloser, error) {
	return nil, &NotFoundError{fmt.Errorf("file %q not found", name)}
}

func (s emptyStorage) URL(name string) (string, error) {
	return "", fmt.Errorf("file %q not found", name)
}

func (s emptyStorage) List(prefix string) ([]string, error) {
	return nil, nil
}
