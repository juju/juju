package main

import (
	"os/exec"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
)

var _ = Suite(&SSHSuite{})

type SSHSuite struct {
	testing.JujuConnSuite
	cmd *exec.Cmd
}

func (s *SSHSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.startSSHDaemon(c)
}

func (s *SSHSuite) TearDownTest(c *C) {
	defer s.shutdownSSHDaemon(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *SSHSuite) startSSHDaemon(c *C) {
	// TODO(dfc)
}

func (s *SSHSuite) shutdownSSHDaemon(c *C) {
	// TODO(dfc)
}
