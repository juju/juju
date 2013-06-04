// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io"
	"strings"
	"launchpad.net/juju-core/errors"
)

// EmptyStorage holds a StorageReader object that contains no files and
// offers no URLs.
var EmptyStorage StorageReader = emptyStorage{}

type emptyStorage struct{}

const VERIFICATION_FILENAME string = "bootstrap-verify"
const VERIFICATION_CONTENT = "juju-core storage writing verified: ok"

func (s emptyStorage) Get(name string) (io.ReadCloser, error) {
	return nil, errors.NotFoundf("file %q", name)
}

func (s emptyStorage) URL(name string) (string, error) {
	return "", fmt.Errorf("file %q not found", name)
}

func (s emptyStorage) List(prefix string) ([]string, error) {
	return nil, nil
}

func VerifyStorage(storage Storage) error {
	reader := strings.NewReader(VERIFICATION_CONTENT)
	err := storage.Put(VERIFICATION_FILENAME, reader,
		int64(len(VERIFICATION_CONTENT)))
	return err
}
