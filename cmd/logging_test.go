// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/loggo"
	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/osenv"
	coretesting "launchpad.net/juju-core/testing"
)

var logger = loggo.GetLogger("juju.test")

type LogSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&LogSuite{})

func (s *LogSuite) SetUpTest(c *gc.C) {
	s.PatchEnvironment(osenv.JujuLoggingConfigEnvKey, "")
	s.AddCleanup(func(_ *gc.C) {
		loggo.ResetLoggers()
		loggo.ResetWriters()
	})
}

func newLogWithFlags(c *gc.C, flags []string) *cmd.Log {
	log := &cmd.Log{}
	flagSet := coretesting.NewFlagSet()
	log.AddFlags(flagSet)
	err := flagSet.Parse(false, flags)
	c.Assert(err, gc.IsNil)
	return log
}

func (s *LogSuite) TestNoFlags(c *gc.C) {
	log := newLogWithFlags(c, []string{})
	c.Assert(log.Path, gc.Equals, "")
	c.Assert(log.Verbose, gc.Equals, false)
	c.Assert(log.Debug, gc.Equals, false)
	c.Assert(log.Config, gc.Equals, "")
}

func (s *LogSuite) TestFlags(c *gc.C) {
	log := newLogWithFlags(c, []string{"--log-file", "foo", "--verbose", "--debug",
		"--logging-config=juju.cmd=INFO;juju.worker.deployer=DEBUG"})
	c.Assert(log.Path, gc.Equals, "foo")
	c.Assert(log.Verbose, gc.Equals, true)
	c.Assert(log.Debug, gc.Equals, true)
	c.Assert(log.Config, gc.Equals, "juju.cmd=INFO;juju.worker.deployer=DEBUG")
}

func (s *LogSuite) TestLogConfigFromEnvironment(c *gc.C) {
	config := "juju.cmd=INFO;juju.worker.deployer=DEBUG"
	s.PatchEnvironment(osenv.JujuLoggingConfigEnvKey, config)
	log := newLogWithFlags(c, []string{})
	c.Assert(log.Path, gc.Equals, "")
	c.Assert(log.Verbose, gc.Equals, false)
	c.Assert(log.Debug, gc.Equals, false)
	c.Assert(log.Config, gc.Equals, config)
}

func (s *LogSuite) TestVerboseSetsLogLevel(c *gc.C) {
	l := &cmd.Log{Verbose: true}
	ctx := coretesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)

	c.Assert(loggo.GetLogger("").LogLevel(), gc.Equals, loggo.INFO)
	c.Assert(coretesting.Stderr(ctx), gc.Equals, "")
	c.Assert(coretesting.Stdout(ctx), gc.Equals, "Flag --verbose is deprecated with the current meaning, use --show-log\n")
}

func (s *LogSuite) TestDebugSetsLogLevel(c *gc.C) {
	l := &cmd.Log{Debug: true}
	ctx := coretesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)

	c.Assert(loggo.GetLogger("").LogLevel(), gc.Equals, loggo.DEBUG)
	c.Assert(coretesting.Stderr(ctx), gc.Equals, "")
	c.Assert(coretesting.Stdout(ctx), gc.Equals, "")
}

func (s *LogSuite) TestShowLogSetsLogLevel(c *gc.C) {
	l := &cmd.Log{ShowLog: true}
	ctx := coretesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)

	c.Assert(loggo.GetLogger("").LogLevel(), gc.Equals, loggo.INFO)
	c.Assert(coretesting.Stderr(ctx), gc.Equals, "")
	c.Assert(coretesting.Stdout(ctx), gc.Equals, "")
}

func (s *LogSuite) TestStderr(c *gc.C) {
	l := &cmd.Log{Verbose: true, Config: "<root>=INFO"}
	ctx := coretesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)
	logger.Infof("hello")
	c.Assert(coretesting.Stderr(ctx), gc.Matches, `^.* INFO .* hello\n`)
}

func (s *LogSuite) TestRelPathLog(c *gc.C) {
	l := &cmd.Log{Path: "foo.log", Config: "<root>=INFO"}
	ctx := coretesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)
	logger.Infof("hello")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "foo.log"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Matches, `^.* INFO .* hello\n`)
	c.Assert(coretesting.Stderr(ctx), gc.Equals, "")
	c.Assert(coretesting.Stdout(ctx), gc.Equals, "")
}

func (s *LogSuite) TestAbsPathLog(c *gc.C) {
	path := filepath.Join(c.MkDir(), "foo.log")
	l := &cmd.Log{Path: path, Config: "<root>=INFO"}
	ctx := coretesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)
	logger.Infof("hello")
	c.Assert(coretesting.Stderr(ctx), gc.Equals, "")
	content, err := ioutil.ReadFile(path)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Matches, `^.* INFO .* hello\n`)
}

func (s *LogSuite) TestLoggingToFileAndStderr(c *gc.C) {
	l := &cmd.Log{Path: "foo.log", Config: "<root>=INFO", ShowLog: true}
	ctx := coretesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)
	logger.Infof("hello")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "foo.log"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Matches, `^.* INFO .* hello\n`)
	c.Assert(coretesting.Stderr(ctx), gc.Matches, `^.* INFO .* hello\n`)
	c.Assert(coretesting.Stdout(ctx), gc.Equals, "")
}

func (s *LogSuite) TestErrorAndWarningLoggingToStderr(c *gc.C) {
	// Error and warning go to stderr even with ShowLog=false
	l := &cmd.Log{Config: "<root>=INFO", ShowLog: false}
	ctx := coretesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)
	logger.Warningf("a warning")
	logger.Errorf("an error")
	logger.Infof("an info")
	c.Assert(coretesting.Stderr(ctx), gc.Matches, `^.*WARNING a warning\n.*ERROR an error\n.*`)
	c.Assert(coretesting.Stdout(ctx), gc.Equals, "")
}
