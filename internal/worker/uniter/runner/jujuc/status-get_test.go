// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type statusGetSuite struct {
	ContextSuite
}

func TestStatusGetSuite(t *stdtesting.T) {
	tc.Run(t, &statusGetSuite{})
}

func (s *statusGetSuite) SetUpTest(c *tc.C) {
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
}

func setFakeStatus(c *tc.C, ctx *Context) {
	ctx.SetUnitStatus(c.Context(), jujuc.StatusInfo{
		Status: statusAttributes["status"].(string),
		Info:   statusAttributes["message"].(string),
		Data:   statusAttributes["status-data"].(map[string]interface{}),
	})
}

func setFakeApplicationStatus(ctx *Context) {
	ctx.info.Status.SetApplicationStatus(
		jujuc.StatusInfo{
			Status: "active",
			Info:   "this is a application status",
			Data:   nil,
		},
		[]jujuc.StatusInfo{{
			Status: "active",
			Info:   "this is a unit status",
			Data:   nil,
		}},
	)
}

func (s *statusGetSuite) TestOutputFormatJustStatus(c *tc.C) {
	for i, t := range statusGetTests {
		c.Logf("test %d: %#v", i, t.args)
		hctx := s.GetStatusHookContext(c)
		setFakeStatus(c, hctx)
		com, err := jujuc.NewCommand(hctx, "status-get")
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Assert(code, tc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), tc.Equals, "")

		var out interface{}
		var outMap map[string]interface{}
		switch t.format {
		case formatYaml:
			c.Check(goyaml.Unmarshal(bufferBytes(ctx.Stdout), &outMap), tc.IsNil)
			out = outMap
		case formatJson:
			c.Check(json.Unmarshal(bufferBytes(ctx.Stdout), &outMap), tc.IsNil)
			out = outMap
		default:
			out = string(bufferBytes(ctx.Stdout))
		}
		c.Check(out, tc.DeepEquals, t.out)
	}
}

func (s *statusGetSuite) TestOutputPath(c *tc.C) {
	hctx := s.GetStatusHookContext(c)
	setFakeStatus(c, hctx)
	com, err := jujuc.NewCommand(hctx, "status-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--format", "json", "--output", "some-file", "--include-data"})
	c.Assert(code, tc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, "")
	content, err := os.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, tc.ErrorIsNil)

	var out map[string]interface{}
	c.Assert(json.Unmarshal(content, &out), tc.IsNil)
	c.Assert(out, tc.DeepEquals, statusAttributes)
}

func (s *statusGetSuite) TestApplicationStatus(c *tc.C) {
	expected := map[string]interface{}{
		"application-status": map[interface{}]interface{}{
			"status-data": map[interface{}]interface{}{},
			"units": map[interface{}]interface{}{
				"": map[interface{}]interface{}{
					"message":     "this is a unit status",
					"status":      "active",
					"status-data": map[interface{}]interface{}{},
				},
			},
			"message": "this is a application status",
			"status":  "active"},
	}
	hctx := s.GetStatusHookContext(c)
	setFakeApplicationStatus(hctx)
	com, err := jujuc.NewCommand(hctx, "status-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--format", "json", "--include-data", "--application"})
	c.Assert(code, tc.Equals, 0)

	var out map[string]interface{}
	c.Assert(goyaml.Unmarshal(bufferBytes(ctx.Stdout), &out), tc.IsNil)
	c.Assert(out, tc.DeepEquals, expected)

}
