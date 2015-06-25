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

var _ = gc.Suite(&steps124Suite{})
var _ = gc.Suite(&steps124SyslogSuite{})

type steps124Suite struct {
	testing.BaseSuite
}

func (s *steps124Suite) TestStateStepsFor124(c *gc.C) {
	expected := []string{
		"add block device documents for existing machines",
		"add instance id field to IP addresses",
	}
	assertStateSteps(c, version.MustParse("1.24.0"), expected)
}

func (s *steps124Suite) TestStepsFor124(c *gc.C) {
	expected := []string{
		"move syslog config from LogDir to ConfDir",
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

type steps124SyslogSuite struct {
	testing.BaseSuite
	data    string
	logDir  string
	confDir string
	ctx     fakeContext
}

func (s *steps124SyslogSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.data = "data!"

	logDir := c.MkDir()
	confDir := c.MkDir()
	s.setPaths(logDir, confDir)
}

func (s *steps124SyslogSuite) setPaths(logDir, confDir string) {
	s.logDir = logDir
	s.confDir = confDir
	s.ctx = fakeContext{
		cfg: fakeConfig{
			logdir: logDir,
		},
	}

	s.PatchValue(&agent.DefaultConfDir, confDir)
}

func (s *steps124SyslogSuite) oldFile(filename string) testFile {
	return newTestFile(s.logDir, filename, s.data)
}

func (s *steps124SyslogSuite) newFile(filename string) testFile {
	return newTestFile(s.confDir, filename, s.data)
}

func (s *steps124SyslogSuite) writeOldFiles(c *gc.C, files []string) {
	for _, f := range files {
		s.oldFile(f).write(c)
	}
}

func (s *steps124SyslogSuite) checkFiles(c *gc.C, files []string) {
	for _, f := range files {
		s.oldFile(f).checkMissing(c)
		s.newFile(f).checkExists(c)
	}
}

func (s *steps124SyslogSuite) TestMoveSyslogConfigDefault(c *gc.C) {
	files := []string{
		"ca-cert.pem",
		"rsyslog-cert.pem",
		"rsyslog-key.pem",
		"logrotate.conf",
		"logrotate.run",
	}
	s.writeOldFiles(c, files)

	err := upgrades.MoveSyslogConfig(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.checkFiles(c, files)
}

func (s *steps124SyslogSuite) TestMoveSyslogConfigConflicts(c *gc.C) {
	files := []string{
		"logrotate.conf",
		"logrotate.run",
	}

	// ensure that we don't overwrite an existing file in datadir, and don't
	// error out if one of the files exists in datadir but not logdir.
	s.oldFile("logrotate.conf").write(c)
	s.newFile("logrotate.run").write(c)

	tf := s.newFile("logrotate.conf")
	tf.data = "different"
	tf.write(c)

	err := upgrades.MoveSyslogConfig(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.checkFiles(c, files)

	tf.data = s.data
	tf.checkFile(c)
}

func (s *steps124SyslogSuite) TestMoveSyslogConfigCantDeleteOld(c *gc.C) {
	// ensure that we don't error out if we can't remove the old file.
	// error out if one of the files exists in datadir but not logdir.
	s.PatchValue(upgrades.OsRemove, func(string) error { return os.ErrPermission })

	file := s.oldFile("logrotate.conf")
	file.write(c)

	err := upgrades.MoveSyslogConfig(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	// should still exist in both places (i.e. check we didn't screw up the test)
	file.checkExists(c)
	s.newFile("logrotate.conf").checkExists(c)
}

func (s *steps124SyslogSuite) TestMoveSyslogConfigTargetDirMissing(c *gc.C) {
	err := os.Remove(s.confDir)
	c.Assert(err, jc.ErrorIsNil)
	files := []string{
		"ca-cert.pem",
		"rsyslog-cert.pem",
		"rsyslog-key.pem",
		"logrotate.conf",
		"logrotate.run",
	}
	s.writeOldFiles(c, files)

	err = upgrades.MoveSyslogConfig(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.checkFiles(c, files)
}

type testFile struct {
	path     string
	dirname  string
	filename string
	mode     os.FileMode
	data     string
}

func newTestFile(dirname, filename, data string) testFile {
	path := filepath.Join(dirname, filename)
	return testFile{
		path:     path,
		dirname:  dirname,
		filename: filename,
		mode:     0644,
		data:     data,
	}
}

func (tf testFile) write(c *gc.C) {
	err := ioutil.WriteFile(tf.path, []byte(tf.data), tf.mode)
	c.Assert(err, jc.ErrorIsNil)
}

func (tf testFile) checkExists(c *gc.C) {
	_, err := os.Stat(tf.path)
	c.Check(err, jc.ErrorIsNil)
}

func (tf testFile) checkMissing(c *gc.C) {
	_, err := os.Stat(tf.path)
	c.Check(err, jc.Satisfies, os.IsNotExist)
}

func (tf testFile) checkFile(c *gc.C) {
	data, err := ioutil.ReadFile(tf.path)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Not(gc.Equals), tf.data)
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

func (f fakeConfig) Value(key string) string {
	return ""
}

func (f fakeConfig) LogDir() string {
	return f.logdir
}

func (f fakeConfig) DataDir() string {
	return f.datadir
}
