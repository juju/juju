package main

import (
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/version"
)

type VersionSuite struct{}

var _ = Suite(&VersionSuite{})

func (s *VersionSuite) TestVersion(c *C) {
	var stdout, stderr bytes.Buffer
	ctx := &cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	code := cmd.Main(&VersionCommand{}, ctx, nil)
	c.Check(code, Equals, 0)
	c.Assert(stderr.String(), Equals, "")
	c.Assert(stdout.String(), Equals, version.Current.String()+"\n")
}

func (s *VersionSuite) TestVersionExtraArgs(c *C) {
	var stdout, stderr bytes.Buffer
	ctx := &cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	code := cmd.Main(&VersionCommand{}, ctx, []string{"foo"})
	c.Check(code, Equals, 2)
	c.Assert(stdout.String(), Equals, "")
	c.Assert(stderr.String(), Matches, "error: unrecognized args.*\n")
}

func (s *VersionSuite) TestVersionJson(c *C) {
	var stdout, stderr bytes.Buffer
	ctx := &cmd.Context{
		Stdout: &stdout,
		Stderr: &stderr,
	}
	code := cmd.Main(&VersionCommand{}, ctx, []string{"--format", "json"})
	c.Check(code, Equals, 0)
	c.Assert(stderr.String(), Equals, "")
	c.Assert(stdout.String(), Equals, fmt.Sprintf("%q", version.Current.String())+"\n")
}
