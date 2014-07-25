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

// TODO(binary132):
// action-set - <YAML>

func (s *ActionSetSuite) TestActionSet(c *gc.C) {
	var actionSetTests = []struct {
		summary string
		args    []string
		values  []string
		results map[string]interface{}
		fail    bool
		code    int
		errMsg  string
	}{{
		summary: "a single bare value",
		args:    []string{},
		values:  []string{"result"},
		results: map[string]interface{}{
			"val0": "result",
		},
	}, {
		summary: "two bare values",
		args:    []string{},
		values:  []string{"result", "other"},
		results: map[string]interface{}{
			"val0": "result",
			"val1": "other",
		},
	}, {
		summary: "a response of one key to one value",
		args:    []string{},
		values:  []string{"outfile=foo.bz2"},
		results: map[string]interface{}{
			"outfile": "foo.bz2",
		},
	}, {
		summary: "two keys, two values",
		args:    []string{},
		values:  []string{"outfile=foo.bz2", "size=10G"},
		results: map[string]interface{}{
			"outfile": "foo.bz2",
			"size":    "10G",
		},
	}, {
		summary: "multiple = are ok",
		args:    []string{},
		values:  []string{"outfile=foo=bz2"},
		results: map[string]interface{}{
			"outfile": "foo=bz2",
		},
	}, {
		summary: "several interleaved values",
		args:    []string{},
		values: []string{"outfile.name=foo.bz2",
			"outfile.kind.util=bzip2",
			"outfile.kind.ratio=high"},
		results: map[string]interface{}{
			"outfile": map[string]interface{}{
				"name": "foo.bz2",
				"kind": map[string]interface{}{
					"util":  "bzip2",
					"ratio": "high",
				},
			},
		},
	}}

	for i, t := range actionSetTests {
		c.Logf("test %d: %s\n args: %#v", i, t.summary, t.args)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, "action-set")
		c.Assert(err, gc.IsNil)
		ctx := testing.Context(c)
		allArgs := append(t.args, t.values...)
		code := cmd.Main(com, ctx, allArgs)
		c.Check(code, gc.Equals, t.code)
		c.Check(bufferString(ctx.Stderr), gc.Equals, "")
		results, failed := hctx.ActionResults()
		c.Check(results, jc.DeepEquals, t.results)
		c.Check(failed, gc.Equals, t.fail)
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
