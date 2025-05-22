// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/jujuctesting"
)

type ActionGetSuite struct {
	jujuctesting.ContextSuite
}

func TestActionGetSuite(t *testing.T) {
	tc.Run(t, &ActionGetSuite{})
}

type actionGetContext struct {
	actionParams map[string]interface{}
	jujuc.Context
}

func (ctx *actionGetContext) ActionParams() (map[string]interface{}, error) {
	return ctx.actionParams, nil
}

type nonActionContext struct {
	jujuc.Context
}

func (ctx *nonActionContext) ActionParams() (map[string]interface{}, error) {
	return nil, fmt.Errorf("ActionParams queried from non-Action hook context")
}

func (s *ActionGetSuite) TestNonActionRunFail(c *tc.C) {
	hctx := &nonActionContext{}
	com, err := jujuc.NewCommand(hctx, "action-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{})
	c.Check(code, tc.Equals, 1)
	c.Check(bufferString(ctx.Stdout), tc.Equals, "")
	expect := fmt.Sprintf(`(\n)*ERROR %s\n`, "ActionParams queried from non-Action hook context")
	c.Check(bufferString(ctx.Stderr), tc.Matches, expect)
}

func (s *ActionGetSuite) TestActionGet(c *tc.C) {
	var actionGetTestMaps = []map[string]interface{}{
		{
			"outfile": "foo.bz2",
		},

		{
			"outfile": map[string]interface{}{
				"filename": "foo.bz2",
				"format":   "bzip",
			},
		},

		{
			"outfile": map[string]interface{}{
				"type": map[string]interface{}{
					"1": "raw",
					"2": "gzip",
					"3": "bzip",
				},
			},
		},

		// A map with a non-string key is not usable.
		{
			"outfile": map[interface{}]interface{}{
				5: map[string]interface{}{
					"1": "raw",
					"2": "gzip",
					"3": "bzip",
				},
			},
		},

		// A map with an inner map[interface{}]interface{} is OK if
		// the keys are strings.
		{
			"outfile": map[interface{}]interface{}{
				"type": map[string]interface{}{
					"1": "raw",
					"2": "gzip",
					"3": "bzip",
				},
			},
		},
	}

	var actionGetTests = []struct {
		summary      string
		args         []string
		actionParams map[string]interface{}
		code         int
		out          string
		errMsg       string
	}{{
		summary: "a simple empty map with nil key",
		args:    []string{},
		out:     "{}\n",
	}, {
		summary: "a simple empty map with nil key",
		args:    []string{"--format", "yaml"},
		out:     "{}\n",
	}, {
		summary: "a simple empty map with nil key",
		args:    []string{"--format", "json"},
		out:     "{}\n",
	}, {
		summary: "a nonexistent key",
		args:    []string{"foo"},
	}, {
		summary: "a nonexistent key",
		args:    []string{"--format", "yaml", "foo"},
	}, {
		summary: "a nonexistent key",
		args:    []string{"--format", "json", "foo"},
		out:     "null\n",
	}, {
		summary:      "a nonexistent inner key",
		args:         []string{"outfile.type"},
		actionParams: actionGetTestMaps[1],
	}, {
		summary:      "a nonexistent inner key",
		args:         []string{"--format", "yaml", "outfile.type"},
		actionParams: actionGetTestMaps[1],
	}, {
		summary:      "a nonexistent inner key",
		args:         []string{"--format", "json", "outfile.type"},
		actionParams: actionGetTestMaps[1],
		out:          "null\n",
	}, {
		summary:      "a nonexistent inner key",
		args:         []string{"outfile.type.1"},
		actionParams: actionGetTestMaps[1],
	}, {
		summary:      "a nonexistent inner key",
		args:         []string{"--format", "yaml", "outfile.type.1"},
		actionParams: actionGetTestMaps[1],
	}, {
		summary:      "a nonexistent inner key",
		args:         []string{"--format", "json", "outfile.type.1"},
		actionParams: actionGetTestMaps[1],
		out:          "null\n",
	}, {
		summary:      "a map with a non-string key",
		args:         []string{"outfile.type"},
		actionParams: actionGetTestMaps[3],
	}, {
		summary:      "a map with a non-string key",
		args:         []string{"--format", "yaml", "outfile.type"},
		actionParams: actionGetTestMaps[3],
	}, {
		summary:      "a map with a non-string key",
		args:         []string{"--format", "json", "outfile.type"},
		actionParams: actionGetTestMaps[3],
		out:          "null\n",
	}, {
		summary:      "a simple map of one value to one key",
		args:         []string{},
		actionParams: actionGetTestMaps[0],
		out:          "outfile: foo.bz2\n",
	}, {
		summary:      "a simple map of one value to one key",
		args:         []string{"--format", "yaml"},
		actionParams: actionGetTestMaps[0],
		out:          "outfile: foo.bz2\n",
	}, {
		summary:      "a simple map of one value to one key",
		args:         []string{"--format", "json"},
		actionParams: actionGetTestMaps[0],
		out:          `{"outfile":"foo.bz2"}` + "\n",
	}, {
		summary:      "an entire map",
		args:         []string{},
		actionParams: actionGetTestMaps[2],
		out: "outfile:\n" +
			"  type:\n" +
			"    \"1\": raw\n" +
			"    \"2\": gzip\n" +
			"    \"3\": bzip\n",
	}, {
		summary:      "an entire map",
		args:         []string{"--format", "yaml"},
		actionParams: actionGetTestMaps[2],
		out: "outfile:\n" +
			"  type:\n" +
			"    \"1\": raw\n" +
			"    \"2\": gzip\n" +
			"    \"3\": bzip\n",
	}, {
		summary:      "an entire map",
		args:         []string{"--format", "json"},
		actionParams: actionGetTestMaps[2],
		out:          `{"outfile":{"type":{"1":"raw","2":"gzip","3":"bzip"}}}` + "\n",
	}, {
		summary:      "an inner map value which is itself a map",
		args:         []string{"outfile.type"},
		actionParams: actionGetTestMaps[2],
		out: "\"1\": raw\n" +
			"\"2\": gzip\n" +
			"\"3\": bzip\n",
	}, {
		summary:      "an inner map value which is itself a map",
		args:         []string{"--format", "yaml", "outfile.type"},
		actionParams: actionGetTestMaps[2],
		out: "\"1\": raw\n" +
			"\"2\": gzip\n" +
			"\"3\": bzip\n",
	}, {
		summary:      "an inner map value which is itself a map",
		args:         []string{"--format", "json", "outfile.type"},
		actionParams: actionGetTestMaps[2],
		out:          `{"1":"raw","2":"gzip","3":"bzip"}` + "\n",
	}, {
		summary:      "a map with an inner map keyed by interface{}",
		args:         []string{"outfile.type"},
		actionParams: actionGetTestMaps[4],
		out: "\"1\": raw\n" +
			"\"2\": gzip\n" +
			"\"3\": bzip\n",
	}, {
		summary:      "a map with an inner map keyed by interface{}",
		args:         []string{"--format", "yaml", "outfile.type"},
		actionParams: actionGetTestMaps[4],
		out: "\"1\": raw\n" +
			"\"2\": gzip\n" +
			"\"3\": bzip\n",
	}, {
		summary:      "a map with an inner map keyed by interface{}",
		args:         []string{"--format", "json", "outfile.type"},
		actionParams: actionGetTestMaps[4],
		out:          `{"1":"raw","2":"gzip","3":"bzip"}` + "\n",
	}, {
		summary: "too many arguments",
		args:    []string{"multiple", "keys"},
		code:    2,
		errMsg:  `unrecognized args: \["keys"\]`,
	}}

	for i, t := range actionGetTests {
		c.Logf("test %d: %s\n args: %#v", i, t.summary, t.args)
		hctx := &actionGetContext{}
		hctx.actionParams = t.actionParams
		com, err := jujuc.NewCommand(hctx, "action-get")
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Check(code, tc.Equals, t.code)
		if code == 0 {
			c.Check(bufferString(ctx.Stdout), tc.Equals, t.out)
			c.Check(bufferString(ctx.Stderr), tc.Equals, "")
		} else {
			c.Check(bufferString(ctx.Stdout), tc.Equals, "")
			expect := fmt.Sprintf(`(\n)*ERROR %s\n`, t.errMsg)
			c.Check(bufferString(ctx.Stderr), tc.Matches, expect)
		}
	}
}
