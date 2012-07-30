package server_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"path/filepath"
)

type ConfigGetSuite struct {
	HookContextSuite
}

var _ = Suite(&ConfigGetSuite{})

func (s *ConfigGetSuite) SetUpTest(c *C) {
	s.HookContextSuite.SetUpTest(c)
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
		hctx := s.GetHookContext(c, -1, "")
		com, err := hctx.NewCommand("config-get")
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
		hctx := s.GetHookContext(c, -1, "")
		com, err := hctx.NewCommand("config-get")
		c.Assert(err, IsNil)
		ctx := dummyContext(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, Equals, t.code)
		c.Assert(bufferString(ctx.Stderr), Equals, "")
		c.Assert(bufferString(ctx.Stdout), Equals, "")
	}
}

func (s *ConfigGetSuite) TestHelp(c *C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := hctx.NewCommand("config-get")
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
    returns non-zero exit code if value is false/zero/empty

If a key is given, only the value for that key will be printed.
`)
}

func (s *ConfigGetSuite) TestOutputPath(c *C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := hctx.NewCommand("config-get")
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
	hctx := s.GetHookContext(c, -1, "")
	com, err := hctx.NewCommand("config-get")
	c.Assert(err, IsNil)
	err = com.Init(dummyFlagSet(), []string{"multiple", "keys"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["keys"\]`)
}
