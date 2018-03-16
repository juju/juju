// Copyright 2018 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type GoalStateSuite struct {
	ContextSuite
}

var _ = gc.Suite(&GoalStateSuite{})

func getGoalStateCommand(s *GoalStateSuite, c *gc.C, args []string) (*cmd.Context, int) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, cmdString("goal-state"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, args)
	return ctx, code
}

//var (
//	goalStateYamlMap = map[string]interface{}{
//		"tag":    "",
//		"status": "error",
//		"info":   "message foo",
//		"data":   map[interface{}]interface{}{"foo": "bar"},
//	}
//	goalStateJsonMap = map[string]interface{}{
//		"Tag":    "",
//		"Status": "error",
//		"Info":   "message foo",
//		"Data":   map[string]interface{}{"foo": "bar"},
//	}
//	goalStateYamlMapAll = map[string]interface{}{
//		"tag":    "",
//		"status": "error",
//		"info":   "message foo",
//		"data":   map[interface{}]interface{}{"foo": "bar"},
//	}
//	goalStateJsonMapAll = map[string]interface{}{
//		"Tag":    "",
//		"Status": "error",
//		"Info":   "message foo",
//		"Data":   map[string]interface{}{"foo": "bar"},
//	}
//)

//var goalStateAllTests = []struct {
//	args   []string
//	format int
//	out    map[string]interface{}
//}{
//	{nil, formatYaml, goalStateYamlMap},
//	{[]string{"--format", "yaml"}, formatYaml, goalStateYamlMap},
//	{[]string{"--format", "json"}, formatJson, goalStateJsonMap},
//	{[]string{"--all", "--format", "yaml"}, formatYaml, goalStateYamlMapAll},
//	{[]string{"--all", "--format", "json"}, formatJson, goalStateJsonMapAll},
//	{[]string{"-a", "--format", "yaml"}, formatYaml, goalStateYamlMapAll},
//	{[]string{"-a", "--format", "json"}, formatJson, goalStateJsonMapAll},
//}

// TODO (agprado): Parse yaml and json in the next PR

//var goalStateAllTests = []struct {
//	args   []string
//	format int
//	out    string
//}{
//	{nil, formatYaml, "foo"},
//	{[]string{"--format", "yaml"}, formatYaml, "bar"},
//	{[]string{"--format", "json"}, formatJson, "foo"},
//	{[]string{"--all", "--format", "yaml"}, formatYaml, "foo"},
//	{[]string{"--all", "--format", "json"}, formatJson, "bar"},
//	{[]string{"-a", "--format", "yaml"}, formatYaml, "foo"},
//	{[]string{"-a", "--format", "json"}, formatJson, "bar"},
//}
//func (s *GoalStateSuite) TestOutputFormatAll(c *gc.C) {
//	for i, t := range goalStateAllTests {
//		c.Logf("test %d: %#v", i, t.args)
//
//		ctx, code := getGoalStateCommand(s, c, t.args)
//
//		c.Assert(code, gc.Equals, 0)
//		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
//		out := ctx.Stdout
//		//switch t.format {
//		//case formatYaml:
//		//	c.Assert(goyaml.Unmarshal(bufferBytes(ctx.Stdout), &out), gc.IsNil)
//		//case formatJson:
//		//	c.Assert(json.Unmarshal(bufferBytes(ctx.Stdout), &out), gc.IsNil)
//		//}
//		c.Assert(out, gc.DeepEquals, t.out)
//	}
//}

func (s *GoalStateSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, cmdString("goal-state"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `Usage: goal-state [options]

Summary:
print the status of the charm's peers and related units

Options:
--format  (= yaml)
    Specify output format (json|yaml)
-o, --output (= "")
    Specify an output file

Details:
'goal-state' command will list the charm units and relations, specifying their status and their relations to other units in different charms.
`)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *GoalStateSuite) TestOutputPath(c *gc.C) {

	args := []string{"--output", "some-file", "monsters"}
	ctx, code := getGoalStateCommand(s, c, args)

	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")

	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "test-goal-state\n")
}

func (s *GoalStateSuite) TestUnknownArg(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, cmdString("goal-state"))
	c.Assert(err, jc.ErrorIsNil)
	cmdtesting.TestInit(c, com, []string{"multiple", "keys"}, `unrecognized args: \["keys"\]`)
}
