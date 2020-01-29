// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils/exec"
)

// ShimExec is used to indirect command-line interactions.
// A mock for this is "patched" over the the listed methods by the test suite.
// This should be phased out in favour of the fileOps approach below.
type ShimExec interface {
	RunCommands(args exec.RunParams) (*exec.ExecResponse, error)
}

// fileOps is a shim wrapping file-system operations,
// avoiding a hard dependency on os and io/ioutil function calls.
type fileOps struct{}

// Remove (FileOps) deletes the file with the input name.
// If the file does not exist, this fact is ignored.
func (f fileOps) Remove(name string) error {
	if _, err := os.Stat(name); os.IsNotExist(err) {
		return nil
	}
	return errors.Trace(os.Remove(name))
}

// RemoveAll (FileOps) recursively deletes everything under the input path.
// If the path does not exist, this fact is ignored.
func (f fileOps) RemoveAll(name string) error {
	return errors.Trace(os.RemoveAll(name))
}

// WriteFile (FileOps) writes the input data to a file with the input name.
// We call Remove because legacy versions of the file may be a dangling
// symbolic link, in which case attempting to write would return an error.
func (f fileOps) WriteFile(fileName string, data []byte, perm os.FileMode) error {
	_ = os.Remove(fileName)
	return errors.Trace(ioutil.WriteFile(fileName, data, perm))
}
