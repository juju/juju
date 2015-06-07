// Copyright 2015 Canonical Ltd.
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

type statusGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&statusGetSuite{})

func (s *statusGetSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)
}

var (
	statusAttributes = map[string]interface{}{
		"status":      "error",
		"message":     "doing work",
		"status-data": map[string]interface{}{"foo": "bar"},
	}
)

var statusGetTests = []struct {
	args   []string
	format int
	out    interface{}
}{
	{[]string{"--format", "json", "--include-data"}, formatJson, statusAttributes},
	{[]string{"--format", "yaml"}, formatYaml, map[string]interface{}{"status": "error"}},
	{[]string{}, -1, "error\n"},
}

func setFakeStatus(ctx *Context) {
	ctx.SetUnitStatus(jujuc.StatusInfo{
		Status: statusAttributes["status"].(string),
		Info:   statusAttributes["message"].(string),
		Data:   statusAttributes["status-data"].(map[string]interface{}),
	})
}

func setFakeServiceStatus(ctx *Context) {
	ctx.info.Status.SetServiceStatus(
		jujuc.StatusInfo{
			Status: "active",
			Info:   "this is a service status",
			Data:   nil,
		},
		[]jujuc.StatusInfo{{
			Status: "active",
			Info:   "this is a unit status",
			Data:   nil,
		}},
	)
}

func (s *statusGetSuite) TestOutputFormatJustStatus(c *gc.C) {
	for i, t := range statusGetTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetStatusHookContext(c)
		setFakeStatus(hctx)
		com, err := jujuc.NewCommand(hctx, cmdString("status-get"))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")

		var out interface{}
		var outMap map[string]interface{}
		switch t.format {
		case formatYaml:
			c.Check(goyaml.Unmarshal(bufferBytes(ctx.Stdout), &outMap), gc.IsNil)
			out = outMap
		case formatJson:
			c.Check(json.Unmarshal(bufferBytes(ctx.Stdout), &outMap), gc.IsNil)
			out = outMap
		default:
			out = string(bufferBytes(ctx.Stdout))
		}
		c.Check(out, gc.DeepEquals, t.out)
	}
}

func (s *statusGetSuite) TestHelp(c *gc.C) {
	hctx := s.GetStatusHookContext(c)
	com, err := jujuc.NewCommand(hctx, cmdString("status-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	expectedHelp := "" +
		"usage: status-get [options] [--include-data] [--service]\n" +
		"purpose: print status information\n" +
		"\n" +
		"options:\n" +
		"--format  (= smart)\n" +
		"    specify output format (json|smart|yaml)\n" +
		"--include-data  (= false)\n" +
		"    print all status data\n" +
		"-o, --output (= \"\")\n" +
		"    specify an output file\n" +
		"--service  (= false)\n" +
		"    print status for all units of this service if this unit is the leader\n" +
		"\n" +
		"By default, only the status value is printed.\n" +
		"If the --include-data flag is passed, the associated data are printed also.\n"

	c.Assert(bufferString(ctx.Stdout), gc.Equals, expectedHelp)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *statusGetSuite) TestOutputPath(c *gc.C) {
	hctx := s.GetStatusHookContext(c)
	setFakeStatus(hctx)
	com, err := jujuc.NewCommand(hctx, cmdString("status-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--format", "json", "--output", "some-file", "--include-data"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, jc.ErrorIsNil)

	var out map[string]interface{}
	c.Assert(json.Unmarshal(content, &out), gc.IsNil)
	c.Assert(out, gc.DeepEquals, statusAttributes)
}

func (s *statusGetSuite) TestServiceStatus(c *gc.C) {
	expected := map[string]interface{}{
		"service-status": map[interface{}]interface{}{
			"status-data": map[interface{}]interface{}{},
			"units": map[interface{}]interface{}{
				"": map[interface{}]interface{}{
					"message":     "this is a unit status",
					"status":      "active",
					"status-data": map[interface{}]interface{}{},
				},
			},
			"message": "this is a service status",
			"status":  "active"},
	}
	hctx := s.GetStatusHookContext(c)
	setFakeServiceStatus(hctx)
	com, err := jujuc.NewCommand(hctx, cmdString("status-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--format", "json", "--include-data", "--service"})
	c.Assert(code, gc.Equals, 0)

	var out map[string]interface{}
	c.Assert(goyaml.Unmarshal(bufferBytes(ctx.Stdout), &out), gc.IsNil)
	c.Assert(out, gc.DeepEquals, expected)

}
