// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/version"
)

type steps124Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps124Suite{})

func (s *steps124Suite) TestStateStepsFor124(c *gc.C) {
	expected := []string{
		"add block device documents for existing machines",
		"move service.UnitSeq to sequence collection",
		"add instance id field to IP addresses",
		"add UUID field to IP addresses",
		"migrate charm archives into environment storage",
		"change entityid field on status history to globalkey",
		"change updated field on statushistory from time to int",
		"change updated field on status from time to int",
	}
	assertStateSteps(c, version.MustParse("1.24.0"), expected)
}

func (s *steps124Suite) TestStateStepsFor1244(c *gc.C) {
	expected := []string{
		"add missing service statuses",
	}
	assertStateSteps(c, version.MustParse("1.24.4"), expected)
}

func (s *steps124Suite) TestStepsFor124(c *gc.C) {
	expected := []string{
		"move syslog config from LogDir to DataDir",
	}
	assertSteps(c, version.MustParse("1.24.0"), expected)
}

func (s *steps124Suite) TestCopyFileNew(c *gc.C) {
	src := c.MkDir()
	dest := c.MkDir()
	srcdata := []byte("new data!")

	// test that a file in src dir and not in dest dir gets copied.

	newSrc := filepath.Join(src, "new")
	err := ioutil.WriteFile(newSrc, srcdata, 0644)
	c.Assert(err, jc.ErrorIsNil)

	newDest := filepath.Join(dest, "new")

	err = upgrades.CopyFile(newDest, newSrc)
	c.Assert(err, jc.ErrorIsNil)

	srcb, err := ioutil.ReadFile(newSrc)
	c.Assert(err, jc.ErrorIsNil)
	destb, err := ioutil.ReadFile(newDest)
	c.Assert(err, jc.ErrorIsNil)
	// convert to string and use Equals because we'll get a better failure message
	c.Assert(string(destb), gc.Equals, string(srcb))
}

func (s *steps124Suite) TestCopyFileExisting(c *gc.C) {
	src := c.MkDir()
	dest := c.MkDir()
	srcdata := []byte("new data!")
	destdata := []byte("old data!")

	exSrc := filepath.Join(src, "existing")
	exDest := filepath.Join(dest, "existing")

	err := ioutil.WriteFile(exSrc, srcdata, 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(exDest, destdata, 0644)
	c.Assert(err, jc.ErrorIsNil)

	err = upgrades.CopyFile(exDest, exSrc)
	c.Assert(err, jc.ErrorIsNil)

	// assert we haven't changed the destination
	b, err := ioutil.ReadFile(exDest)

	c.Assert(err, jc.ErrorIsNil)
	// convert to string because we'll get a better failure message
	c.Assert(string(b), gc.Equals, string(destdata))
}

func (s *steps124Suite) TestMoveSyslogConfigDefault(c *gc.C) {
	logdir := c.MkDir()
	datadir := c.MkDir()
	data := []byte("data!")
	files := []string{
		"ca-cert.pem",
		"rsyslog-cert.pem",
		"rsyslog-key.pem",
		"logrotate.conf",
		"logrotate.run",
	}
	for _, f := range files {
		err := ioutil.WriteFile(filepath.Join(logdir, f), data, 0644)
		c.Assert(err, jc.ErrorIsNil)
	}

	ctx := fakeContext{cfg: fakeConfig{logdir: logdir, datadir: datadir}}
	err := upgrades.MoveSyslogConfig(ctx)
	c.Assert(err, jc.ErrorIsNil)

	for _, f := range files {
		_, err := os.Stat(filepath.Join(datadir, f))
		c.Assert(err, jc.ErrorIsNil)
		_, err = os.Stat(filepath.Join(logdir, f))
		c.Assert(err, jc.Satisfies, os.IsNotExist)
	}
}

func (s *steps124Suite) TestMoveSyslogConfig(c *gc.C) {
	logdir := c.MkDir()
	datadir := c.MkDir()
	data := []byte("data!")
	files := []string{
		"logrotate.conf",
		"logrotate.run",
	}

	// ensure that we don't overwrite an existing file in datadir, and don't
	// error out if one of the files exists in datadir but not logdir.

	err := ioutil.WriteFile(filepath.Join(logdir, "logrotate.conf"), data, 0644)
	c.Assert(err, jc.ErrorIsNil)

	err = ioutil.WriteFile(filepath.Join(datadir, "logrotate.run"), data, 0644)
	c.Assert(err, jc.ErrorIsNil)

	differentData := []byte("different")
	existing := filepath.Join(datadir, "logrotate.conf")
	err = ioutil.WriteFile(existing, differentData, 0644)
	c.Assert(err, jc.ErrorIsNil)

	ctx := fakeContext{cfg: fakeConfig{logdir: logdir, datadir: datadir}}
	err = upgrades.MoveSyslogConfig(ctx)
	c.Assert(err, jc.ErrorIsNil)

	for _, f := range files {
		_, err := os.Stat(filepath.Join(datadir, f))
		c.Assert(err, jc.ErrorIsNil)
		_, err = os.Stat(filepath.Join(logdir, f))
		c.Assert(err, jc.Satisfies, os.IsNotExist)
	}

	b, err := ioutil.ReadFile(existing)
	c.Assert(err, jc.ErrorIsNil)
	// convert to string because we'll get a better failure message
	c.Assert(string(b), gc.Not(gc.Equals), string(existing))

}

func (s *steps124Suite) TestMoveSyslogConfigCantDeleteOld(c *gc.C) {
	logdir := c.MkDir()
	datadir := c.MkDir()
	file := filepath.Join(logdir, "logrotate.conf")

	// ensure that we don't error out if we can't remove the old file.
	// error out if one of the files exists in datadir but not logdir.
	s.PatchValue(upgrades.OsRemove, func(string) error { return os.ErrPermission })

	err := ioutil.WriteFile(file, []byte("data!"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	ctx := fakeContext{cfg: fakeConfig{logdir: logdir, datadir: datadir}}
	err = upgrades.MoveSyslogConfig(ctx)
	c.Assert(err, jc.ErrorIsNil)

	// should still exist in both places (i.e. check we didn't screw up the test)
	_, err = os.Stat(file)
	c.Assert(err, jc.ErrorIsNil)
	_, err = os.Stat(filepath.Join(datadir, "logrotate.conf"))
	c.Assert(err, jc.ErrorIsNil)
}

type fakeContext struct {
	upgrades.Context
	cfg fakeConfig
}

func (f fakeContext) AgentConfig() agent.ConfigSetter {
	return f.cfg
}

type fakeConfig struct {
	agent.ConfigSetter
	logdir  string
	datadir string
}

func (f fakeConfig) LogDir() string {
	return f.logdir
}

func (f fakeConfig) DataDir() string {
	return f.datadir
}
