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
		"add instance id field to IP addresses",
	}
	assertStateSteps(c, version.MustParse("1.24.0"), expected)
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
	c.Assert(err, gc.IsNil)

	newDest := filepath.Join(dest, "new")

	err = upgrades.CopyFile(newDest, newSrc)
	c.Assert(err, gc.IsNil)

	srcb, err := ioutil.ReadFile(newSrc)
	c.Assert(err, gc.IsNil)
	destb, err := ioutil.ReadFile(newDest)
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(exDest, destdata, 0644)
	c.Assert(err, gc.IsNil)

	err = upgrades.CopyFile(exDest, exSrc)
	c.Assert(err, gc.IsNil)

	// assert we haven't changed the destination
	b, err := ioutil.ReadFile(exDest)

	c.Assert(err, gc.IsNil)
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
		c.Assert(err, gc.IsNil)
	}

	ctx := fakeContext{cfg: fakeConfig{logdir: logdir, datadir: datadir}}
	err := upgrades.MoveSyslogConfig(ctx)
	c.Assert(err, gc.IsNil)

	for _, f := range files {
		_, err := os.Stat(filepath.Join(datadir, f))
		c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)

	err = ioutil.WriteFile(filepath.Join(datadir, "logrotate.run"), data, 0644)
	c.Assert(err, gc.IsNil)

	differentData := []byte("different")
	existing := filepath.Join(datadir, "logrotate.conf")
	err = ioutil.WriteFile(existing, differentData, 0644)
	c.Assert(err, gc.IsNil)

	ctx := fakeContext{cfg: fakeConfig{logdir: logdir, datadir: datadir}}
	err = upgrades.MoveSyslogConfig(ctx)
	c.Assert(err, gc.IsNil)

	for _, f := range files {
		_, err := os.Stat(filepath.Join(datadir, f))
		c.Assert(err, gc.IsNil)
		_, err = os.Stat(filepath.Join(logdir, f))
		c.Assert(err, jc.Satisfies, os.IsNotExist)
	}

	b, err := ioutil.ReadFile(existing)
	c.Assert(err, gc.IsNil)
	// convert to string because we'll get a better failure message
	c.Assert(string(b), gc.Not(gc.Equals), string(existing))

}

func (s *steps124Suite) TestMoveSyslogConfigCantDeleteOld(c *gc.C) {
	logdir := c.MkDir()
	datadir := c.MkDir()
	data := []byte("data!")
	file := filepath.Join(logdir, "logrotate.conf")

	// ensure that we don't error out if we can't remove the old file.
	// error out if one of the files exists in datadir but not logdir.
	*upgrades.OsRemove = func(string) error { return os.ErrPermission }
	defer func() { *upgrades.OsRemove = os.Remove }()

	err := ioutil.WriteFile(file, data, 0644)
	c.Assert(err, gc.IsNil)

	ctx := fakeContext{cfg: fakeConfig{logdir: logdir, datadir: datadir}}
	err = upgrades.MoveSyslogConfig(ctx)
	c.Assert(err, gc.IsNil)

	// should still exist in both places (i.e. check we didn't screw up the test)
	_, err = os.Stat(file)
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(filepath.Join(logdir, "logrotate.conf"))
	c.Assert(err, gc.IsNil)
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
