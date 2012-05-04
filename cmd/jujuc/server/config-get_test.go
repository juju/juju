package server_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	"path/filepath"
)

type ConfigGetSuite struct {
	UnitFixture
}

var _ = Suite(&ConfigGetSuite{})

func (s *ConfigGetSuite) SetUpTest(c *C) {
	s.UnitFixture.SetUpTest(c)
	conf, err := s.service.Config()
	c.Assert(err, IsNil)
	conf.Update(map[string]interface{}{
		"monsters":            false,
		"spline-reticulation": 45.0,
	})
	_, err = conf.Write()
	c.Assert(err, IsNil)
}

var configGetSmartMap = `map\[(spline-reticulation:45 monsters:false|monsters:false spline-reticulation:45)\]` + "\n"
var configGetTests = []struct {
	args []string
	out  string
}{
	{[]string{"monsters"}, "false\n"},
	{[]string{"--format", "smart", "monsters"}, "false\n"},
	{[]string{"--format", "json", "monsters"}, "false\n"},
	{[]string{"spline-reticulation"}, "45\n"},
	{[]string{"--format", "smart", "spline-reticulation"}, "45\n"},
	{[]string{"--format", "json", "spline-reticulation"}, "45\n"},
	{[]string{"missing"}, ""},
	{[]string{"--format", "smart", "missing"}, ""},
	{[]string{"--format", "json", "missing"}, "null\n"},
	{nil, configGetSmartMap},
	{[]string{"--format", "smart"}, configGetSmartMap},
	{[]string{"--format", "json"}, `{"monsters":false,"spline-reticulation":45}` + "\n"},
}

func (s *ConfigGetSuite) TestOutputFormat(c *C) {
	for _, t := range configGetTests {
		com, err := s.ctx.NewCommand("config-get")
		c.Assert(err, IsNil)
		ctx := dummyContext(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, Equals, 0)
		c.Assert(bufferString(ctx.Stderr), Equals, "")
		c.Assert(bufferString(ctx.Stdout), Matches, t.out)
	}
}

func (s *ConfigGetSuite) TestHelp(c *C) {
	com, err := s.ctx.NewCommand("config-get")
	c.Assert(err, IsNil)
	ctx := dummyContext(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, Equals, 0)
	c.Assert(bufferString(ctx.Stdout), Equals, "")
	c.Assert(bufferString(ctx.Stderr), Equals, `usage: config-get [options] [<key>]
purpose: print service configuration

options:
--format  (= smart)
    specify output format (json|smart)
-o, --output (= "")
    specify an output file

If a key is given, only the value for that key will be printed.
`)
}

func (s *ConfigGetSuite) TestOutputPath(c *C) {
	com, err := s.ctx.NewCommand("config-get")
	c.Assert(err, IsNil)
	ctx := dummyContext(c)
	code := cmd.Main(com, ctx, []string{"--output", "some-file", "monsters"})
	c.Assert(code, Equals, 0)
	c.Assert(bufferString(ctx.Stderr), Equals, "")
	c.Assert(bufferString(ctx.Stdout), Equals, "")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "false\n")
}

func (s *ConfigGetSuite) TestUnknownArg(c *C) {
	com, err := s.ctx.NewCommand("config-get")
	c.Assert(err, IsNil)
	err = com.Init(dummyFlagSet(), []string{"multiple", "keys"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["keys"\]`)
}

func (s *ConfigGetSuite) TestUnitCommand(c *C) {
	s.AssertUnitCommand(c, "config-get")
}
