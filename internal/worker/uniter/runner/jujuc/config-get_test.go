// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/juju/tc"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type ConfigGetSuite struct {
	ContextSuite
}

var _ = tc.Suite(&ConfigGetSuite{})

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

func (s *ConfigGetSuite) TestOutputFormatKey(c *tc.C) {
	for i, t := range configGetKeyTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, "config-get")
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Check(code, tc.Equals, 0)
		c.Check(bufferString(ctx.Stderr), tc.Equals, "")
		c.Check(bufferString(ctx.Stdout), tc.Matches, t.out)
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

func (s *ConfigGetSuite) TestOutputFormatAll(c *tc.C) {
	for i, t := range configGetAllTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, "config-get")
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Assert(code, tc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), tc.Equals, "")

		out := map[string]interface{}{}
		switch t.format {
		case formatYaml:
			c.Assert(goyaml.Unmarshal(bufferBytes(ctx.Stdout), &out), tc.IsNil)
		case formatJson:
			c.Assert(json.Unmarshal(bufferBytes(ctx.Stdout), &out), tc.IsNil)
		}
		c.Assert(out, tc.DeepEquals, t.out)
	}
}

func (s *ConfigGetSuite) TestOutputPath(c *tc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "config-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--output", "some-file", "monsters"})
	c.Assert(code, tc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, "")
	content, err := os.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(content), tc.Equals, "False\n")
}

func (s *ConfigGetSuite) TestUnknownArg(c *tc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "config-get")
	c.Assert(err, tc.ErrorIsNil)
	cmdtesting.TestInit(c, jujuc.NewJujucCommandWrappedForTest(com), []string{"multiple", "keys"}, `unrecognized args: \["keys"\]`)
}

func (s *ConfigGetSuite) TestAllPlusKey(c *tc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "config-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--all", "--format", "json", "monsters"})
	c.Assert(code, tc.Equals, 2)
	c.Assert(bufferString(ctx.Stderr), tc.Equals, "ERROR cannot use argument --all together with key \"monsters\"\n")
}
