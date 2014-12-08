// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type ConfigGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&ConfigGetSuite{})

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

func (s *ConfigGetSuite) TestOutputFormatKey(c *gc.C) {
	for i, t := range configGetKeyTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, cmdString("config-get"))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Matches, t.out)
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

func (s *ConfigGetSuite) TestOutputFormatAll(c *gc.C) {
	for i, t := range configGetAllTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, cmdString("config-get"))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")

		out := map[string]interface{}{}
		switch t.format {
		case formatYaml:
			c.Assert(goyaml.Unmarshal(bufferBytes(ctx.Stdout), &out), gc.IsNil)
		case formatJson:
			c.Assert(json.Unmarshal(bufferBytes(ctx.Stdout), &out), gc.IsNil)
		}
		c.Assert(out, gc.DeepEquals, t.out)
	}
}

func (s *ConfigGetSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, cmdString("config-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `usage: config-get [options] [<key>]
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
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *ConfigGetSuite) TestOutputPath(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, cmdString("config-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--output", "some-file", "monsters"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "False\n")
}

func (s *ConfigGetSuite) TestUnknownArg(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, cmdString("config-get"))
	c.Assert(err, jc.ErrorIsNil)
	testing.TestInit(c, com, []string{"multiple", "keys"}, `unrecognized args: \["keys"\]`)
}

func (s *ConfigGetSuite) TestAllPlusKey(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, cmdString("config-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--all", "--format", "json", "monsters"})
	c.Assert(code, gc.Equals, 2)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "error: cannot use argument --all together with key \"monsters\"\n")
}
