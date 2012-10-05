package jujuc_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"path/filepath"
)

type UnitGetSuite struct {
	HookContextSuite
}

var _ = Suite(&UnitGetSuite{})

func (s *UnitGetSuite) SetUpTest(c *C) {
	s.HookContextSuite.SetUpTest(c)
	err := s.unit.SetPublicAddress("gimli.minecraft.example.com")
	c.Assert(err, IsNil)
	err = s.unit.SetPrivateAddress("192.168.0.99")
	c.Assert(err, IsNil)
}

var unitGetTests = []struct {
	args []string
	out  string
}{
	{[]string{"private-address"}, "192.168.0.99\n"},
	{[]string{"private-address", "--format", "yaml"}, "192.168.0.99\n"},
	{[]string{"private-address", "--format", "json"}, `"192.168.0.99"` + "\n"},
	{[]string{"public-address"}, "gimli.minecraft.example.com\n"},
	{[]string{"public-address", "--format", "yaml"}, "gimli.minecraft.example.com\n"},
	{[]string{"public-address", "--format", "json"}, `"gimli.minecraft.example.com"` + "\n"},
}

func (s *UnitGetSuite) TestOutputFormat(c *C) {
	for _, t := range unitGetTests {
		hctx := s.GetHookContext(c, -1, "")
		com, err := hctx.NewCommand("unit-get")
		c.Assert(err, IsNil)
		ctx := dummyContext(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, Equals, 0)
		c.Assert(bufferString(ctx.Stderr), Equals, "")
		c.Assert(bufferString(ctx.Stdout), Matches, t.out)
	}
}

func (s *UnitGetSuite) TestHelp(c *C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := hctx.NewCommand("unit-get")
	c.Assert(err, IsNil)
	ctx := dummyContext(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, Equals, 0)
	c.Assert(bufferString(ctx.Stdout), Equals, "")
	c.Assert(bufferString(ctx.Stderr), Equals, `usage: unit-get [options] <setting>
purpose: print public-address or private-address

options:
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file
`)
}

func (s *UnitGetSuite) TestOutputPath(c *C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := hctx.NewCommand("unit-get")
	c.Assert(err, IsNil)
	ctx := dummyContext(c)
	code := cmd.Main(com, ctx, []string{"--output", "some-file", "private-address"})
	c.Assert(code, Equals, 0)
	c.Assert(bufferString(ctx.Stderr), Equals, "")
	c.Assert(bufferString(ctx.Stdout), Equals, "")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "192.168.0.99\n")
}

func (s *UnitGetSuite) TestUnknownSetting(c *C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := hctx.NewCommand("unit-get")
	c.Assert(err, IsNil)
	err = com.Init(dummyFlagSet(), []string{"protected-address"})
	c.Assert(err, ErrorMatches, `unknown setting "protected-address"`)
}

func (s *UnitGetSuite) TestUnknownArg(c *C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := hctx.NewCommand("unit-get")
	c.Assert(err, IsNil)
	err = com.Init(dummyFlagSet(), []string{"private-address", "blah"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["blah"\]`)
}
