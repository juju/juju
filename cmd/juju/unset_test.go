// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
)

type UnsetSuite struct {
	testing.JujuConnSuite
	svc *state.Service
	dir string
}

var _ = gc.Suite(&UnsetSuite{})

func (s *UnsetSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	ch := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "dummy-service", ch)
	s.svc = svc
	s.dir = c.MkDir()
	setupConfigFile(c, s.dir)
}

func (s *UnsetSuite) TestUnsetOptionOneByOneSuccess(c *gc.C) {
	// Set options as preparation.
	assertSetSuccess(c, s.dir, s.svc, []string{
		"username=hello",
		"outlook=hello@world.tld",
	}, charm.Settings{
		"username": "hello",
		"outlook":  "hello@world.tld",
	})

	// Unset one by one.
	assertUnsetSuccess(c, s.dir, s.svc, []string{"username"}, charm.Settings{
		"outlook": "hello@world.tld",
	})
	assertUnsetSuccess(c, s.dir, s.svc, []string{"outlook"}, charm.Settings{})
}

func (s *UnsetSuite) TestUnsetOptionMultipleAtOnceSuccess(c *gc.C) {
	// Set options as preparation.
	assertSetSuccess(c, s.dir, s.svc, []string{
		"username=hello",
		"outlook=hello@world.tld",
	}, charm.Settings{
		"username": "hello",
		"outlook":  "hello@world.tld",
	})

	// Unset multiple options at once.
	assertUnsetSuccess(c, s.dir, s.svc, []string{"username", "outlook"}, charm.Settings{})
}

func (s *UnsetSuite) TestUnsetOptionFail(c *gc.C) {
	assertUnsetFail(c, s.dir, []string{}, "error: no configuration options specified\n")
	assertUnsetFail(c, s.dir, []string{"invalid"}, "error: unknown option \"invalid\"\n")
	assertUnsetFail(c, s.dir, []string{"username=bar"}, "error: unknown option \"username=bar\"\n")
	assertUnsetFail(c, s.dir, []string{
		"username",
		"outlook",
		"invalid",
	}, "error: unknown option \"invalid\"\n")
}

// assertUnsetSuccess unsets configuration options and checks the expected settings.
func assertUnsetSuccess(c *gc.C, dir string, svc *state.Service, args []string, expect charm.Settings) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(&UnsetCommand{}, ctx, append([]string{"dummy-service"}, args...))
	c.Check(code, gc.Equals, 0)
	settings, err := svc.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, expect)
}

// assertUnsetFail unsets configuration options and checks the expected error.
func assertUnsetFail(c *gc.C, dir string, args []string, err string) {
	ctx := coretesting.ContextForDir(c, dir)
	code := cmd.Main(&UnsetCommand{}, ctx, append([]string{"dummy-service"}, args...))
	c.Check(code, gc.Not(gc.Equals), 0)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Matches, err)
}
