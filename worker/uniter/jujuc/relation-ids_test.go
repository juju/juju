// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/jujuc"
)

type RelationIdsSuite struct {
	ContextSuite
}

var _ = gc.Suite(&RelationIdsSuite{})

func (s *RelationIdsSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)
	s.rels = map[int]*ContextRelation{}
	s.AddRelatedServices(c, "x", 3)
	s.AddRelatedServices(c, "y", 1)
}

func (s *RelationIdsSuite) AddRelatedServices(c *gc.C, relname string, count int) {
	for i := 0; i < count; i++ {
		id := len(s.rels)
		s.rels[id] = &ContextRelation{id, relname, nil}
	}
}

var relationIdsTests = []struct {
	summary string
	relid   int
	args    []string
	code    int
	out     string
}{
	{
		summary: "no default, no name",
		relid:   -1,
		code:    2,
		out:     "(.|\n)*error: no relation name specified\n",
	}, {
		summary: "default name",
		relid:   1,
		out:     "x:0\nx:1\nx:2",
	}, {
		summary: "explicit name",
		relid:   -1,
		args:    []string{"x"},
		out:     "x:0\nx:1\nx:2",
	}, {
		summary: "explicit different name",
		relid:   -1,
		args:    []string{"y"},
		out:     "y:3",
	}, {
		summary: "nonexistent name",
		relid:   -1,
		args:    []string{"z"},
	}, {
		summary: "explicit smart formatting 1",
		args:    []string{"--format", "smart", "x"},
		out:     "x:0\nx:1\nx:2",
	}, {
		summary: "explicit smart formatting 2",
		args:    []string{"--format", "smart", "y"},
		out:     "y:3",
	}, {
		summary: "explicit smart formatting 3",
		args:    []string{"--format", "smart", "z"},
	}, {
		summary: "json formatting 1",
		args:    []string{"--format", "json", "x"},
		out:     `["x:0","x:1","x:2"]`,
	}, {
		summary: "json formatting 2",
		args:    []string{"--format", "json", "y"},
		out:     `["y:3"]`,
	}, {
		summary: "json formatting 3",
		args:    []string{"--format", "json", "z"},
		out:     `[]`,
	}, {
		summary: "yaml formatting 1",
		args:    []string{"--format", "yaml", "x"},
		out:     "- x:0\n- x:1\n- x:2",
	}, {
		summary: "yaml formatting 2",
		args:    []string{"--format", "yaml", "y"},
		out:     "- y:3",
	}, {
		summary: "yaml formatting 3",
		args:    []string{"--format", "yaml", "z"},
		out:     "[]",
	},
}

func (s *RelationIdsSuite) TestRelationIds(c *gc.C) {
	for i, t := range relationIdsTests {
		c.Logf("test %d: %s", i, t.summary)
		hctx := s.GetHookContext(c, t.relid, "")
		com, err := jujuc.NewCommand(hctx, "relation-ids")
		c.Assert(err, gc.IsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, gc.Equals, t.code)
		if code == 0 {
			c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
			expect := t.out
			if expect != "" {
				expect += "\n"
			}
			c.Assert(bufferString(ctx.Stdout), gc.Equals, expect)
		} else {
			c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
			c.Assert(bufferString(ctx.Stderr), gc.Matches, t.out)
		}
	}
}

func (s *RelationIdsSuite) TestHelp(c *gc.C) {
	template := `
usage: %s
purpose: list all relation ids with the given relation name

options:
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file
%s`[1:]

	for relid, t := range map[int]struct {
		usage, doc string
	}{
		-1: {"relation-ids [options] <name>", ""},
		0:  {"relation-ids [options] [<name>]", "\nCurrent default relation name is \"x\".\n"},
		3:  {"relation-ids [options] [<name>]", "\nCurrent default relation name is \"y\".\n"},
	} {
		c.Logf("relid %d", relid)
		hctx := s.GetHookContext(c, relid, "")
		com, err := jujuc.NewCommand(hctx, "relation-ids")
		c.Assert(err, gc.IsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, []string{"--help"})
		c.Assert(code, gc.Equals, 0)
		expect := fmt.Sprintf(template, t.usage, t.doc)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, expect)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	}
}
