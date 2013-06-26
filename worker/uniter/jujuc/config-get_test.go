// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/jujuc"
)

type ConfigGetSuite struct {
	ContextSuite
}

var _ = Suite(&ConfigGetSuite{})

var configGetKeyTests = []struct {
	args []string
	out  string
}{
	{[]string{"monsters"}, "False\n"},
	{[]string{"--format", "yaml", "monsters"}, "false\n"},
	{[]string{"--format", "json", "monsters"}, "false\n"},
	{[]string{"spline-reticulation"}, "45\n"},
	{[]string{"--format", "yaml", "spline-reticulation"}, "45\n"},
	{[]string{"--format", "json", "spline-reticulation"}, "45\n"},
	{[]string{"missing"}, ""},
	{[]string{"--format", "yaml", "missing"}, ""},
	{[]string{"--format", "json", "missing"}, "null\n"},
}

func (s *ConfigGetSuite) TestOutputFormatKey(c *C) {
	for i, t := range configGetKeyTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, "config-get")
		c.Assert(err, IsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, Equals, 0)
		c.Assert(bufferString(ctx.Stderr), Equals, "")
		c.Assert(bufferString(ctx.Stdout), Matches, t.out)
	}
}

var (
	configGetYamlMap = map[string]interface{}{
		"monsters":            false,
		"spline-reticulation": 45,
		"title":               "My Title",
		"username":            "admin001",
	}
	configGetJsonMap = map[string]interface{}{
		"monsters":            false,
		"spline-reticulation": 45.0,
		"title":               "My Title",
		"username":            "admin001",
	}
	configGetYamlMapAll = map[string]interface{}{
		"empty":               nil,
		"monsters":            false,
		"spline-reticulation": 45,
		"title":               "My Title",
		"username":            "admin001",
	}
	configGetJsonMapAll = map[string]interface{}{
		"empty":               nil,
		"monsters":            false,
		"spline-reticulation": 45.0,
		"title":               "My Title",
		"username":            "admin001",
	}
)

const (
	formatYaml = iota
	formatJson
)

var configGetAllTests = []struct {
	args   []string
	format int
	out    map[string]interface{}
}{
	{nil, formatYaml, configGetYamlMap},
	{[]string{"--format", "yaml"}, formatYaml, configGetYamlMap},
	{[]string{"--format", "json"}, formatJson, configGetJsonMap},
	{[]string{"--all", "--format", "yaml"}, formatYaml, configGetYamlMapAll},
	{[]string{"--all", "--format", "json"}, formatJson, configGetJsonMapAll},
	{[]string{"-a", "--format", "yaml"}, formatYaml, configGetYamlMapAll},
	{[]string{"-a", "--format", "json"}, formatJson, configGetJsonMapAll},
}

func (s *ConfigGetSuite) TestOutputFormatAll(c *C) {
	for i, t := range configGetAllTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, "config-get")
		c.Assert(err, IsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, Equals, 0)
		c.Assert(bufferString(ctx.Stderr), Equals, "")

		out := map[string]interface{}{}
		switch t.format {
		case formatYaml:
			c.Assert(goyaml.Unmarshal(bufferBytes(ctx.Stdout), &out), IsNil)
		case formatJson:
			c.Assert(json.Unmarshal(bufferBytes(ctx.Stdout), &out), IsNil)
		}
		c.Assert(out, DeepEquals, t.out)
	}
}

func (s *ConfigGetSuite) TestHelp(c *C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "config-get")
	c.Assert(err, IsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, Equals, 0)
	c.Assert(bufferString(ctx.Stdout), Equals, `usage: config-get [options] [<key>]
purpose: print service configuration

options:
-a, --all  (= false)
    print all keys
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file

When no <key> is supplied, all keys with values or defaults are printed. If
--all is set, all known keys are printed; those without defaults or values are
reported as null. <key> and --all are mutually exclusive.
`)
	c.Assert(bufferString(ctx.Stderr), Equals, "")
}

func (s *ConfigGetSuite) TestOutputPath(c *C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "config-get")
	c.Assert(err, IsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--output", "some-file", "monsters"})
	c.Assert(code, Equals, 0)
	c.Assert(bufferString(ctx.Stderr), Equals, "")
	c.Assert(bufferString(ctx.Stdout), Equals, "")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "False\n")
}

func (s *ConfigGetSuite) TestUnknownArg(c *C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "config-get")
	c.Assert(err, IsNil)
	testing.TestInit(c, com, []string{"multiple", "keys"}, `unrecognized args: \["keys"\]`)
}

func (s *ConfigGetSuite) TestAllPlusKey(c *C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "config-get")
	c.Assert(err, IsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--all", "--format", "json", "monsters"})
	c.Assert(code, Equals, 2)
	c.Assert(bufferString(ctx.Stderr), Equals, "error: cannot use argument --all together with key \"monsters\"\n")
}
