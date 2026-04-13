// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/tc"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type statusGetSuite struct {
	ContextSuite
}

func TestStatusGetSuite(t *testing.T) {
	tc.Run(t, &statusGetSuite{})
}

func (s *statusGetSuite) SetUpTest(c *tc.C) {
	s.ContextSuite.SetUpTest(c)
}

var (
	statusAttributes = map[string]any{
		"status":      "error",
		"message":     "doing work",
		"status-data": map[string]any{"foo": "bar"},
	}
)

var statusGetTests = []struct {
	args   []string
	format int
	out    any
}{
	{[]string{"--format", "json", "--include-data"}, formatJson, statusAttributes},
	{[]string{"--format", "yaml"}, formatYaml, map[string]any{"status": "error"}},
}

func setFakeStatus(c *tc.C, ctx *Context) {
	ctx.SetUnitStatus(c.Context(), jujuc.StatusInfo{
		Status: statusAttributes["status"].(string),
		Info:   statusAttributes["message"].(string),
		Data:   statusAttributes["status-data"].(map[string]any),
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

		var out any
		var outMap map[string]any
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

	var out map[string]any
	c.Assert(json.Unmarshal(content, &out), tc.IsNil)
	c.Assert(out, tc.DeepEquals, statusAttributes)
}

func (s *statusGetSuite) TestApplicationStatus(c *tc.C) {
	expected := map[string]any{
		"application-status": map[any]any{
			"status-data": map[any]any{},
			"units": map[any]any{
				"": map[any]any{
					"message":     "this is a unit status",
					"status":      "active",
					"status-data": map[any]any{},
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

	var out map[string]any
	c.Assert(goyaml.Unmarshal(bufferBytes(ctx.Stdout), &out), tc.IsNil)
	c.Assert(out, tc.DeepEquals, expected)

}
