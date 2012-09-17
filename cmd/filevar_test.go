package cmd_test

import (
	"io/ioutil"
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"os"
	"path/filepath"
)

type FileVarSuite struct {
	ValidPath   string
	InvalidPath string // invalid path refers to a file which is not readable
}

var _ = Suite(&FileVarSuite{})

func (s *FileVarSuite) SetUpTest(c *C) {
	dir := c.MkDir()
	s.ValidPath = filepath.Join(dir, "valid.yaml")
	s.InvalidPath = filepath.Join(dir, "invalid.yaml")
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
	c.Assert(config.ReadCloser, NotNil)
}

func (s *FileVarSuite) TestInvalidFileVar(c *C) {
	fs, config := fs()
	err := fs.Parse(false, []string{"--config", s.InvalidPath})
	c.Assert(err, ErrorMatches, ".*permission denied")
	c.Assert(config.Path, Equals, "")
	c.Assert(config.ReadCloser, IsNil)
}

func fs() (*gnuflag.FlagSet, *cmd.FileVar) {
	var config cmd.FileVar
	fs := gnuflag.NewFlagSet("", gnuflag.ContinueOnError)
	fs.SetOutput(ioutil.Discard)
	fs.Var(&config, "config", "the config")
	return fs, &config
}
