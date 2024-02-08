// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/juju/sockets"
	coretesting "github.com/juju/juju/testing"
)

type CAASUnitInitSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&CAASUnitInitSuite{})

func (s *CAASUnitInitSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *CAASUnitInitSuite) newCommand(c *gc.C, st *testing.Stub) *CAASUnitInitCommand {
	cmd := NewCAASUnitInitCommand()
	cmd.copyFunc = func(src, dst string) error {
		st.AddCall("Copy", src, dst)
		return st.NextErr()
	}
	cmd.symlinkFunc = func(src, dst string) error {
		st.AddCall("Symlink", src, dst)
		return st.NextErr()
	}
	cmd.removeAllFunc = func(path string) error {
		st.AddCall("RemoveAll", path)
		return st.NextErr()
	}
	cmd.mkdirAllFunc = func(path string, mode os.FileMode) error {
		st.AddCall("MkdirAll", path, mode)
		return st.NextErr()
	}
	cmd.statFunc = func(path string) (os.FileInfo, error) {
		st.AddCall("Stat", path)
		return nil, st.NextErr()
	}
	cmd.waitForPIDFunc = func(pid int) {
		st.AddCall("waitForPID", pid)
	}
	return cmd
}

func (s *CAASUnitInitSuite) checkCommand(c *gc.C, cmd *CAASUnitInitCommand, args []string,
	unit string, operatorFile string,
	operatorCACertFile string, charmDir string,
) []testing.StubCall {
	ctx, err := cmdtesting.RunCommand(c, cmd, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)

	toolsPath := "/var/lib/juju/tools/" + unit
	agentPath := "/var/lib/juju/agents/" + unit

	// Directory setup
	calls := []testing.StubCall{
		{FuncName: "Stat", Args: []interface{}{"/var/lib/juju/tools/jujuc"}},
		{FuncName: "RemoveAll", Args: []interface{}{toolsPath}},
		{FuncName: "MkdirAll", Args: []interface{}{toolsPath, os.FileMode(0775)}},
	}

	calls = append(calls,
		testing.StubCall{FuncName: "RemoveAll", Args: []interface{}{agentPath}},
		testing.StubCall{FuncName: "MkdirAll", Args: []interface{}{agentPath, os.FileMode(0775)}},
	)

	// Symlinks
	calls = append(calls,
		testing.StubCall{FuncName: "Symlink", Args: []interface{}{"/var/lib/juju/tools/jujud", toolsPath + "/jujud"}},
	)
	for _, cmdName := range jujuc.CommandNames() {
		_ = cmdName
		calls = append(calls,
			testing.StubCall{FuncName: "Symlink", Args: []interface{}{"/var/lib/juju/tools/jujuc", toolsPath + "/" + cmdName}})
	}

	// Copies
	if operatorFile != "" {
		calls = append(calls,
			testing.StubCall{FuncName: "Copy", Args: []interface{}{operatorFile, agentPath + "/operator-client.yaml"}},
		)
	}
	if operatorCACertFile != "" {
		calls = append(calls,
			testing.StubCall{FuncName: "Copy", Args: []interface{}{operatorCACertFile, agentPath + "/ca.crt"}},
		)
	}
	if charmDir != "" {
		calls = append(calls,
			testing.StubCall{FuncName: "Copy", Args: []interface{}{charmDir, agentPath + "/charm"}},
		)
	}

	return calls
}

func (s *CAASUnitInitSuite) TestInitUnit(c *gc.C) {
	args := []string{"--unit", "unit-wow-0",
		"--operator-file", "operator/file/path",
		"--operator-ca-cert-file", "operator/cert/file/path",
		"--charm-dir", "charm/dir"}
	st := &testing.Stub{}
	cmd := s.newCommand(c, st)
	calls := s.checkCommand(c, cmd, args, "unit-wow-0",
		"operator/file/path", "operator/cert/file/path", "charm/dir")
	st.CheckCalls(c, calls)
}

func (s *CAASUnitInitSuite) TestInitUnitWaitSend(c *gc.C) {
	socketName := fmt.Sprintf("@%d", rand.Int63())
	listening := make(chan struct{})
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		st := &testing.Stub{}
		cmd := s.newCommand(c, st)
		cmd.socketName = socketName
		cmd.listenFunc = func(s sockets.Socket) (net.Listener, error) {
			l, err := sockets.Listen(s)
			close(listening)
			return l, err
		}
		calls := s.checkCommand(c, cmd, []string{"--wait"}, "unit-wow-0",
			"operator/file/path", "operator/cert/file/path", "charm/dir")
		calls = append(calls,
			testing.StubCall{FuncName: "waitForPID", Args: []interface{}{os.Getpid()}})
		st.CheckCalls(c, calls)
	}()

	select {
	case <-listening:
	case <-time.After(coretesting.LongWait):
		c.Fatal("failed to listen")
	}

	stdErr := &bytes.Buffer{}
	args := []string{"--send", "--unit", "unit-wow-0",
		"--operator-file", "operator/file/path",
		"--operator-ca-cert-file", "operator/cert/file/path",
		"--charm-dir", "charm/dir"}
	st := &testing.Stub{}
	cmd := s.newCommand(c, st)
	cmd.stdErr = stdErr
	cmd.socketName = socketName
	ctx, err := cmdtesting.RunCommand(c, cmd, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx, gc.NotNil)
	c.Assert(stdErr.Bytes(), gc.Not(gc.HasLen), 0)

	wg.Wait()
}

func (s *CAASUnitInitSuite) TestWaitPID(c *gc.C) {
	var cmd *exec.Cmd
	pid := 0
	cmd = exec.Command("sleep", "2")
	err := cmd.Start()
	c.Assert(err, jc.ErrorIsNil)
	pid = cmd.Process.Pid
	go func() {
		// Need this to reap the zombie process.
		_ = cmd.Wait()
	}()
	c.Assert(pid, gc.Not(gc.Equals), 0)
	waitChan := make(chan struct{})
	go func() {
		defer close(waitChan)
		waitForPID(pid)
	}()
	select {
	case <-waitChan:
	case <-time.After(testing.LongWait):
		c.Errorf("waited too long for waitForPID")
	}
}
