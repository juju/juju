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

type UnitGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&UnitGetSuite{})

var unitGetTests = []struct {
	args []string
	out  string
}{
	{[]string{"private-address"}, "192.168.0.99\n"},
	{[]string{"private-address", "--format", "yaml"}, "192.168.0.99\n"},
	{[]string{"private-address", "--format", "json"}, `"192.168.0.99"` + "\n"},
	{[]string{"public-address"}, "gimli.minecraft.testing.invalid\n"},
	{[]string{"public-address", "--format", "yaml"}, "gimli.minecraft.testing.invalid\n"},
	{[]string{"public-address", "--format", "json"}, `"gimli.minecraft.testing.invalid"` + "\n"},
}

func (s *UnitGetSuite) createCommand(c *gc.C) cmd.Command {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "unit-get")
	c.Assert(err, gc.IsNil)
	return com
}

func (s *UnitGetSuite) TestOutputFormat(c *gc.C) {
	for _, t := range unitGetTests {
		com := s.createCommand(c)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Matches, t.out)
	}
}

func (s *UnitGetSuite) TestHelp(c *gc.C) {
	com := s.createCommand(c)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `usage: unit-get [options] <setting>
purpose: print public-address or private-address

options:
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file
`)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *UnitGetSuite) TestOutputPath(c *gc.C) {
	com := s.createCommand(c)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--output", "some-file", "private-address"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(content), gc.Equals, "192.168.0.99\n")
}

func (s *UnitGetSuite) TestUnknownSetting(c *gc.C) {
	com := s.createCommand(c)
	err := testing.InitCommand(com, []string{"protected-address"})
	c.Assert(err, gc.ErrorMatches, `unknown setting "protected-address"`)
}

func (s *UnitGetSuite) TestUnknownArg(c *gc.C) {
	com := s.createCommand(c)
	err := testing.InitCommand(com, []string{"private-address", "blah"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["blah"\]`)
}
