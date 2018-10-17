// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

import (
	"os"

	"github.com/juju/utils/exec"
)

// Interfaces for call surfaces of command-line and file-system side-effects.
// These are "patched" over the existing methods by the test suite.
// To regenerate the mock for these interfaces,
// run "go generate" from the package directory.
//go:generate mockgen -package systemd -destination shims_mock_test.go github.com/juju/juju/service/systemd ShimFileOps,ShimExec

type ShimFileOps interface {
	RemoveAll(name string) error
	MkdirAll(dirname string) error
	CreateFile(filename string, data []byte, perm os.FileMode) error
}

type ShimExec interface {
	RunCommands(args exec.RunParams) (*exec.ExecResponse, error)
}
