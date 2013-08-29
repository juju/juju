// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io"
	"strings"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/utils"
)

// EmptyStorage composes EmptyStorageReader and EmptyStorageWriter.
var EmptyStorage Storage = struct {
	StorageReader
	StorageWriter
}{EmptyStorageReader, EmptyStorageWriter}

// EmptyStorageReader is a StorageReader implementation that contains no
// files and offers no URLs.
var EmptyStorageReader StorageReader = emptyStorageReader{}

// EmptyStorageReader is a StorageWriter object that produces an error
// for all operations.
var EmptyStorageWriter StorageWriter = emptyStorageWriter{}

type emptyStorageReader struct{}
type emptyStorageWriter struct{}

// File named `verificationFilename` in the storage will contain
// `verificationContent`.  This is also used to differentiate between
// Python Juju and juju-core environments, so change the content with
// care (and update CheckEnvironment below when you do that).
const verificationFilename string = "bootstrap-verify"
const verificationContent = "juju-core storage writing verified: ok\n"

var VerifyStorageError error = fmt.Errorf(
	"provider storage is not writable")

func (s emptyStorageReader) Get(name string) (io.ReadCloser, error) {
	return nil, errors.NotFoundf("file %q", name)
}

func (s emptyStorageReader) URL(name string) (string, error) {
	return "", fmt.Errorf("file %q not found", name)
}

// ConsistencyStrategy is specified in the StorageReader interface.
func (s emptyStorageReader) ConsistencyStrategy() utils.AttemptStrategy {
	return utils.AttemptStrategy{}
}

func (s emptyStorageReader) List(prefix string) ([]string, error) {
	return nil, nil
}

func (s emptyStorageWriter) Put(name string, r io.Reader, length int64) error {
	return fmt.Errorf("cannot put file %q to empty storage", name)
}

func (s emptyStorageWriter) Remove(name string) error {
	return fmt.Errorf("cannot remove file %q from empty storage", name)
}

func (s emptyStorageWriter) RemoveAll() error {
	return fmt.Errorf("cannot remove files from empty storage")
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
