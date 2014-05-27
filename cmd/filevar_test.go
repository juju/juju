// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"launchpad.net/gnuflag"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type FileVarSuite struct {
	testing.FakeHomeSuite
	ctx         *cmd.Context
	ValidPath   string
	InvalidPath string // invalid path refers to a file which is not readable
}

var _ = gc.Suite(&FileVarSuite{})

func (s *FileVarSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	s.ctx = testing.Context(c)
	s.ValidPath = s.ctx.AbsPath("valid.yaml")
	s.InvalidPath = s.ctx.AbsPath("invalid.yaml")
	f, err := os.Create(s.ValidPath)
	c.Assert(err, gc.IsNil)
	f.Close()
	f, err = os.Create(s.InvalidPath)
	c.Assert(err, gc.IsNil)
	f.Close()
	err = os.Chmod(s.InvalidPath, 0) // make unreadable
	c.Assert(err, gc.IsNil)
}

func (s *FileVarSuite) TestTildeFileVar(c *gc.C) {
	path := filepath.Join(utils.Home(), "config.yaml")
	err := ioutil.WriteFile(path, []byte("abc"), 0644)
	c.Assert(err, gc.IsNil)

	var config cmd.FileVar
	config.Set("~/config.yaml")
	file, err := config.Read(s.ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(string(file), gc.Equals, "abc")
}

func (s *FileVarSuite) TestValidFileVar(c *gc.C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.ValidPath})
	c.Assert(err, gc.IsNil)
	c.Assert(config.Path, gc.Equals, s.ValidPath)
	_, err = config.Read(s.ctx)
	c.Assert(err, gc.IsNil)
}

func (s *FileVarSuite) TestInvalidFileVar(c *gc.C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.InvalidPath})
	c.Assert(config.Path, gc.Equals, s.InvalidPath)
	_, err = config.Read(s.ctx)
	c.Assert(err, gc.ErrorMatches, "*permission denied")
}

func fs() (*gnuflag.FlagSet, *cmd.FileVar) {
	var config cmd.FileVar
	fs := testing.NewFlagSet()
	fs.Var(&config, "config", "the config")
	return fs, &config
}
