// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/jujuc"
	"path/filepath"
)

type UnitGetSuite struct {
	ContextSuite
}

var _ = Suite(&UnitGetSuite{})

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

func (s *UnitGetSuite) createCommand(c *C) cmd.Command {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "unit-get")
	c.Assert(err, IsNil)
	return com
}

func (s *UnitGetSuite) TestOutputFormat(c *C) {
	for _, t := range unitGetTests {
		com := s.createCommand(c)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, Equals, 0)
		c.Assert(bufferString(ctx.Stderr), Equals, "")
		c.Assert(bufferString(ctx.Stdout), Matches, t.out)
	}
}

func (s *UnitGetSuite) TestHelp(c *C) {
	com := s.createCommand(c)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, Equals, 0)
	c.Assert(bufferString(ctx.Stdout), Equals, `usage: unit-get [options] <setting>
purpose: print public-address or private-address

options:
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file
`)
	c.Assert(bufferString(ctx.Stderr), Equals, "")
}

func (s *UnitGetSuite) TestOutputPath(c *C) {
	com := s.createCommand(c)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--output", "some-file", "private-address"})
	c.Assert(code, Equals, 0)
	c.Assert(bufferString(ctx.Stderr), Equals, "")
	c.Assert(bufferString(ctx.Stdout), Equals, "")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "192.168.0.99\n")
}

func (s *UnitGetSuite) TestUnknownSetting(c *C) {
	com := s.createCommand(c)
	err := testing.InitCommand(com, []string{"protected-address"})
	c.Assert(err, ErrorMatches, `unknown setting "protected-address"`)
}

func (s *UnitGetSuite) TestUnknownArg(c *C) {
	com := s.createCommand(c)
	err := testing.InitCommand(com, []string{"private-address", "blah"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["blah"\]`)
}
