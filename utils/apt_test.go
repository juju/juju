// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"os/exec"

	gc "launchpad.net/gocheck"
)

type AptSuite struct{}

var _ = gc.Suite(&AptSuite{})

// hookRunCommand intercepts runCommand to a function that passes the actual
// command back via a channel, and returns the error passed into this function.
// It also returns a cleanup function so you can restore the original function
func (s *AptSuite) hookRunCommand(err error) (<-chan *exec.Cmd, func()) {
	cmdChan := make(chan *exec.Cmd, 1)
	origRunCommand := runCommand
	cleanup := func() {
		runCommand = origRunCommand
	}
	runCommand = func(cmd *exec.Cmd) error {
		cmdChan <- cmd
		return err
	}
	return cmdChan, cleanup
}

// hookCommandOutput intercepts commandOutput to a function that passes the
// actual command and it's output back via a channel, and returns the error
// passed into this function.  It also returns a cleanup function so you can
// restore the original function
func (s *AptSuite) hookCommandOutput(output []byte, err error) (<-chan *exec.Cmd, func()) {
	cmdChan := make(chan *exec.Cmd, 1)
	origCommandOutput := commandOutput
	cleanup := func() {
		commandOutput = origCommandOutput
	}
	commandOutput = func(cmd *exec.Cmd) ([]byte, error) {
		cmdChan <- cmd
		return output, err
	}
	return cmdChan, cleanup
}

func (s *AptSuite) TestOnePackage(c *gc.C) {
	cmdChan, cleanup := s.hookRunCommand(nil)
	defer cleanup()
	err := AptGetInstall("test-package")
	c.Assert(err, gc.IsNil)
	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-get", "--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
		"install", "test-package",
	})
	c.Assert(cmd.Env[len(cmd.Env)-1], gc.Equals, "DEBIAN_FRONTEND=noninteractive")
}

func (s *AptSuite) TestConfigProxyEmpty(c *gc.C) {
	cmdChan, cleanup := s.hookCommandOutput([]byte{}, nil)
	defer cleanup()
	out, err := AptConfigProxy()
	c.Assert(err, gc.IsNil)
	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-config", "dump", "Acquire::http::Proxy",
		"Acquire::https::Proxy", "Acquire::ftp::Proxy",
	})
	c.Assert(out, gc.Equals, "")
}

func (s *AptSuite) TestConfigProxyConfigured(c *gc.C) {
	const expected = `Acquire::http::Proxy "10.0.3.1:3142";
Acquire::https::Proxy "false";`
	cmdChan, cleanup := s.hookCommandOutput([]byte(expected), nil)
	defer cleanup()
	out, err := AptConfigProxy()
	c.Assert(err, gc.IsNil)
	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-config", "dump", "Acquire::http::Proxy",
		"Acquire::https::Proxy", "Acquire::ftp::Proxy",
	})
	c.Assert(out, gc.Equals, expected)
}
