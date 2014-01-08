// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils/ssh"
)

type SSHCommandSuite struct {
	testbase.LoggingSuite
	testbin string
	fakessh string
}

var _ = gc.Suite(&SSHCommandSuite{})

func (s *SSHCommandSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.testbin = c.MkDir()
	s.fakessh = filepath.Join(s.testbin, "ssh")
	err := ioutil.WriteFile(s.fakessh, nil, 0755)
	c.Assert(err, gc.IsNil)
	s.PatchEnvironment("PATH", s.testbin)
}

func (s *SSHCommandSuite) TestCommand(c *gc.C) {
	s.assertCommandArgs(c, "localhost", []string{"echo", "123"}, []string{
		"ssh", "-o", "StrictHostKeyChecking no", "localhost", "--", "echo", "123",
	})
}

func (s *SSHCommandSuite) assertCommandArgs(c *gc.C, hostname string, command []string, expected []string) {
	cmd := ssh.Command("localhost", []string{"echo", "123"})
	c.Assert(cmd, gc.NotNil)
	c.Assert(cmd.Args, gc.DeepEquals, expected)
}

func (s *SSHCommandSuite) TestCommandSSHPass(c *gc.C) {
	// First create a fake sshpass, but don't set SSHPASS
	fakesshpass := filepath.Join(s.testbin, "sshpass")
	err := ioutil.WriteFile(fakesshpass, nil, 0755)
	s.assertCommandArgs(c, "localhost", []string{"echo", "123"}, []string{
		"ssh", "-o", "StrictHostKeyChecking no", "localhost", "--", "echo", "123",
	})

	// Now set SSHPASS.
	s.PatchEnvironment("SSHPASS", "anyoldthing")
	s.assertCommandArgs(c, "localhost", []string{"echo", "123"}, []string{
		fakesshpass, "-e", "ssh", "-o", "StrictHostKeyChecking no", "localhost", "--", "echo", "123",
	})

	// Finally, remove sshpass from $PATH.
	err = os.Remove(fakesshpass)
	c.Assert(err, gc.IsNil)
	s.assertCommandArgs(c, "localhost", []string{"echo", "123"}, []string{
		"ssh", "-o", "StrictHostKeyChecking no", "localhost", "--", "echo", "123",
	})
}
