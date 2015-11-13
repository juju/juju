// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"gopkg.in/yaml.v2"
)

// WriteYaml marshals obj as yaml to a temporary file in the same directory
// as path, than atomically replaces path with the temporary file.
func WriteYaml(path string, obj interface{}) error {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return errors.Trace(err)
	}
	dir := filepath.Dir(path)
	f, err := ioutil.TempFile(dir, "juju")
	if err != nil {
		return errors.Trace(err)
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()      // don't leak file handle
		os.Remove(tmp) // don't leak half written files on disk
		return errors.Trace(err)
	}
	// Explicitly close the file before moving it. This is needed on Windows
	// where the OS will not allow us to move a file that still has an open
	// file handle. Must check the error on close because filesystems can delay
	// reporting errors until the file is closed.
	if err := f.Close(); err != nil {
		os.Remove(tmp) // don't leak half written files on disk
		return errors.Trace(err)
	}

	// ioutils.TempFile creates files 0600, but this function has a contract
	// that files will be world readable, 0644 after replacement.
	if err := os.Chmod(tmp, 0644); err != nil {
		os.Remove(tmp) // remove file with incorrect permissions.
		return errors.Trace(err)
	}

	return utils.ReplaceFile(tmp, path)
}

// ReadYaml unmarshals the yaml contained in the file at path into obj. See
// goyaml.Unmarshal.
func ReadYaml(path string, obj interface{}) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err // cannot wrap here because callers check for NotFound.
	}
	return yaml.Unmarshal(data, obj)
}
