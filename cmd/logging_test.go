// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/testing"
	"path/filepath"
)

type LogSuite struct {
}

var _ = Suite(&LogSuite{})

func (s *LogSuite) TestAddFlags(c *C) {
	l := &cmd.Log{}
	f := testing.NewFlagSet()
	l.AddFlags(f)

	err := f.Parse(false, []string{})
	c.Assert(err, IsNil)
	c.Assert(l.Path, Equals, "")
	c.Assert(l.Config, Equals, "")

	err = f.Parse(false, []string{"--log-file", "foo", "--log-config=juju.cmd=INFO;juju.worker.deployer=DEBUG"})
	c.Assert(err, IsNil)
	c.Assert(l.Path, Equals, "foo")
	c.Assert(l.Config, Equals, "juju.cmd=INFO;juju.worker.deployer=DEBUG")
}

func (s *LogSuite) TestStderr(c *C) {
	l := &cmd.Log{Config: "<root>=INFO"}
	ctx := testing.Context(c)
	err := l.Start(ctx)
	c.Assert(err, IsNil)
	log.Infof("hello")
	c.Assert(bufferString(ctx.Stderr), Matches, `^.* INFO .* hello\n`)
}

func (s *LogSuite) TestRelPathLog(c *C) {
	l := &cmd.Log{Path: "foo.log", Config: "<root>=INFO"}
	ctx := testing.Context(c)
	err := l.Start(ctx)
	c.Assert(err, IsNil)
	log.Infof("hello")
	c.Assert(bufferString(ctx.Stderr), Equals, "")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "foo.log"))
	c.Assert(err, IsNil)
	c.Assert(string(content), Matches, `^.* INFO .* hello\n`)
}

func (s *LogSuite) TestAbsPathLog(c *C) {
	path := filepath.Join(c.MkDir(), "foo.log")
	l := &cmd.Log{Path: path, Config: "<root>=INFO"}
	ctx := testing.Context(c)
	err := l.Start(ctx)
	c.Assert(err, IsNil)
	log.Infof("hello")
	c.Assert(bufferString(ctx.Stderr), Equals, "")
	content, err := ioutil.ReadFile(path)
	c.Assert(err, IsNil)
	c.Assert(string(content), Matches, `^.* INFO .* hello\n`)
}
