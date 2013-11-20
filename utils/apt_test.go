// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
)

type AptSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&AptSuite{})

func (s *AptSuite) TestOnePackage(c *gc.C) {
	cmdChan, cleanup := testbase.HookCommandOutput(&AptCommandOutput, []byte{}, nil)
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

func (s *AptSuite) TestAptGetError(c *gc.C) {
	const expected = `E: frobnicator failure detected`
	cmdError := fmt.Errorf("error")
	cmdExpectedError := fmt.Errorf("apt-get failed: error")
	cmdChan, cleanup := testbase.HookCommandOutput(&AptCommandOutput, []byte(expected), cmdError)
	defer cleanup()
	err := AptGetInstall("foo")
	c.Assert(err, gc.DeepEquals, cmdExpectedError)
	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-get", "--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
		"install", "foo",
	})
}

func (s *AptSuite) TestConfigProxyEmpty(c *gc.C) {
	cmdChan, cleanup := testbase.HookCommandOutput(&AptCommandOutput, []byte{}, nil)
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
	cmdChan, cleanup := testbase.HookCommandOutput(&AptCommandOutput, []byte(expected), nil)
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

func (s *AptSuite) TestConfigProxyConfiguredFilterOutput(c *gc.C) {
	const (
		output = `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";`
		expected = `Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";`
	)
	cmdChan, cleanup := testbase.HookCommandOutput(&AptCommandOutput, []byte(output), nil)
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

func (s *AptSuite) TestConfigProxyError(c *gc.C) {
	const expected = `E: frobnicator failure detected`
	cmdError := fmt.Errorf("error")
	cmdExpectedError := fmt.Errorf("apt-config failed: error")
	cmdChan, cleanup := testbase.HookCommandOutput(&AptCommandOutput, []byte(expected), cmdError)
	defer cleanup()
	out, err := AptConfigProxy()
	c.Assert(err, gc.DeepEquals, cmdExpectedError)
	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-config", "dump", "Acquire::http::Proxy",
		"Acquire::https::Proxy", "Acquire::ftp::Proxy",
	})
	c.Assert(out, gc.Equals, "")
}
