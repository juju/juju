// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/upgrades"
)

var v276 = version.MustParse("2.7.6")

type steps276Suite struct {
	testing.IsolationSuite

	dir string
}

var _ = gc.Suite(&steps276Suite{})

func (s *steps276Suite) SetUpTest(c *gc.C) {
	s.dir = c.MkDir()
	files, err := filepath.Glob("testdata/uniter-state-*")
	c.Assert(err, jc.ErrorIsNil)
	copyCommand := exec.Command("/bin/cp", append(files, s.dir)...)
	err = copyCommand.Run()
	c.Assert(err, jc.ErrorIsNil)

	err = loggo.ConfigureLoggers("<root>=TRACE")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *steps276Suite) TestStepRegistered(c *gc.C) {
	step := findStep(c, v276, "add remote-application key to hooks in uniter state files")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.HostMachine})
}

func (s *steps276Suite) readStateFile(c *gc.C, dir, file string) map[string]interface{} {
	var result map[string]interface{}
	err := utils.ReadYaml(filepath.Join(dir, fmt.Sprintf("uniter-state-%s.yaml", file)), &result)
	c.Assert(err, jc.ErrorIsNil)
	return result
}

func (s *steps276Suite) TestAddRemoteApplicationToHookSuccess(c *gc.C) {
	err := upgrades.AddRemoteApplicationToHook(filepath.Join(s.dir, "uniter-state-no-app.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	// The no-app file should now be identical to the app file.
	c.Assert(s.readStateFile(c, s.dir, "no-app"), gc.DeepEquals, s.readStateFile(c, s.dir, "app"))
}

func (s *steps276Suite) TestAddRemoteApplicationToHookNoHook(c *gc.C) {
	err := upgrades.AddRemoteApplicationToHook(filepath.Join(s.dir, "uniter-state-no-hook.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	// The no-hook file hasn't changed from the original.
	c.Assert(s.readStateFile(c, s.dir, "no-hook"), gc.DeepEquals, s.readStateFile(c, "testdata", "no-hook"))
}

func (s *steps276Suite) TestAddRemoteApplicationToHookRemoteApplicationSet(c *gc.C) {
	err := upgrades.AddRemoteApplicationToHook(filepath.Join(s.dir, "uniter-state-app.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	// The app file hasn't changed from the original.
	c.Assert(s.readStateFile(c, s.dir, "app"), gc.DeepEquals, s.readStateFile(c, "testdata", "app"))
}

func (s *steps276Suite) TestAddRemoteApplicationToRunningHooks(c *gc.C) {
	err := upgrades.AddRemoteApplicationToRunningHooks(filepath.Join(s.dir, "uniter-state-*.yaml"))(nil)
	c.Assert(err, jc.ErrorIsNil)
	// The no-app file's been updated but the others have been left alone.
	c.Assert(s.readStateFile(c, s.dir, "no-app"), gc.DeepEquals, s.readStateFile(c, s.dir, "app"))
	c.Assert(s.readStateFile(c, s.dir, "no-hook"), gc.DeepEquals, s.readStateFile(c, "testdata", "no-hook"))
	c.Assert(s.readStateFile(c, s.dir, "app"), gc.DeepEquals, s.readStateFile(c, "testdata", "app"))
}
