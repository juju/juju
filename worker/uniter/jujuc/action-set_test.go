// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/jujuc"
)

type ActionSetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&ActionSetSuite{})

func (s *ActionSetSuite) TestGoodActionSet(c *gc.C) {
	var actionSetTests = []struct {
		summary  string
		commands [][]string
		results  map[string]interface{}
	}{{
		summary: "a single bare value",
		commands: [][]string{
			[]string{"result"},
		},
		results: map[string]interface{}{
			"val0": "result",
		},
	}, {
		summary: "a single bare string",
		commands: [][]string{
			[]string{"result is a string"},
		},
		results: map[string]interface{}{
			"val0": `result is a string`,
		},
	}, {
		summary: "two bare values",
		commands: [][]string{
			[]string{"result", "other"},
		},
		results: map[string]interface{}{
			"val0": "result",
			"val1": "other",
		},
	}, {
		summary: "a response of one key to one value",
		commands: [][]string{
			[]string{"outfile=foo.bz2"},
		},
		results: map[string]interface{}{
			"outfile": "foo.bz2",
		},
	}, {
		summary: "two keys, two values",
		commands: [][]string{
			[]string{"outfile=foo.bz2", "size=10G"},
		},
		results: map[string]interface{}{
			"outfile": "foo.bz2",
			"size":    "10G",
		},
	}, {
		summary: "multiple = are ok",
		commands: [][]string{
			[]string{"outfile=foo=bz2"},
		},
		results: map[string]interface{}{
			"outfile": "foo=bz2",
		},
	}, {
		summary: "several interleaved values",
		commands: [][]string{
			[]string{"outfile.name=foo.bz2",
				"outfile.kind.util=bzip2",
				"outfile.kind.ratio=high"},
		},
		results: map[string]interface{}{
			"outfile": map[string]interface{}{
				"name": "foo.bz2",
				"kind": map[string]interface{}{
					"util":  "bzip2",
					"ratio": "high",
				},
			},
		},
	}, {
		summary: "conflicting simple values in one command result in overwrite",
		commands: [][]string{
			[]string{"util=bzip2", "util=5"},
		},
		results: map[string]interface{}{
			"util": 5,
		},
	}, {
		summary: "conflicting simple values in two commands results in overwrite",
		commands: [][]string{
			[]string{"util=bzip2"},
			[]string{"util=5"},
		},
		results: map[string]interface{}{
			"util": 5,
		},
	}, {
		summary: "conflicted map spec: {map1:{key:val}} vs {map1:val2}",
		commands: [][]string{
			[]string{"map1.key=val", "map1=val"},
		},
		results: map[string]interface{}{
			"map1": "val",
		},
	}, {
		summary: "two-invocation conflicted map spec: {map1:{key:val}} vs {map1:val2}",
		commands: [][]string{
			[]string{"map1.key=val"},
			[]string{"map1=val"},
		},
		results: map[string]interface{}{
			"map1": "val",
		},
	}, {
		summary: "conflicted map spec: {map1:val2} vs {map1:{key:val}}",
		commands: [][]string{
			[]string{"map1=val", "map1.key=val"},
		},
		results: map[string]interface{}{
			"map1": map[string]interface{}{
				"key": "val",
			},
		},
	}, {
		summary: "two-invocation conflicted map spec: {map1:val2} vs {map1:{key:val}}",
		commands: [][]string{
			[]string{"map1=val"},
			[]string{"map1.key=val"},
		},
		results: map[string]interface{}{
			"map1": map[string]interface{}{
				"key": "val",
			},
		},
	}}

	for i, t := range actionSetTests {
		c.Logf("test %d: %s", i, t.summary)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, "action-set")
		c.Assert(err, gc.IsNil)
		ctx := testing.Context(c)
		for j, command := range t.commands {
			c.Logf("  command %d: %#v", j, command)
			code := cmd.Main(com, ctx, command)
			c.Check(code, gc.Equals, 0)
			c.Check(bufferString(ctx.Stderr), gc.Equals, "")
			_, failed := hctx.ActionResults()
			c.Check(failed, gc.Equals, false)
		}
		results, _ := hctx.ActionResults()
		c.Check(results, jc.DeepEquals, t.results)
	}
}

func (s *ActionSetSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "action-set")
	c.Assert(err, gc.IsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `usage: action-set <values>
purpose: set action response

action-set commits the given map as the return value of the Action.  If a
bare value is given, it will be converted to a map.  This value will be
returned to the stateservice and client after completion of the Action.
Subsequent calls to action-set before completion of the Action will add the
values to the map, unless there is a conflict in which case the new value
will overwrite the old value.

Example usage:
 action-set outfile.size=10G
 action-set foo
 action-set foo.bar.baz=2 foo.bar.zab="3"
`)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}
