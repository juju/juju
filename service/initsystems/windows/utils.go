// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"github.com/juju/utils/fs"

	"github.com/juju/juju/service/initsystems"
)

type fileOperations interface {
	ReadFile(filename string) ([]byte, error)
}

func newFileOperations() fileOperations {
	return &fs.Ops{}
}

type cmdRunner interface {
	RunCommandStr(cmd string) ([]byte, error)
}

func newCmdRunner() cmdRunner {
	return &initsystems.LocalShell{}
}
