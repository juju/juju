// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/gnuflag"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
)

type FileVarSuite struct {
	testhelpers.FakeHomeSuite
	ctx         *cmd.Context
	ValidPath   string
	InvalidPath string // invalid path refers to a file which is not readable
}

func TestFileVarSuite(t *testing.T) {
	tc.Run(t, &FileVarSuite{})
}

func (s *FileVarSuite) SetUpTest(c *tc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	s.ctx = cmdtesting.Context(c)
	s.ValidPath = s.ctx.AbsPath("valid.yaml")
	s.InvalidPath = s.ctx.AbsPath("invalid.yaml")
	f, err := os.Create(s.ValidPath)
	c.Assert(err, tc.IsNil)
	f.Close()
	f, err = os.Create(s.InvalidPath)
	c.Assert(err, tc.IsNil)
	f.Close()
	err = os.Chmod(s.InvalidPath, 0) // make unreadable
	c.Assert(err, tc.IsNil)
}

func (s *FileVarSuite) TestSetStdin(c *tc.C) {
	var config cmd.FileVar
	c.Assert(config.Path, tc.Equals, "")
	c.Assert(config.StdinMarkers, tc.DeepEquals, []string{})

	config.SetStdin()
	c.Assert(config.Path, tc.Equals, "")
	c.Assert(config.StdinMarkers, tc.DeepEquals, []string{"-"})

	config.SetStdin("<>", "@")
	c.Assert(config.Path, tc.Equals, "")
	c.Assert(config.StdinMarkers, tc.DeepEquals, []string{"<>", "@"})
}

func (s *FileVarSuite) TestIsStdin(c *tc.C) {
	var config cmd.FileVar
	c.Check(config.IsStdin(), tc.IsFalse)

	config.StdinMarkers = []string{"-"}
	c.Check(config.IsStdin(), tc.IsFalse)

	config.Path = "spam"
	c.Check(config.IsStdin(), tc.IsFalse)

	config.Path = "-"
	c.Check(config.IsStdin(), tc.IsTrue)

	config.StdinMarkers = nil
	c.Check(config.IsStdin(), tc.IsFalse)

	config.StdinMarkers = []string{"<>", "@"}
	c.Check(config.IsStdin(), tc.IsFalse)

	config.Path = "<>"
	c.Check(config.IsStdin(), tc.IsTrue)

	config.Path = "@"
	c.Check(config.IsStdin(), tc.IsTrue)
}

func (s *FileVarSuite) checkOpen(c *tc.C, file io.ReadCloser, expected string) {
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, expected)
}

func (s *FileVarSuite) TestOpenTilde(c *tc.C) {
	path := filepath.Join(utils.Home(), "config.yaml")
	err := ioutil.WriteFile(path, []byte("abc"), 0644)
	c.Assert(err, tc.IsNil)

	var config cmd.FileVar
	config.Set("~/config.yaml")
	file, err := config.Open(s.ctx)
	c.Assert(err, tc.IsNil)

	s.checkOpen(c, file, "abc")
}

func (s *FileVarSuite) TestOpenStdin(c *tc.C) {
	s.ctx.Stdin = bytes.NewBufferString("abc")

	var config cmd.FileVar
	config.SetStdin()
	config.Set("-")
	file, err := config.Open(s.ctx)
	c.Assert(err, tc.IsNil)
	s.checkOpen(c, file, "abc")
}

func (s *FileVarSuite) TestOpenNotStdin(c *tc.C) {
	var config cmd.FileVar
	config.Set("-")
	_, err := config.Open(s.ctx)

	c.Check(err, tc.Satisfies, os.IsNotExist)
}

func (s *FileVarSuite) TestOpenValid(c *tc.C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.ValidPath})
	c.Assert(err, tc.IsNil)
	c.Assert(config.Path, tc.Equals, s.ValidPath)
	_, err = config.Open(s.ctx)
	c.Assert(err, tc.IsNil)
}

func (s *FileVarSuite) TestOpenInvalid(c *tc.C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.InvalidPath})
	c.Assert(err, tc.IsNil)
	c.Assert(config.Path, tc.Equals, s.InvalidPath)
	_, err = config.Open(s.ctx)
	c.Assert(err, tc.ErrorMatches, "*permission denied")
}

func (s *FileVarSuite) TestReadTilde(c *tc.C) {
	path := filepath.Join(utils.Home(), "config.yaml")
	err := ioutil.WriteFile(path, []byte("abc"), 0644)
	c.Assert(err, tc.IsNil)

	var config cmd.FileVar
	config.Set("~/config.yaml")
	file, err := config.Read(s.ctx)
	c.Assert(err, tc.IsNil)
	c.Assert(string(file), tc.Equals, "abc")
}

func (s *FileVarSuite) TestReadStdin(c *tc.C) {
	s.ctx.Stdin = bytes.NewBufferString("abc")

	var config cmd.FileVar
	config.SetStdin()
	config.Set("-")
	file, err := config.Read(s.ctx)
	c.Assert(err, tc.IsNil)
	c.Assert(string(file), tc.Equals, "abc")
}

func (s *FileVarSuite) TestReadNotStdin(c *tc.C) {
	var config cmd.FileVar
	config.Set("-")
	_, err := config.Read(s.ctx)

	c.Check(err, tc.Satisfies, os.IsNotExist)
}

func (s *FileVarSuite) TestReadValid(c *tc.C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.ValidPath})
	c.Assert(err, tc.IsNil)
	c.Assert(config.Path, tc.Equals, s.ValidPath)
	_, err = config.Read(s.ctx)
	c.Assert(err, tc.IsNil)
}

func (s *FileVarSuite) TestReadInvalid(c *tc.C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.InvalidPath})
	c.Assert(err, tc.IsNil)
	c.Assert(config.Path, tc.Equals, s.InvalidPath)
	_, err = config.Read(s.ctx)
	c.Assert(err, tc.ErrorMatches, "*permission denied")
}

func fs() (*gnuflag.FlagSet, *cmd.FileVar) {
	var config cmd.FileVar
	fs := cmdtesting.NewFlagSet()
	fs.Var(&config, "config", "the config")
	return fs, &config
}
