// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"os"

	"github.com/juju/utils/fs"

	"github.com/juju/juju/service/initsystems"
)

type fileOperations interface {
	// Exists implements fs.Operations.
	Exists(name string) (bool, error)

	// ListDir implements fs.Operations.
	ListDir(dirname string) ([]os.FileInfo, error)

	// ReadFile implements fs.Operations.
	ReadFile(filename string) ([]byte, error)

	// Symlink implements fs.Operations.
	Symlink(oldname, newname string) error

	// Readlink implements fs.Operations.
	Readlink(name string) (string, error)

	// RemoveAll implements fs.Operations.
	RemoveAll(name string) error
}

func newFileOperations() fileOperations {
	return &fs.Ops{}
}

type cmdRunner interface {
	RunCommand(cmd string, args ...string) ([]byte, error)
}

func newCmdRunner() cmdRunner {
	return &initsystems.LocalShell{}
}
