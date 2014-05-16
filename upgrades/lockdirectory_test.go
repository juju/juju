// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/upgrades"
)

type ensureLockDirSuite struct {
	testing.FakeJujuHomeSuite
	bin     string
	home    string
	datadir string
	lockdir string
	ctx     upgrades.Context
}

var _ = gc.Suite(&ensureLockDirSuite{})

// fakecommand outputs its arguments to stdout for verification
var fakecommand = `#!/bin/bash

echo $@ | tee $0.args
`

func (s *ensureLockDirSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)

	s.bin = c.MkDir()
	s.PatchEnvPathPrepend(s.bin)

	err := ioutil.WriteFile(
		filepath.Join(s.bin, "chown"),
		[]byte(fakecommand), 0777)
	c.Assert(err, gc.IsNil)

	loggo.GetLogger("juju.upgrade").SetLogLevel(loggo.TRACE)

	s.home = c.MkDir()
	s.PatchValue(upgrades.UbuntuHome, s.home)

	s.datadir = c.MkDir()
	s.lockdir = filepath.Join(s.datadir, "locks")
	s.ctx = &mockContext{agentConfig: &mockAgentConfig{dataDir: s.datadir}}
}

func (s *ensureLockDirSuite) assertChownCalled(c *gc.C) {
	bytes, err := ioutil.ReadFile(filepath.Join(s.bin, "chown.args"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(bytes), gc.Equals, fmt.Sprintf("ubuntu:ubuntu %s\n", s.lockdir))
}

func (s *ensureLockDirSuite) assertNoChownCalled(c *gc.C) {
	c.Assert(filepath.Join(s.bin, "chown.args"), jc.DoesNotExist)
}

func (s *ensureLockDirSuite) TestLockDirCreated(c *gc.C) {
	err := upgrades.EnsureLockDirExistsAndUbuntuWritable(s.ctx)
	c.Assert(err, gc.IsNil)

	c.Assert(s.lockdir, jc.IsDirectory)
	s.assertChownCalled(c)
}

func (s *ensureLockDirSuite) TestIdempotent(c *gc.C) {
	err := upgrades.EnsureLockDirExistsAndUbuntuWritable(s.ctx)
	c.Assert(err, gc.IsNil)

	err = upgrades.EnsureLockDirExistsAndUbuntuWritable(s.ctx)
	c.Assert(err, gc.IsNil)

	c.Assert(s.lockdir, jc.IsDirectory)
	s.assertChownCalled(c)
}

func (s *ensureLockDirSuite) TestNoChownIfNoHome(c *gc.C) {
	s.PatchValue(upgrades.UbuntuHome, filepath.Join(s.home, "not-exist"))
	err := upgrades.EnsureLockDirExistsAndUbuntuWritable(s.ctx)
	c.Assert(err, gc.IsNil)

	c.Assert(s.lockdir, jc.IsDirectory)
	s.assertNoChownCalled(c)
}
