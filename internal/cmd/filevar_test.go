// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/gnuflag"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

type FileVarSuite struct {
	gitjujutesting.FakeHomeSuite
	ctx         *cmd.Context
	ValidPath   string
	InvalidPath string // invalid path refers to a file which is not readable
}

var _ = gc.Suite(&FileVarSuite{})

func (s *FileVarSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	s.ctx = cmdtesting.Context(c)
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

func (s *FileVarSuite) TestSetStdin(c *gc.C) {
	var config cmd.FileVar
	c.Assert(config.Path, gc.Equals, "")
	c.Assert(config.StdinMarkers, jc.DeepEquals, []string{})

	config.SetStdin()
	c.Assert(config.Path, gc.Equals, "")
	c.Assert(config.StdinMarkers, jc.DeepEquals, []string{"-"})

	config.SetStdin("<>", "@")
	c.Assert(config.Path, gc.Equals, "")
	c.Assert(config.StdinMarkers, jc.DeepEquals, []string{"<>", "@"})
}

func (s *FileVarSuite) TestIsStdin(c *gc.C) {
	var config cmd.FileVar
	c.Check(config.IsStdin(), jc.IsFalse)

	config.StdinMarkers = []string{"-"}
	c.Check(config.IsStdin(), jc.IsFalse)

	config.Path = "spam"
	c.Check(config.IsStdin(), jc.IsFalse)

	config.Path = "-"
	c.Check(config.IsStdin(), jc.IsTrue)

	config.StdinMarkers = nil
	c.Check(config.IsStdin(), jc.IsFalse)

	config.StdinMarkers = []string{"<>", "@"}
	c.Check(config.IsStdin(), jc.IsFalse)

	config.Path = "<>"
	c.Check(config.IsStdin(), jc.IsTrue)

	config.Path = "@"
	c.Check(config.IsStdin(), jc.IsTrue)
}

func (FileVarSuite) checkOpen(c *gc.C, file io.ReadCloser, expected string) {
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Equals, expected)
}

func (s *FileVarSuite) TestOpenTilde(c *gc.C) {
	path := filepath.Join(utils.Home(), "config.yaml")
	err := ioutil.WriteFile(path, []byte("abc"), 0644)
	c.Assert(err, gc.IsNil)

	var config cmd.FileVar
	config.Set("~/config.yaml")
	file, err := config.Open(s.ctx)
	c.Assert(err, gc.IsNil)

	s.checkOpen(c, file, "abc")
}

func (s *FileVarSuite) TestOpenStdin(c *gc.C) {
	s.ctx.Stdin = bytes.NewBufferString("abc")

	var config cmd.FileVar
	config.SetStdin()
	config.Set("-")
	file, err := config.Open(s.ctx)
	c.Assert(err, gc.IsNil)
	s.checkOpen(c, file, "abc")
}

func (s *FileVarSuite) TestOpenNotStdin(c *gc.C) {
	var config cmd.FileVar
	config.Set("-")
	_, err := config.Open(s.ctx)

	c.Check(err, jc.Satisfies, os.IsNotExist)
}

func (s *FileVarSuite) TestOpenValid(c *gc.C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.ValidPath})
	c.Assert(err, gc.IsNil)
	c.Assert(config.Path, gc.Equals, s.ValidPath)
	_, err = config.Open(s.ctx)
	c.Assert(err, gc.IsNil)
}

func (s *FileVarSuite) TestOpenInvalid(c *gc.C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.InvalidPath})
	c.Assert(err, gc.IsNil)
	c.Assert(config.Path, gc.Equals, s.InvalidPath)
	_, err = config.Open(s.ctx)
	c.Assert(err, gc.ErrorMatches, "*permission denied")
}

func (s *FileVarSuite) TestReadTilde(c *gc.C) {
	path := filepath.Join(utils.Home(), "config.yaml")
	err := ioutil.WriteFile(path, []byte("abc"), 0644)
	c.Assert(err, gc.IsNil)

	var config cmd.FileVar
	config.Set("~/config.yaml")
	file, err := config.Read(s.ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(string(file), gc.Equals, "abc")
}

func (s *FileVarSuite) TestReadStdin(c *gc.C) {
	s.ctx.Stdin = bytes.NewBufferString("abc")

	var config cmd.FileVar
	config.SetStdin()
	config.Set("-")
	file, err := config.Read(s.ctx)
	c.Assert(err, gc.IsNil)
	c.Assert(string(file), gc.Equals, "abc")
}

func (s *FileVarSuite) TestReadNotStdin(c *gc.C) {
	var config cmd.FileVar
	config.Set("-")
	_, err := config.Read(s.ctx)

	c.Check(err, jc.Satisfies, os.IsNotExist)
}

func (s *FileVarSuite) TestReadValid(c *gc.C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.ValidPath})
	c.Assert(err, gc.IsNil)
	c.Assert(config.Path, gc.Equals, s.ValidPath)
	_, err = config.Read(s.ctx)
	c.Assert(err, gc.IsNil)
}

func (s *FileVarSuite) TestReadInvalid(c *gc.C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.InvalidPath})
	c.Assert(err, gc.IsNil)
	c.Assert(config.Path, gc.Equals, s.InvalidPath)
	_, err = config.Read(s.ctx)
	c.Assert(err, gc.ErrorMatches, "*permission denied")
}

func fs() (*gnuflag.FlagSet, *cmd.FileVar) {
	var config cmd.FileVar
	fs := cmdtesting.NewFlagSet()
	fs.Var(&config, "config", "the config")
	return fs, &config
}
