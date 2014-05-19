// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"io/ioutil"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
)

type SetSuite struct {
	testing.JujuConnSuite
	svc *state.Service
	dir string
}

var _ = gc.Suite(&SetSuite{})

func (s *SetSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	ch := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "dummy-service", ch)
	s.svc = svc
	s.dir = c.MkDir()
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

}

func (s *SetSuite) TestSetOptionFail(c *gc.C) {
	assertSetFail(c, s.dir, []string{"foo", "bar"}, "error: invalid option: \"foo\"\n")
	assertSetFail(c, s.dir, []string{"=bar"}, "error: invalid option: \"=bar\"\n")
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
