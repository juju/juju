// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"strings"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type RelationIdsSuite struct {
	relationSuite
}

var _ = tc.Suite(&RelationIdsSuite{})

func (s *RelationIdsSuite) newHookContext(relid int, remote string) (jujuc.Context, *relationInfo) {
	hctx, info := s.relationSuite.newHookContext(-1, "", "")
	info.reset()
	info.addRelatedApplications("x", 3)
	info.addRelatedApplications("y", 1)
	if relid >= 0 {
		info.SetAsRelationHook(relid, remote)
	}
	return hctx, info
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
		out:     "(.|\n)*ERROR no endpoint name specified\n",
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

func (s *RelationIdsSuite) TestRelationIds(c *tc.C) {
	for i, t := range relationIdsTests {
		c.Logf("test %d: %s", i, t.summary)
		hctx, _ := s.newHookContext(t.relid, "")
		com, err := jujuc.NewCommand(hctx, "relation-ids")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Assert(code, tc.Equals, t.code)
		if code == 0 {
			c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
			expect := t.out
			if expect != "" {
				expect += "\n"
			}
			c.Assert(bufferString(ctx.Stdout), tc.Equals, expect)
		} else {
			c.Assert(bufferString(ctx.Stdout), tc.Equals, "")
			c.Assert(bufferString(ctx.Stderr), tc.Matches, t.out)
		}
	}
}

func (s *RelationIdsSuite) TestHelp(c *tc.C) {
	for relid, t := range map[int]struct {
		usage, doc string
	}{
		-1: {"relation-ids [options] <name>", "\nDetails:\nOnly relation ids for relations which are not broken are included.\n"},
		0:  {"relation-ids [options] [<name>]", "\nDetails:\nCurrent default endpoint name is \"x\".\nOnly relation ids for relations which are not broken are included.\n"},
		3:  {"relation-ids [options] [<name>]", "\nDetails:\nCurrent default endpoint name is \"y\".\nOnly relation ids for relations which are not broken are included.\n"},
	} {
		c.Logf("relid %d", relid)
		hctx, _ := s.newHookContext(relid, "")
		com, err := jujuc.NewCommand(hctx, "relation-ids")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--help"})
		c.Assert(code, tc.Equals, 0)
		c.Assert(strings.Contains(bufferString(ctx.Stdout), t.usage), jc.IsTrue)
	}
}

func (s *RelationIdsSuite) TestFilterNonLiveRelations(c *tc.C) {
	hctx, info := s.newHookContext(1, "")

	// Relations starts as alive so they all will end up in the output.
	exp := "x:0\nx:1\nx:2\n"
	s.assertOutputMatches(c, hctx, exp)

	// Change relation 1 life to dying and ensure it still shows up in the output
	info.rels[1].Life = life.Dying
	exp = "x:0\nx:1\nx:2\n"
	s.assertOutputMatches(c, hctx, exp)

	// Change relation 0 life to dead and ensure it doesn't show up in the output
	info.rels[0].Life = life.Dead
	exp = "x:1\nx:2\n"
	s.assertOutputMatches(c, hctx, exp)
}

func (s *RelationIdsSuite) assertOutputMatches(c *tc.C, hctx jujuc.Context, expOutput string) {
	com, err := jujuc.NewCommand(hctx, "relation-ids")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, nil)
	c.Assert(code, tc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, expOutput)
}
