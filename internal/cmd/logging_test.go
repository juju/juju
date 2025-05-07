// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd_test

import (
	"context"
	"io/ioutil"
	"path/filepath"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.test")

type LogSuite struct {
	testing.LoggingCleanupSuite
}

var _ = tc.Suite(&LogSuite{})

func newLogWithFlags(c *tc.C, defaultConfig string, flags ...string) *cmd.Log {
	log := &cmd.Log{
		DefaultConfig: defaultConfig,
	}
	flagSet := cmdtesting.NewFlagSet()
	log.AddFlags(flagSet)
	err := flagSet.Parse(false, flags)
	c.Assert(err, tc.IsNil)
	return log
}

func (s *LogSuite) TestNoFlags(c *tc.C) {
	log := newLogWithFlags(c, "")
	c.Assert(log.Path, tc.Equals, "")
	c.Assert(log.Quiet, tc.Equals, false)
	c.Assert(log.Verbose, tc.Equals, false)
	c.Assert(log.Debug, tc.Equals, false)
	c.Assert(log.Config, tc.Equals, "")
}

func (s *LogSuite) TestFlags(c *tc.C) {
	log := newLogWithFlags(c, "", "--log-file", "foo", "--verbose", "--debug", "--show-log",
		"--logging-config=juju.cmd=INFO;juju.worker.deployer=DEBUG")
	c.Assert(log.Path, tc.Equals, "foo")
	c.Assert(log.Verbose, tc.Equals, true)
	c.Assert(log.Debug, tc.Equals, true)
	c.Assert(log.ShowLog, tc.Equals, true)
	c.Assert(log.Config, tc.Equals, "juju.cmd=INFO;juju.worker.deployer=DEBUG")
}

func (s *LogSuite) TestLogConfigFromDefault(c *tc.C) {
	config := "juju.cmd=INFO;juju.worker.deployer=DEBUG"
	log := newLogWithFlags(c, config)
	log.DefaultConfig = config
	c.Assert(log.Path, tc.Equals, "")
	c.Assert(log.Verbose, tc.Equals, false)
	c.Assert(log.Debug, tc.Equals, false)
	c.Assert(log.Config, tc.Equals, config)
}

func (s *LogSuite) TestDebugSetsLogLevel(c *tc.C) {
	l := &cmd.Log{Debug: true}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)

	c.Assert(loggo.GetLogger("").LogLevel(), tc.Equals, loggo.DEBUG)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *LogSuite) TestShowLogSetsLogLevel(c *tc.C) {
	l := &cmd.Log{ShowLog: true}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)

	c.Assert(loggo.GetLogger("").LogLevel(), tc.Equals, loggo.INFO)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *LogSuite) TestStderr(c *tc.C) {
	l := &cmd.Log{ShowLog: true, Config: "<root>=INFO"}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)
	logger.Infof(context.TODO(), "hello")
	c.Assert(cmdtesting.Stderr(ctx), tc.Matches, `^.* INFO .* hello\n`)
}

func (s *LogSuite) TestRelPathLog(c *tc.C) {
	l := &cmd.Log{Path: "foo.log", Config: "<root>=INFO"}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)
	logger.Infof(context.TODO(), "hello")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "foo.log"))
	c.Assert(err, tc.IsNil)
	c.Assert(string(content), tc.Matches, `^.* INFO .* hello\n`)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *LogSuite) TestAbsPathLog(c *tc.C) {
	path := filepath.Join(c.MkDir(), "foo.log")
	l := &cmd.Log{Path: path, Config: "<root>=INFO"}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)
	logger.Infof(context.TODO(), "hello")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	content, err := ioutil.ReadFile(path)
	c.Assert(err, tc.IsNil)
	c.Assert(string(content), tc.Matches, `^.* INFO .* hello\n`)
}

func (s *LogSuite) TestLoggingToFileAndStderr(c *tc.C) {
	l := &cmd.Log{Path: "foo.log", Config: "<root>=INFO", ShowLog: true}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)
	logger.Infof(context.TODO(), "hello")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "foo.log"))
	c.Assert(err, tc.IsNil)
	c.Assert(string(content), tc.Matches, `^.* INFO .* hello\n`)
	c.Assert(cmdtesting.Stderr(ctx), tc.Matches, `^.* INFO .* hello\n`)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *LogSuite) TestErrorAndWarningLoggingToStderr(c *tc.C) {
	// Error and warning go to stderr even with ShowLog=false
	l := &cmd.Log{Config: "<root>=INFO", ShowLog: false}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)
	logger.Warningf(context.TODO(), "a warning")
	logger.Errorf(context.TODO(), "an error")
	logger.Infof(context.TODO(), "an info")
	c.Assert(cmdtesting.Stderr(ctx), tc.Matches, `^.*WARNING a warning\n.*ERROR an error\n.*`)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *LogSuite) TestQuietAndVerbose(c *tc.C) {
	l := &cmd.Log{Verbose: true, Quiet: true}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.ErrorMatches, `"verbose" and "quiet" flags clash, please use one or the other, not both`)
}

func (s *LogSuite) TestOutputDefault(c *tc.C) {
	l := &cmd.Log{}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)

	ctx.Infof("Writing info output")
	ctx.Verbosef("Writing verbose output")

	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Writing info output\n")
}

func (s *LogSuite) TestOutputVerbose(c *tc.C) {
	l := &cmd.Log{Verbose: true}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)

	ctx.Infof("Writing info output")
	ctx.Verbosef("Writing verbose output")

	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Writing info output\nWriting verbose output\n")
}

func (s *LogSuite) TestOutputQuiet(c *tc.C) {
	l := &cmd.Log{Quiet: true}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)

	ctx.Infof("Writing info output")
	ctx.Verbosef("Writing verbose output")

	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
}

func (s *LogSuite) TestOutputQuietLogs(c *tc.C) {
	l := &cmd.Log{Quiet: true, Path: "foo.log", Config: "<root>=INFO"}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)

	ctx.Infof("Writing info output")
	ctx.Verbosef("Writing verbose output")

	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "foo.log"))
	c.Assert(err, tc.IsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(string(content), tc.Matches, `^.*INFO .* Writing info output\n.*INFO .*Writing verbose output\n.*`)
}

func (s *LogSuite) TestOutputDefaultLogsVerbose(c *tc.C) {
	l := &cmd.Log{Path: "foo.log", Config: "<root>=INFO"}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)

	ctx.Infof("Writing info output")
	ctx.Verbosef("Writing verbose output")

	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "foo.log"))
	c.Assert(err, tc.IsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "Writing info output\n")
	c.Assert(string(content), tc.Matches, `^.*INFO .*Writing verbose output\n.*`)
}

func (s *LogSuite) TestOutputDebugForcesQuiet(c *tc.C) {
	l := &cmd.Log{Verbose: true, Debug: true}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)

	ctx.Infof("Writing info output")
	ctx.Verbosef("Writing verbose output")

	c.Assert(cmdtesting.Stderr(ctx), tc.Matches, `^.*INFO .* Writing info output\n.*INFO .*Writing verbose output\n.*`)
}

func (s *LogSuite) TestOutputWarning(c *tc.C) {
	l := &cmd.Log{Verbose: true, Debug: true}
	ctx := cmdtesting.Context(c)
	err := l.Start(ctx)
	c.Assert(err, tc.IsNil)

	ctx.Warningf("Writing warning output")

	c.Assert(cmdtesting.Stderr(ctx), tc.Matches, `^.* WARN .* Writing warning output\n.*`)
}
