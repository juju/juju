package systemd

import (
	"os"

	"github.com/juju/utils/exec"
)

type ShimFileOps interface {
	RemoveAll(name string) error
	MkdirAll(dirname string) error
	CreateFile(filename string, data []byte, perm os.FileMode) error
}

type ShimExec interface {
	RunCommands(args exec.RunParams) (*exec.ExecResponse, error)
}
