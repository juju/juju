// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/version"
)

// Stub stubs out the external functions used in the service package.
type Stub struct {
	*testing.Stub

	Version      version.Binary
	GOOS         string
	PID1Filename string
	Executable   string
	Service      Service
}

// GetVersion stubs out .
func (s *Stub) GetVersion() version.Binary {
	s.AddCall("GetVersion")

	// Pop the next error off the queue, even though we don't use it.
	s.NextErr()
	return s.Version
}

// GetOS stubs out runtime.GOOS.
func (s *Stub) GetOS() string {
	s.AddCall("GetOS")

	// Pop the next error off the queue, even though we don't use it.
	s.NextErr()
	return s.GOOS
}

// GetPID1Filename stubs out /proc/1/cmdline.
func (s *Stub) GetPID1Filename() string {
	s.AddCall("GetPID1Filename")

	// Pop the next error off the queue, even though we don't use it.
	s.NextErr()
	return s.PID1Filename
}

// GetInitSystemExecutable stubs out the contents of /proc/1/cmdline.
func (s *Stub) GetInitSystemExecutable() (string, error) {
	s.AddCall("GetInitSystemExecutable")

	return s.Executable, s.NextErr()
}

// DiscoverService stubs out service.DiscoverService.
func (s *Stub) DiscoverService(name string) (Service, error) {
	s.AddCall("DiscoverService", name)

	return s.Service, s.NextErr()
}

// BaseSuite is the base test suite for the service package.
type BaseSuite struct {
	testing.IsolationSuite

	Dirname string
	Name    string
	Conf    common.Conf
	Failure error

	Stub    *testing.Stub
	Service *FakeService
	Patched *Stub
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.Dirname = c.MkDir()
	s.Name = "juju-agent-machine-0"
	s.Conf = common.Conf{
		Desc:      "some service",
		ExecStart: "/bin/jujud machine 0",
	}
	s.Failure = errors.New("<failed>")

	s.Stub = &testing.Stub{}
	s.Service = NewFakeService(s.Name, s.Conf)
	s.Service.Stub = s.Stub
	s.Patched = &Stub{Stub: s.Stub}
	s.PatchValue(&discoverService, s.Patched.DiscoverService)
}

func (s *BaseSuite) PatchAttempts(retries int) {
	s.PatchValue(&installStartRetryAttempts, utils.AttemptStrategy{
		Min: retries,
	})
}

func (s *BaseSuite) PatchVersion(vers version.Binary) {
	s.Patched.Version = vers
	s.PatchValue(&getVersion, s.Patched.GetVersion)
}

func (s *BaseSuite) PatchGOOS(os string) {
	s.Patched.GOOS = os
	s.PatchValue(&runtimeOS, s.Patched.GetOS)
}

func (s *BaseSuite) PatchInitSystemExecutable(executable string) {
	s.Patched.Executable = executable
	s.PatchValue(&initExecutable, s.Patched.GetInitSystemExecutable)
}

func (s *BaseSuite) PatchPid1File(c *gc.C, executable, verText string) string {
	exeName := s.resolveExecutable(executable)
	if verText != "" {
		s.writeExecutable(c, exeName, verText)
	}

	// Now write the actual fake /proc/1/cmdline file.
	filename := filepath.Join(s.Dirname, "pid1cmdline")
	err := ioutil.WriteFile(filename, []byte(exeName), 0644)
	c.Assert(err, jc.ErrorIsNil)

	s.Patched.PID1Filename = filename
	s.PatchValue(&pid1Filename, s.Patched.GetPID1Filename)
	return exeName
}

func (s *BaseSuite) resolveExecutable(executable string) string {
	exeSuffix := ".sh"
	if runtime.GOOS == "windows" {
		executable = filepath.FromSlash(executable)
		exeSuffix = ".bat"
	}
	return filepath.Join(s.Dirname, executable) + exeSuffix
}

func (s *BaseSuite) writeExecutable(c *gc.C, exeName, verText string) {
	err := os.MkdirAll(filepath.Dir(exeName), 0755)
	c.Assert(err, jc.ErrorIsNil)

	script := []byte(`
#!/usr/bin/env bash
echo ` + verText)
	err = ioutil.WriteFile(exeName, script[1:], 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BaseSuite) CheckFailure(c *gc.C, err error) {
	c.Check(errors.Cause(err), gc.Equals, s.Failure)
}
