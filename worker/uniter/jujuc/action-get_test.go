// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"

	"github.com/juju/cmd"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/jujuc"
)

type ActionGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&ActionGetSuite{})

func (s *ActionGetSuite) TestActionGet(c *gc.C) {
	var actionGetTestMaps = []map[string]interface{}{
		map[string]interface{}{
			"outfile": "foo.bz2",
		},

		map[string]interface{}{
			"outfile": map[string]interface{}{
				"filename": "foo.bz2",
				"format":   "bzip",
			},
		},

		map[string]interface{}{
			"outfile": map[string]interface{}{
				"type": map[string]interface{}{
					"1": "raw",
					"2": "gzip",
					"3": "bzip",
				},
			},
		},

		// A map with a non-string key is not usable.
		map[string]interface{}{
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
		map[string]interface{}{
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
		out:     "null\n",
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
		out:          "{\"outfile\":\"foo.bz2\"}\n",
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
		hctx := s.GetHookContext(c, -1, "")
		hctx.actionParams = t.actionParams

		com, err := jujuc.NewCommand(hctx, "action-get")
		c.Assert(err, gc.IsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Check(code, gc.Equals, t.code)
		if code == 0 {
			c.Check(bufferString(ctx.Stderr), gc.Equals, "")
			c.Check(bufferString(ctx.Stdout), gc.Equals, t.out)
		} else {
			c.Check(bufferString(ctx.Stdout), gc.Equals, "")
			expect := fmt.Sprintf(`(.|\n)*error: %s\n`, t.errMsg)
			c.Check(bufferString(ctx.Stderr), gc.Matches, expect)
		}
	}
}

func (s *ActionGetSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "action-get")
	c.Assert(err, gc.IsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `usage: action-get [options] [<key>[.<key>.<key>...]]
purpose: get action parameters

options:
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file

action-get will print the value of the parameter at the given key, serialized
as YAML.  If multiple keys are passed, action-get will recurse into the param
map as needed.
`)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}
