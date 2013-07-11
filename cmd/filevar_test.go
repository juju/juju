// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"os"

	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type FileVarSuite struct {
	ctx         *cmd.Context
	ValidPath   string
	InvalidPath string // invalid path refers to a file which is not readable
}

var _ = Suite(&FileVarSuite{})

func (s *FileVarSuite) SetUpTest(c *C) {
	s.ctx = testing.Context(c)
	s.ValidPath = s.ctx.AbsPath("valid.yaml")
	s.InvalidPath = s.ctx.AbsPath("invalid.yaml")
	f, err := os.Create(s.ValidPath)
	c.Assert(err, IsNil)
	f.Close()
	f, err = os.Create(s.InvalidPath)
	c.Assert(err, IsNil)
	f.Close()
	err = os.Chmod(s.InvalidPath, 0) // make unreadable
	c.Assert(err, IsNil)
}

func (s *FileVarSuite) TestValidFileVar(c *C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.ValidPath})
	c.Assert(err, IsNil)
	c.Assert(config.Path, Equals, s.ValidPath)
	_, err = config.Read(s.ctx)
	c.Assert(err, IsNil)
}

func (s *FileVarSuite) TestInvalidFileVar(c *C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.InvalidPath})
	c.Assert(config.Path, Equals, s.InvalidPath)
	_, err = config.Read(s.ctx)
	c.Assert(err, ErrorMatches, "*permission denied")
}

func fs() (*gnuflag.FlagSet, *cmd.FileVar) {
	var config cmd.FileVar
	fs := testing.NewFlagSet()
	fs.Var(&config, "config", "the config")
	return fs, &config
}
