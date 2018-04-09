package systemd

import (
	"os"

	"github.com/juju/utils/exec"
)

// Interfaces exposing call surfaces
// To regenerate the mock for these interfaces,
// run "go generate" from the package directory.
//go:generate mockgen -package systemd -destination shims_mock.go github.com/juju/juju/service/systemd ShimFileOps,ShimExec

type ShimFileOps interface {
	RemoveAll(name string) error
	MkdirAll(dirname string) error
	CreateFile(filename string, data []byte, perm os.FileMode) error
}

type ShimExec interface {
	RunCommands(args exec.RunParams) (*exec.ExecResponse, error)
}
