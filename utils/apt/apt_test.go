// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apt_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils/apt"
	"launchpad.net/juju-core/utils/proxy"
)

type AptSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&AptSuite{})

func (s *AptSuite) TestOnePackage(c *gc.C) {
	cmdChan := s.HookCommandOutput(&apt.CommandOutput, []byte{}, nil)
	err := apt.GetInstall("test-package")
	c.Assert(err, gc.IsNil)
	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-get", "--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
		"install", "test-package",
	})
	c.Assert(cmd.Env[len(cmd.Env)-1], gc.Equals, "DEBIAN_FRONTEND=noninteractive")
}

func (s *AptSuite) TestAptGetPreparePackages(c *gc.C) {
	packagesList := apt.GetPreparePackages([]string{"lxc", "bridge-utils", "git", "mongodb"}, "precise")
	c.Assert(packagesList[0], gc.DeepEquals, []string{"--target-release", "precise-updates/cloud-tools", "lxc", "mongodb"})
	c.Assert(packagesList[1], gc.DeepEquals, []string{"bridge-utils", "git"})
}

func (s *AptSuite) TestAptGetError(c *gc.C) {
	const expected = `E: frobnicator failure detected`
	cmdError := fmt.Errorf("error")
	cmdExpectedError := fmt.Errorf("apt-get failed: error")
	cmdChan := s.HookCommandOutput(&apt.CommandOutput, []byte(expected), cmdError)
	err := apt.GetInstall("foo")
	c.Assert(err, gc.DeepEquals, cmdExpectedError)
	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-get", "--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
		"install", "foo",
	})
}

func (s *AptSuite) TestConfigProxyEmpty(c *gc.C) {
	cmdChan := s.HookCommandOutput(&apt.CommandOutput, []byte{}, nil)
	out, err := apt.ConfigProxy()
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
	cmdChan := s.HookCommandOutput(&apt.CommandOutput, []byte(expected), nil)
	out, err := apt.ConfigProxy()
	c.Assert(err, gc.IsNil)
	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-config", "dump", "Acquire::http::Proxy",
		"Acquire::https::Proxy", "Acquire::ftp::Proxy",
	})
	c.Assert(out, gc.Equals, expected)
}

func (s *AptSuite) TestDetectAptProxy(c *gc.C) {
	const output = `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";
Acquire::ftp::Proxy "none";
Acquire::magic::Proxy "none";
`
	_ = s.HookCommandOutput(&apt.CommandOutput, []byte(output), nil)

	proxySettings, err := apt.DetectProxies()
	c.Assert(err, gc.IsNil)
	c.Assert(proxySettings, gc.DeepEquals, proxy.Settings{
		Http:  "10.0.3.1:3142",
		Https: "false",
		Ftp:   "none",
	})
}

func (s *AptSuite) TestDetectAptProxyNone(c *gc.C) {
	_ = s.HookCommandOutput(&apt.CommandOutput, []byte{}, nil)
	proxySettings, err := apt.DetectProxies()
	c.Assert(err, gc.IsNil)
	c.Assert(proxySettings, gc.DeepEquals, proxy.Settings{})
}

func (s *AptSuite) TestDetectAptProxyPartial(c *gc.C) {
	const output = `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::ftp::Proxy "here-it-is";
Acquire::magic::Proxy "none";
`
	_ = s.HookCommandOutput(&apt.CommandOutput, []byte(output), nil)

	proxySettings, err := apt.DetectProxies()
	c.Assert(err, gc.IsNil)
	c.Assert(proxySettings, gc.DeepEquals, proxy.Settings{
		Http: "10.0.3.1:3142",
		Ftp:  "here-it-is",
	})
}

func (s *AptSuite) TestAptProxyContentEmpty(c *gc.C) {
	output := apt.ProxyContent(proxy.Settings{})
	c.Assert(output, gc.Equals, "")
}

func (s *AptSuite) TestAptProxyContentPartial(c *gc.C) {
	proxySettings := proxy.Settings{
		Http: "user@10.0.0.1",
	}
	output := apt.ProxyContent(proxySettings)
	expected := `Acquire::http::Proxy "user@10.0.0.1";`
	c.Assert(output, gc.Equals, expected)
}

func (s *AptSuite) TestAptProxyContentRoundtrip(c *gc.C) {
	proxySettings := proxy.Settings{
		Http:  "http://user@10.0.0.1",
		Https: "https://user@10.0.0.1",
		Ftp:   "ftp://user@10.0.0.1",
	}
	output := apt.ProxyContent(proxySettings)

	s.HookCommandOutput(&apt.CommandOutput, []byte(output), nil)

	detected, err := apt.DetectProxies()
	c.Assert(err, gc.IsNil)
	c.Assert(detected, gc.DeepEquals, proxySettings)
}

func (s *AptSuite) TestConfigProxyConfiguredFilterOutput(c *gc.C) {
	const (
		output = `CommandLine::AsString "apt-config dump";
Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";`
		expected = `Acquire::http::Proxy  "10.0.3.1:3142";
Acquire::https::Proxy "false";`
	)
	cmdChan := s.HookCommandOutput(&apt.CommandOutput, []byte(output), nil)
	out, err := apt.ConfigProxy()
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
	cmdChan := s.HookCommandOutput(&apt.CommandOutput, []byte(expected), cmdError)
	out, err := apt.ConfigProxy()
	c.Assert(err, gc.DeepEquals, cmdExpectedError)
	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-config", "dump", "Acquire::http::Proxy",
		"Acquire::https::Proxy", "Acquire::ftp::Proxy",
	})
	c.Assert(out, gc.Equals, "")
}

func (s *AptSuite) patchDpkgQuery(c *gc.C, installed bool) {
	rc := 0
	if !installed {
		rc = 1
	}
	content := fmt.Sprintf("#!/bin/bash --norc\nexit %v", rc)
	patchExecutable(s, c.MkDir(), "dpkg-query", content)
}

func (s *AptSuite) TestIsPackageInstalled(c *gc.C) {
	s.patchDpkgQuery(c, true)
	c.Assert(apt.IsPackageInstalled("foo-bar"), jc.IsTrue)
}

func (s *AptSuite) TestIsPackageNotInstalled(c *gc.C) {
	s.patchDpkgQuery(c, false)
	c.Assert(apt.IsPackageInstalled("foo-bar"), jc.IsFalse)
}

type EnvironmentPatcher interface {
	PatchEnvironment(name, value string)
}

func patchExecutable(patcher EnvironmentPatcher, dir, execName, script string) {
	patcher.PatchEnvironment("PATH", dir)
	filename := filepath.Join(dir, execName)
	ioutil.WriteFile(filename, []byte(script), 0755)
}
