// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"io/ioutil"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/jujuc"
)

type OwnerGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&OwnerGetSuite{})

var ownerGetTests = []struct {
	args []string
	out  string
}{
	{[]string{"tag"}, "test-owner\n"},
	{[]string{"tag", "--format", "yaml"}, "test-owner\n"},
	{[]string{"tag", "--format", "json"}, `"test-owner"` + "\n"},
}

func (s *OwnerGetSuite) createCommand(c *gc.C) cmd.Command {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "owner-get")
	c.Assert(err, gc.IsNil)
	return com
}

func (s *OwnerGetSuite) TestOutputFormat(c *gc.C) {
	for _, t := range ownerGetTests {
		com := s.createCommand(c)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Matches, t.out)
	}
}

func (s *OwnerGetSuite) TestHelp(c *gc.C) {
	com := s.createCommand(c)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `usage: owner-get [options] <setting>
purpose: print information about the owner of the service. The only valid value for <setting> is currently tag

options:
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file
`)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *OwnerGetSuite) TestOutputPath(c *gc.C) {
	com := s.createCommand(c)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--output", "some-file", "tag"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, "test-owner\n")
}

func (s *OwnerGetSuite) TestUnknownSetting(c *gc.C) {
	com := s.createCommand(c)
	err := testing.InitCommand(com, []string{"unknown-setting"})
	c.Assert(err, gc.ErrorMatches, `unknown setting "unknown-setting"`)
}

func (s *OwnerGetSuite) TestUnknownArg(c *gc.C) {
	com := s.createCommand(c)
	err := testing.InitCommand(com, []string{"tag", "blah"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["blah"\]`)
}
