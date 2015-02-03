// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"os"

	"github.com/juju/utils/fs"

	"github.com/juju/juju/service/initsystems"
)

type fileOperations interface {
	Exists(name string) (bool, error)
	ListDir(dirname string) ([]os.FileInfo, error)
	ReadFile(filename string) ([]byte, error)
	Symlink(oldname, newname string) error
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
