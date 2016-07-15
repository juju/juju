// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package shell

import (
	"os"
	"time"
)

// CommandRenderer provides methods that may be used to generate shell
// commands for a variety of shell and filesystem operations.
type CommandRenderer interface {
	// Chown returns a shell command for changing the ownership of
	// a file or directory. The copies the behavior of os.Chown,
	// though it also supports names in addition to ints.
	Chown(name, user, group string) []string

	// Chmod returns a shell command that sets the given file's
	// permissions. The result is equivalent to os.Chmod.
	Chmod(path string, perm os.FileMode) []string

	// WriteFile returns a shell command that writes the provided
	// content to a file. The command is functionally equivalent to
	// ioutil.WriteFile with permissions from the current umask.
	WriteFile(filename string, data []byte) []string

	// Mkdir returns a shell command for creating a directory. The
	// command is functionally equivalent to os.MkDir using permissions
	// appropriate for a directory.
	Mkdir(dirname string) []string

	// MkdirAll returns a shell command for creating a directory and
	// all missing parent directories. The command is functionally
	// equivalent to os.MkDirAll using permissions appropriate for
	// a directory.
	MkdirAll(dirname string) []string

	// Touch returns a shell command that updates the atime and ctime
	// of the named file. If the provided timestamp is nil then the
	// current time is used. If the file does not exist then it is
	// created. If UTC is desired then Time.UTC() should be called
	// before calling Touch.
	Touch(filename string, timestamp *time.Time) []string
}
