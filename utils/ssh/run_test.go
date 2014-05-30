// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils/ssh"
)

const shortWait = 50 * time.Millisecond

type ExecuteSSHCommandSuite struct {
	testing.IsolationSuite
	originalPath string
	testbin      string
	fakessh      string
}

var _ = gc.Suite(&ExecuteSSHCommandSuite{})

func (s *ExecuteSSHCommandSuite) SetUpSuite(c *gc.C) {
	s.originalPath = os.Getenv("PATH")
	s.IsolationSuite.SetUpSuite(c)
}

func (s *ExecuteSSHCommandSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	err := os.Setenv("PATH", s.originalPath)
	c.Assert(err, gc.IsNil)
	s.testbin = c.MkDir()
	s.fakessh = filepath.Join(s.testbin, "ssh")
	s.PatchEnvPathPrepend(s.testbin)

}

func (s *ExecuteSSHCommandSuite) fakeSSH(c *gc.C, cmd string) {
	err := ioutil.WriteFile(s.fakessh, []byte(cmd), 0755)
	c.Assert(err, gc.IsNil)
}

func (s *ExecuteSSHCommandSuite) TestCaptureOutput(c *gc.C) {
	s.fakeSSH(c, echoSSH)

	response, err := ssh.ExecuteCommandOnMachine(ssh.ExecParams{
		Host:    "hostname",
		Command: "sudo apt-get update\nsudo apt-get upgrade",
		Timeout: shortWait,
	})

	c.Assert(err, gc.IsNil)
	c.Assert(response.Code, gc.Equals, 0)
	c.Assert(string(response.Stdout), gc.Equals, "sudo apt-get update\nsudo apt-get upgrade\n")
	c.Assert(string(response.Stderr), gc.Equals,
		"-o StrictHostKeyChecking no -o PasswordAuthentication no hostname /bin/bash -s\n")
}

func (s *ExecuteSSHCommandSuite) TestIdentityFile(c *gc.C) {
	s.fakeSSH(c, echoSSH)

	response, err := ssh.ExecuteCommandOnMachine(ssh.ExecParams{
		IdentityFile: "identity-file",
		Host:         "hostname",
		Timeout:      shortWait,
	})

	c.Assert(err, gc.IsNil)
	c.Assert(string(response.Stderr), jc.Contains, " -i identity-file ")
}

func (s *ExecuteSSHCommandSuite) TestTimoutCaptureOutput(c *gc.C) {
	s.fakeSSH(c, slowSSH)

	response, err := ssh.ExecuteCommandOnMachine(ssh.ExecParams{
		IdentityFile: "identity-file",
		Host:         "hostname",
		Command:      "ignored",
		Timeout:      shortWait,
	})

	c.Check(err, gc.ErrorMatches, "command timed out")
	c.Assert(response.Code, gc.Equals, 0)
	c.Assert(string(response.Stdout), gc.Equals, "stdout\n")
	c.Assert(string(response.Stderr), gc.Equals, "stderr\n")
}

func (s *ExecuteSSHCommandSuite) TestCapturesReturnCode(c *gc.C) {
	s.fakeSSH(c, passthroughSSH)

	response, err := ssh.ExecuteCommandOnMachine(ssh.ExecParams{
		IdentityFile: "identity-file",
		Host:         "hostname",
		Command:      "echo stdout; exit 42",
		Timeout:      shortWait,
	})

	c.Check(err, gc.IsNil)
	c.Assert(response.Code, gc.Equals, 42)
	c.Assert(string(response.Stdout), gc.Equals, "stdout\n")
	c.Assert(string(response.Stderr), gc.Equals, "")
}

// echoSSH outputs the command args to stderr, and copies stdin to stdout
var echoSSH = `#!/bin/bash
# Write the args to stderr
echo "$*" >&2
cat /dev/stdin
`

// slowSSH sleeps for a while after outputting some text to stdout and stderr
var slowSSH = `#!/bin/bash
echo "stderr" >&2
echo "stdout"
sleep 5s
`

// passthroughSSH creates an ssh that executes stdin.
var passthroughSSH = `#!/bin/bash -s`
