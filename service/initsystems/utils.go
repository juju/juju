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

// TODO(ericsnow) Move Shell, etc. to the utils repo (utils/exec?).

// Shell is used to interact with the shell on a host.
type Shell interface {
	// RunCommand runs the provided shell command and args and returns
	// the shell output.
	RunCommand(cmd string, args ...string) ([]byte, error)

	// RunCommand runs the provided shell command as-is and returns
	// the shell output.
	RunCommandStr(cmd string) ([]byte, error)
}

// LocalShell is a Shell implementation that operates on the local host.
type LocalShell struct{}

// RunCommand implements Shell.
func (s LocalShell) RunCommand(cmd string, args ...string) ([]byte, error) {
	// TODO(ericsnow) Use uexec here (or call s.RunCommandStr).
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

// RunCommandStr implements Shell.
func (s LocalShell) RunCommandStr(cmd string) ([]byte, error) {
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
	return out.Stdout, nil
}

// FakeShell is a Shell implementation for use in testing.
type FakeShell struct {
	*testing.Fake
	// Out is the return value for RunCommand and RunCommandStr for
	// successive calls.
	Out [][]byte
}

// NewFakeShell creates a new FakeShell and returns it.
func NewFakeShell() *FakeShell {
	return &FakeShell{
		Fake: &testing.Fake{},
	}
}

// SetOut sets the sequence of RunCommand* return values.
func (fs *FakeShell) SetOut(out ...[]byte) {
	fs.Out = out
}

// SetOutString sets the sequence of RunCommand* return values.
func (fs *FakeShell) SetOutString(out ...string) {
	fs.Out = make([][]byte, len(out))
	for i, v := range out {
		fs.Out[i] = []byte(v)
	}
}

// AddToOut sets the sequence of RunCommand* return values.
func (fs *FakeShell) AddToOut(out ...string) {
	for _, v := range out {
		fs.Out = append(fs.Out, []byte(v))
	}
}

func (fs *FakeShell) out() []byte {
	if len(fs.Out) == 0 {
		return nil
	}
	out := fs.Out[0]
	fs.Out = fs.Out[1:]
	return out
}

// RunCommand implements Shell.
func (fs *FakeShell) RunCommand(cmd string, args ...string) ([]byte, error) {
	fs.AddCall("RunCommand", testing.FakeCallArgs{
		"cmd":  cmd,
		"args": args,
	})
	return fs.out(), fs.Err()
}

// RunCommandStr implements Shell.
func (fs *FakeShell) RunCommandStr(cmd string) ([]byte, error) {
	fs.AddCall("RunCommandStr", testing.FakeCallArgs{
		"cmd": cmd,
	})
	return fs.out(), fs.Err()
}
