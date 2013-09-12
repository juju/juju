// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"io/ioutil"
	"path/filepath"

	gc "launchpad.net/gocheck"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/testing"
)

type LogSuite struct {
}

var _ = gc.Suite(&LogSuite{})

func (s *LogSuite) TearDownTest(c *gc.C) {
	loggo.ResetLoggers()
	loggo.ResetWriters()
}

func (s *LogSuite) TestAddFlags(c *gc.C) {
	l := &cmd.Log{}
	f := testing.NewFlagSet()
	l.AddFlags(f)

	err := f.Parse(false, []string{})
	c.Assert(err, gc.IsNil)
	c.Assert(l.Path, gc.Equals, "")
	c.Assert(l.Verbose, gc.Equals, false)
	c.Assert(l.Debug, gc.Equals, false)
	c.Assert(l.Config, gc.Equals, "")

	err = f.Parse(false, []string{"--log-file", "foo", "--verbose", "--debug",
		"--log-config=juju.cmd=INFO;juju.worker.deployer=DEBUG"})
	c.Assert(err, gc.IsNil)
	c.Assert(l.Path, gc.Equals, "foo")
	c.Assert(l.Verbose, gc.Equals, true)
	c.Assert(l.Debug, gc.Equals, true)
	c.Assert(l.Config, gc.Equals, "juju.cmd=INFO;juju.worker.deployer=DEBUG")
}

func (s *LogSuite) TestVerboseSetsLogLevel(c *gc.C) {
	l := &cmd.Log{Verbose: true}
	ctx := testing.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)

	c.Assert(loggo.GetLogger("").LogLevel(), gc.Equals, loggo.INFO)
}

func (s *LogSuite) TestDebugSetsLogLevel(c *gc.C) {
	l := &cmd.Log{Debug: true}
	ctx := testing.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)

	c.Assert(loggo.GetLogger("").LogLevel(), gc.Equals, loggo.DEBUG)
}

func (s *LogSuite) TestStderr(c *gc.C) {
	l := &cmd.Log{Verbose: true, Config: "<root>=INFO"}
	ctx := testing.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)
	log.Infof("hello")
	c.Assert(bufferString(ctx.Stderr), gc.Matches, `^.* INFO .* hello\n`)
}

func (s *LogSuite) TestRelPathLog(c *gc.C) {
	l := &cmd.Log{Path: "foo.log", Config: "<root>=INFO"}
	ctx := testing.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)
	log.Infof("hello")
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "foo.log"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Matches, `^.* INFO .* hello\n`)
}

func (s *LogSuite) TestAbsPathLog(c *gc.C) {
	path := filepath.Join(c.MkDir(), "foo.log")
	l := &cmd.Log{Path: path, Config: "<root>=INFO"}
	ctx := testing.Context(c)
	err := l.Start(ctx)
	c.Assert(err, gc.IsNil)
	log.Infof("hello")
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	content, err := ioutil.ReadFile(path)
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Matches, `^.* INFO .* hello\n`)
}
