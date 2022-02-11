// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows
// +build !windows

package reboot

import (
	"os"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc/params"
	jujutesting "github.com/juju/juju/testing"
)

// on linux we use the "nohup" command to run a reboot
var rebootBin = "nohup"

var expectedRebootScript = `#!/bin/bash
sleep 15
shutdown -r now`

var expectedShutdownScript = `#!/bin/bash
sleep 15
shutdown -h now`

type NixRebootSuite struct {
	jujutesting.BaseSuite
	tmpDir           string
	rebootScriptName string
}

var _ = gc.Suite(&NixRebootSuite{})

func (s *NixRebootSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	testing.PatchExecutableAsEchoArgs(c, s, rebootBin)
	s.tmpDir = c.MkDir()
	s.rebootScriptName = "juju-reboot-script"
	s.PatchValue(&tmpFile, func() (*os.File, error) {
		script := s.rebootScript()
		return os.Create(script)
	})
}

func (s *NixRebootSuite) TestReboot(c *gc.C) {
	expectedParams := s.commandParams()
	err := scheduleAction(params.ShouldReboot, 15)
	c.Assert(err, jc.ErrorIsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedParams...)
	ft.File{s.rebootScriptName, expectedRebootScript, 0755}.Check(c, s.tmpDir)
}

func (s *NixRebootSuite) TestShutdownNoContainers(c *gc.C) {
	expectedParams := s.commandParams()

	err := scheduleAction(params.ShouldShutdown, 15)
	c.Assert(err, jc.ErrorIsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedParams...)
	ft.File{s.rebootScriptName, expectedShutdownScript, 0755}.Check(c, s.tmpDir)
}

func (s *NixRebootSuite) rebootScript() string {
	return filepath.Join(s.tmpDir, s.rebootScriptName)
}

func (s *NixRebootSuite) commandParams() []string {
	return []string{
		"sh",
		s.rebootScript(),
		"&",
	}
}
