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

var configGetYamlMap = "(spline-reticulation: 45\nmonsters: false\n|monsters: false\nspline-reticulation: 45\n)\n"
var configGetTests = []struct {
	args []string
	out  string
}{
	{[]string{"monsters"}, "false\n\n"},
	{[]string{"--format", "yaml", "monsters"}, "false\n\n"},
	{[]string{"--format", "json", "monsters"}, "false\n"},
	{[]string{"spline-reticulation"}, "45\n\n"},
	{[]string{"--format", "yaml", "spline-reticulation"}, "45\n\n"},
	{[]string{"--format", "json", "spline-reticulation"}, "45\n"},
	{[]string{"missing"}, ""},
	{[]string{"--format", "yaml", "missing"}, ""},
	{[]string{"--format", "json", "missing"}, "null\n"},
	{nil, configGetYamlMap},
	{[]string{"--format", "yaml"}, configGetYamlMap},
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

var configGetTestModeTests = []struct {
	args []string
	code int
}{
	{[]string{"monsters", "--test"}, 1},
	{[]string{"spline-reticulation", "--test"}, 0},
	{[]string{"missing", "--test"}, 1},
	{[]string{"--test"}, 0},
}

func (s *ConfigGetSuite) TestTestMode(c *C) {
	for _, t := range configGetTestModeTests {
		com, err := s.ctx.NewCommand("config-get")
		c.Assert(err, IsNil)
		ctx := dummyContext(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, Equals, t.code)
		c.Assert(bufferString(ctx.Stderr), Equals, "")
		c.Assert(bufferString(ctx.Stdout), Equals, "")
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
--format  (= yaml)
    specify output format (json|yaml)
-o, --output (= "")
    specify an output file
--test  (= false)
    suppress output; communicate result truthiness in return code

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
	c.Assert(string(content), Equals, "false\n\n")
}

func (s *ConfigGetSuite) TestUnknownArg(c *C) {
	com, err := s.ctx.NewCommand("config-get")
	c.Assert(err, IsNil)
	err = com.Init(dummyFlagSet(), []string{"multiple", "keys"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["keys"\]`)
}

func (s *ConfigGetSuite) TestBadState(c *C) {
	s.ctx.State = nil
	com, err := s.ctx.NewCommand("config-get")
	c.Assert(com, IsNil)
	c.Assert(err, ErrorMatches, "context TestCtx cannot access state")
}

func (s *ConfigGetSuite) TestBadUnit(c *C) {
	s.ctx.LocalUnitName = ""
	com, err := s.ctx.NewCommand("config-get")
	c.Assert(com, IsNil)
	c.Assert(err, ErrorMatches, "context TestCtx is not attached to a unit")
}
