// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/version"
)

type VersionSuite struct{}

var _ = gc.Suite(&VersionSuite{})

func (s *VersionSuite) TestVersion(c *gc.C) {
	var stdout, stderr bytes.Buffer
	ctx := &Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	code := Main(&VersionCommand{}, ctx, nil)
	c.Check(code, gc.Equals, 0)
	c.Assert(stderr.String(), gc.Equals, "")
	c.Assert(stdout.String(), gc.Equals, version.Current.String()+"\n")
}

func (s *VersionSuite) TestVersionExtraArgs(c *gc.C) {
	var stdout, stderr bytes.Buffer
	ctx := &Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	code := Main(&VersionCommand{}, ctx, []string{"foo"})
	c.Check(code, gc.Equals, 2)
	c.Assert(stdout.String(), gc.Equals, "")
	c.Assert(stderr.String(), gc.Matches, "error: unrecognized args.*\n")
}

func (s *VersionSuite) TestVersionJson(c *gc.C) {
	var stdout, stderr bytes.Buffer
	ctx := &Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	code := Main(&VersionCommand{}, ctx, []string{"--format", "json"})
	c.Check(code, gc.Equals, 0)
	c.Assert(stderr.String(), gc.Equals, "")
	c.Assert(stdout.String(), gc.Equals, fmt.Sprintf("%q\n", version.Current.String()))
}
