package cmd_test

import (
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"os"
)

type FileVarSuite struct {
	ctx         *cmd.Context
	ValidPath   string
	InvalidPath string // invalid path refers to a file which is not readable
}

var _ = Suite(&FileVarSuite{})

func (s *FileVarSuite) SetUpTest(c *C) {
	s.ctx = &cmd.Context{c.MkDir(), nil, nil, nil}
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
	fs := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	fs.SetOutput(ioutil.Discard)
	fs.Var(&config, "config", "the config")
	return fs, &config
}
