// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"bytes"
	"os/exec"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	"github.com/juju/utils"
	uexec "github.com/juju/utils/exec"
)

// RetryAttempts provides a uniform retry strategy that may be shared
// across InitSystem implementations.
var RetryAttempts = utils.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 250 * time.Millisecond,
}

// TODO(ericsnow) Combine RunCommand and RunPsCommand.

// RunPsCommand runs the provided shell command on the local host. This
// is useful for InitSystem implementations that cannot make another
// mechanism (e.g. Go bindings).
func RunPsCommand(cmd string) (*uexec.ExecResponse, error) {
	com := uexec.RunParams{
		Commands: cmd,
	}
	out, err := uexec.RunCommands(com)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if out.Code != 0 {
		return nil, errors.Errorf("running %s: %s", cmd, string(out.Stderr))
	}
	return out, nil
}

// TODO(ericsnow) Move Commands, etc. to the utils repo (utils/exec?).

type Shell interface {
	// RunCommand runs the provided shell command and args and returns
	// the shell output.
	RunCommand(cmd string, args ...string) ([]byte, error)
}

// LocalShell is a Shell implementation that operates on the local host.
type LocalShell struct{}

// RunCommand implements Shell.
func (s LocalShell) RunCommand(cmd string, args ...string) ([]byte, error) {
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err == nil {
		return out, nil
	}

	cmdAndArgs := append([]string{cmd}, args...)
	out = bytes.TrimSpace(out)
	if len(out) > 0 {
		return nil, errors.Annotatef(err, "exec %q: (%s)", cmdAndArgs, out)
	}
	return nil, errors.Annotatef(err, "exec %q", cmdAndArgs)
}

type FakeShell struct {
	testing.Fake
	// Out is the return value for RunCommand.
	Out []byte
}

// RunCommand implements Shell.
func (fs *FakeShell) RunCommand(cmd string, args ...string) ([]byte, error) {
	fs.AddCall("RunCommand", testing.FakeCallArgs{
		"cmd":  cmd,
		"args": args,
	})
	return fs.Out, fs.Err()
}
