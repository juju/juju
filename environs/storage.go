// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io"
	"strings"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/log"
)

// EmptyStorage holds a StorageReader object that contains no files and
// offers no URLs.
var EmptyStorage StorageReader = emptyStorage{}

type emptyStorage struct{}

const verificationFilename string = "bootstrap-verify"
const verificationContent = "juju-core storage writing verified: ok\n"

var VerifyStorageError error = fmt.Errorf(
	"provider storage is not writable")

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
	reader := strings.NewReader(verificationContent)
	err := storage.Put(verificationFilename, reader,
		int64(len(verificationContent)))
	if err != nil {
		log.Debugf(
			"environs: failed to write bootstrap-verify file: %v",
			err)
		return VerifyStorageError
	}
	return nil
}
