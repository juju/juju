// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

type AptSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&AptSuite{})

func (s *AptSuite) TestOnePackage(c *gc.C) {
	cmdChan, cleanup := testbase.HookCommandOutput(&utils.AptCommandOutput, []byte{}, nil)
	defer cleanup()
	err := utils.AptGetInstall("test-package")
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
	cmdChan, cleanup := testbase.HookCommandOutput(&utils.AptCommandOutput, []byte(expected), cmdError)
	defer cleanup()
	err := utils.AptGetInstall("foo")
	c.Assert(err, gc.DeepEquals, cmdExpectedError)
	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-get", "--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
		"install", "foo",
	})
}

func (s *AptSuite) TestConfigProxyEmpty(c *gc.C) {
	cmdChan, cleanup := testbase.HookCommandOutput(&utils.AptCommandOutput, []byte{}, nil)
	defer cleanup()
	out, err := utils.AptConfigProxy()
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
	cmdChan, cleanup := testbase.HookCommandOutput(&utils.AptCommandOutput, []byte(expected), nil)
	defer cleanup()
	out, err := utils.AptConfigProxy()
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
	cmdChan, cleanup := testbase.HookCommandOutput(&utils.AptCommandOutput, []byte(output), nil)
	defer cleanup()
	out, err := utils.AptConfigProxy()
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
	cmdChan, cleanup := testbase.HookCommandOutput(&utils.AptCommandOutput, []byte(expected), cmdError)
	defer cleanup()
	out, err := utils.AptConfigProxy()
	c.Assert(err, gc.DeepEquals, cmdExpectedError)
	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-config", "dump", "Acquire::http::Proxy",
		"Acquire::https::Proxy", "Acquire::ftp::Proxy",
	})
	c.Assert(out, gc.Equals, "")
}

func (s *AptSuite) patchExecutable(c *gc.C, execName string, script string) {
	dir := c.MkDir()
	s.PatchEnvironment("PATH", dir)
	filename := filepath.Join(dir, execName)
	ioutil.WriteFile(filename, []byte(script), 0755)
}

func (s *AptSuite) TestRunCommandCombinesOutput(c *gc.C) {
	content := `#!/bin/bash --norc
echo stdout
echo stderr 1>&2
`
	s.patchExecutable(c, "test-output", content)
	output, err := utils.RunCommand("test-output")
	c.Assert(err, gc.IsNil)
	c.Assert(output, gc.Equals, "stdout\nstderr\n")
}

func (s *AptSuite) TestRunCommandNonZeroExit(c *gc.C) {
	content := `#!/bin/bash --norc
echo stdout
exit 1
`
	s.patchExecutable(c, "test-output", content)
	output, err := utils.RunCommand("test-output")
	c.Assert(err, gc.ErrorMatches, `exit status 1`)
	c.Assert(output, gc.Equals, "stdout\n")
}

func (s *AptSuite) patchLsbRelease(c *gc.C, name string) {
	content := fmt.Sprintf("#!/bin/bash --norc\necho %s", name)
	s.patchExecutable(c, "lsb_release", content)
}

func (s *AptSuite) TestIsUbuntu(c *gc.C) {
	s.patchLsbRelease(c, "Ubuntu")
	c.Assert(utils.IsUbuntu(), jc.IsTrue)
}

func (s *AptSuite) TestIsNotUbuntu(c *gc.C) {
	s.patchLsbRelease(c, "Windows NT")
	c.Assert(utils.IsUbuntu(), jc.IsFalse)
}

func (s *AptSuite) patchDpkgQuery(c *gc.C, installed bool) {
	rc := 0
	if !installed {
		rc = 1
	}
	content := fmt.Sprintf("#!/bin/bash --norc\nexit %v", rc)
	s.patchExecutable(c, "dpkg-query", content)
}

func (s *AptSuite) TestIsPackageInstalled(c *gc.C) {
	s.patchDpkgQuery(c, true)
	c.Assert(utils.IsPackageInstalled("foo-bar"), jc.IsTrue)
}

func (s *AptSuite) TestIsPackageNotInstalled(c *gc.C) {
	s.patchDpkgQuery(c, false)
	c.Assert(utils.IsPackageInstalled("foo-bar"), jc.IsFalse)
}
