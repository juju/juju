// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils/v3/exec"
)

// ShimExec is used to indirect command-line interactions.
// A mock for this is "patched" over the the listed methods by the test suite.
// This should be phased out in favour of the fileSystemOps approach below.
type ShimExec interface {
	RunCommands(args exec.RunParams) (*exec.ExecResponse, error)
}

// fileSystemOps is a shim wrapping file-system operations,
// avoiding a hard dependency on os and io/ioutil function calls.
type fileSystemOps struct{}

// Remove (FileSystemOps) deletes the file with the input name.
// If the file does not exist, this fact is ignored.
func (f fileSystemOps) Remove(name string) error {
	if _, err := os.Stat(name); os.IsNotExist(err) {
		return nil
	}
	return errors.Trace(os.Remove(name))
}

// RemoveAll (FileSystemOps) recursively deletes everything under the input path.
// If the path does not exist, this fact is ignored.
func (f fileSystemOps) RemoveAll(name string) error {
	return errors.Trace(os.RemoveAll(name))
}

// WriteFile (FileSystemOps) writes the input data to a file with the input name.
// We call Remove because legacy versions of the file may be a dangling
// symbolic link, in which case attempting to write would return an error.
func (f fileSystemOps) WriteFile(fileName string, data []byte, perm os.FileMode) error {
	_ = os.Remove(fileName)
	return errors.Trace(os.WriteFile(fileName, data, perm))
}
