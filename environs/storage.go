// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/log"
	"strings"
)

// EmptyStorage holds a StorageReader object that contains no files and
// offers no URLs.
var EmptyStorage StorageReader = emptyStorage{}

type emptyStorage struct{}

// File named `verificationFilename` in the storage will contain
// `verificationContent`.  This is also used to differentiate between
// Python Juju and juju-core environments, so change the content with
// care (and update CheckEnvironment below when you do that).
const verificationFilename string = "bootstrap-verify"
const verificationContent = "juju-core storage writing verified: ok\n"

var VerifyStorageError error = fmt.Errorf(
	"provider storage is not writable")

var InvalidEnvironmentError error = fmt.Errorf(
	"Environment is not a juju-core environment")

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

// Checks if an environment has a bootstrap-verify that is written by
// juju-core commands (as compared to one being written by Python juju).
//
// If there is no bootstrap-verify file in the storage, it is still
// considered to be a Juju-core environment since early versions have
// not written it out.
//
// Returns InvalidEnvironmentError on failure, nil otherwise.
func CheckEnvironment(environ Environ) error {
	storage := environ.Storage()
	reader, err := storage.Get(verificationFilename)
	if errors.IsNotFoundError(err) {
		fmt.Printf("%v\n", err)
		// When verification file does not exist, this is a juju-core
		// environment.
		return nil
	} else if err == nil {
		content, err := ioutil.ReadAll(reader)
		if err == nil && string(content) == verificationContent {
			// Content matches what juju-core puts in the
			// verificationFilename.
			return nil
		}
	} else {
		fmt.Printf("%v\n", err)
	}
	return InvalidEnvironmentError
}
