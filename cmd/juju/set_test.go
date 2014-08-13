// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"unicode/utf8"

	"github.com/juju/cmd"
	"gopkg.in/juju/charm.v3"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type SetSuite struct {
	testing.JujuConnSuite
	svc *state.Service
	dir string
}

var _ = gc.Suite(&SetSuite{})

var (
	validSetTestValue   = "a value with spaces\nand newline\nand UTF-8 characters: \U0001F604 / \U0001F44D"
	invalidSetTestValue = "a value with an invalid UTF-8 sequence: " + string([]byte{0xFF, 0xFF})
)

func (s *SetSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	ch := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "dummy-service", ch)
	s.svc = svc
	s.dir = c.MkDir()
	c.Assert(utf8.ValidString(validSetTestValue), gc.Equals, true)
	c.Assert(utf8.ValidString(invalidSetTestValue), gc.Equals, false)
	setupValueFile(c, s.dir, "valid.txt", validSetTestValue)
	setupValueFile(c, s.dir, "invalid.txt", invalidSetTestValue)
	setupBigFile(c, s.dir)
	setupConfigFile(c, s.dir)
}

func (s *SetSuite) TestSetOptionSuccess(c *gc.C) {
	assertSetSuccess(c, s.dir, s.svc, []string{
		"username=hello",
		"outlook=hello@world.tld",
	}, charm.Settings{
		"username": "hello",
		"outlook":  "hello@world.tld",
	})
	assertSetSuccess(c, s.dir, s.svc, []string{
		"username=hello=foo",
	}, charm.Settings{
		"username": "hello=foo",
		"outlook":  "hello@world.tld",
	})
	assertSetSuccess(c, s.dir, s.svc, []string{
		"username=@valid.txt",
	}, charm.Settings{
		"username": validSetTestValue,
		"outlook":  "hello@world.tld",
	})
}

func (s *SetSuite) TestSetOptionFail(c *gc.C) {
	assertSetFail(c, s.dir, []string{"foo", "bar"}, "error: invalid option: \"foo\"\n")
	assertSetFail(c, s.dir, []string{"=bar"}, "error: invalid option: \"=bar\"\n")
	assertSetFail(c, s.dir, []string{
		"username=@missing.txt",
	}, "error: cannot read option from file \"missing.txt\": .* no such file or directory\n")
	assertSetFail(c, s.dir, []string{
		"username=@big.txt",
	}, "error: size of option file is larger than 5M\n")
	assertSetFail(c, s.dir, []string{
		"username=@invalid.txt",
	}, "error: value for option \"username\" contains non-UTF-8 sequences\n")
}

func (s *SetSuite) TestSetConfig(c *gc.C) {
	assertSetFail(c, s.dir, []string{
		"--config",
		"missing.yaml",
	}, "error.*no such file or directory\n")

	assertSetSuccess(c, s.dir, s.svc, []string{
		"--config",
		"testconfig.yaml",
	}, charm.Settings{
		"username":    "admin001",
		"skill-level": int64(9000),
	})
}

// assertSetSuccess sets configuration options and checks the expected settings.
func assertSetSuccess(c *gc.C, dir string, svc *state.Service, args []string, expect charm.Settings) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(envcmd.Wrap(&SetCommand{}), ctx, append([]string{"dummy-service"}, args...))
	c.Check(code, gc.Equals, 0)
	settings, err := svc.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, expect)
}

// assertSetFail sets configuration options and checks the expected error.
func assertSetFail(c *gc.C, dir string, args []string, err string) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(envcmd.Wrap(&SetCommand{}), ctx, append([]string{"dummy-service"}, args...))
	c.Check(code, gc.Not(gc.Equals), 0)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Matches, err)
}

// setupValueFile creates a file containing one value for testing
// set with name=@filename.
func setupValueFile(c *gc.C, dir, filename, value string) string {
	ctx := coretesting.ContextForDir(c, dir)
	path := ctx.AbsPath(filename)
	content := []byte(value)
	err := ioutil.WriteFile(path, content, 0666)
	c.Assert(err, gc.IsNil)
	return path
}

// setupBigFile creates a too big file for testing
// set with name=@filename.
func setupBigFile(c *gc.C, dir string) string {
	ctx := coretesting.ContextForDir(c, dir)
	path := ctx.AbsPath("big.txt")
	file, err := os.Create(path)
	c.Assert(err, gc.IsNil)
	defer file.Close()
	chunk := make([]byte, 1024)
	for i := 0; i < cap(chunk); i++ {
		chunk[i] = byte(i % 256)
	}
	for i := 0; i < 6000; i++ {
		_, err = file.Write(chunk)
		c.Assert(err, gc.IsNil)
	}
	return path
}

// setupConfigFile creates a configuration file for testing set
// with the --config argument specifying a configuration file.
func setupConfigFile(c *gc.C, dir string) string {
	ctx := coretesting.ContextForDir(c, dir)
	path := ctx.AbsPath("testconfig.yaml")
	content := []byte("dummy-service:\n  skill-level: 9000\n  username: admin001\n\n")
	err := ioutil.WriteFile(path, content, 0666)
	c.Assert(err, gc.IsNil)
	return path
}
