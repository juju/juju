// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type RelationModelGetSuite struct {
	relationSuite
}

func TestRelationModelGetSuite(t *stdtesting.T) { tc.Run(t, &RelationModelGetSuite{}) }

type relationModelGetInitTest struct {
	ctxrelid int
	relid    int
	args     []string
	err      string
}

var relationModelGetInitTests = []relationModelGetInitTest{
	{
		// compatibility: 0 args is valid.
	}, {
		ctxrelid: -1,
		err:      `no relation id specified`,
	}, {
		ctxrelid: -1,
		args:     []string{"-r", "one"},
		err:      `invalid value "one" for option -r: invalid relation id`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "one"},
		err:      `invalid value "one" for option -r: invalid relation id`,
	}, {
		ctxrelid: -1,
		args:     []string{"-r", "ignored:one"},
		err:      `invalid value "ignored:one" for option -r: invalid relation id`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "ignored:one"},
		err:      `invalid value "ignored:one" for option -r: invalid relation id`,
	}, {
		ctxrelid: -1,
		args:     []string{"-r", "2"},
		err:      `invalid value "2" for option -r: relation not found`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "ignored:2"},
		err:      `invalid value "ignored:2" for option -r: relation not found`,
	}, {
		ctxrelid: -1,
		err:      `no relation id specified`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "ignored:0"},
		relid:    0,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "0"},
		relid:    0,
	}, {
		ctxrelid: -1,
		args:     []string{"-r", "1"},
		relid:    1,
	}, {
		ctxrelid: 0,
		args:     []string{"-r", "1"},
		relid:    1,
	},
}

func (s *RelationModelGetSuite) TestInit(c *tc.C) {
	for i, t := range relationModelGetInitTests {
		c.Logf("test %d", i)
		hctx, _ := s.newHookContext(t.ctxrelid, "", "")
		com, err := jujuc.NewCommand(hctx, "relation-model-get")
		c.Assert(err, tc.ErrorIsNil)

		err = cmdtesting.InitCommand(com, t.args)
		if t.err == "" {
			if !c.Check(err, tc.ErrorIsNil) {
				return
			}
			rset := com.(*jujuc.RelationModelGetCommand)
			c.Check(rset.RelationId, tc.Equals, t.relid)
		} else {
			c.Check(err, tc.ErrorMatches, t.err)
		}
	}
}

func (s *RelationModelGetSuite) TestRun(c *tc.C) {
	hctx, _ := s.newHookContext(0, "", "")
	com, err := jujuc.NewCommand(hctx, "relation-model-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, nil)
	c.Check(code, tc.Equals, 0)
	c.Check(bufferString(ctx.Stderr), tc.Equals, "")
	expect := "uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d\n"
	c.Check(bufferString(ctx.Stdout), tc.Equals, expect)
}

func (s *RelationModelGetSuite) TestRunFormatJSON(c *tc.C) {
	hctx, _ := s.newHookContext(0, "", "")
	com, err := jujuc.NewCommand(hctx, "relation-model-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--format", "json"})
	c.Check(code, tc.Equals, 0)
	c.Check(bufferString(ctx.Stderr), tc.Equals, "")
	expect := `{"uuid":"deadbeef-0bad-400d-8000-4b1d0d06f00d"}` + "\n"
	c.Check(bufferString(ctx.Stdout), tc.Equals, expect)
}
