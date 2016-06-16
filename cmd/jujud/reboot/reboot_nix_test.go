// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package reboot_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/reboot"
)

// on linux we use the "at" command to schedule a reboot
var rebootBin = "at"

var expectedRebootScript = `#!/bin/bash
sleep 15
shutdown -r now`

var expectedShutdownScript = `#!/bin/bash
sleep 15
shutdown -h now`

func (s *RebootSuite) rebootCommandParams(c *gc.C) []string {
	return []string{
		"-f",
		s.rebootScript(c),
		"now",
	}
}

func (s *RebootSuite) shutdownCommandParams(c *gc.C) []string {
	return []string{
		"-f",
		s.rebootScript(c),
		"now",
	}
}

func (s *RebootSuite) TestRebootNoContainers(c *gc.C) {
	w, err := reboot.NewRebootWaiter(s.st, s.acfg)
	c.Assert(err, jc.ErrorIsNil)
	expectedRebootParams := s.rebootCommandParams(c)

	err = w.ExecuteReboot(params.ShouldReboot)
	c.Assert(err, jc.ErrorIsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedRebootParams...)
	ft.File{s.rebootScriptName, expectedRebootScript, 0755}.Check(c, s.tmpDir)
}

func (s *RebootSuite) TestShutdownNoContainers(c *gc.C) {
	w, err := reboot.NewRebootWaiter(s.st, s.acfg)
	c.Assert(err, jc.ErrorIsNil)
	expectedShutdownParams := s.shutdownCommandParams(c)

	err = w.ExecuteReboot(params.ShouldShutdown)
	c.Assert(err, jc.ErrorIsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedShutdownParams...)
	ft.File{s.rebootScriptName, expectedShutdownScript, 0755}.Check(c, s.tmpDir)
}
