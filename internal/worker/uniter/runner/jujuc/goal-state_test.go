// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type GoalStateSuite struct {
	ContextSuite
}

var _ = gc.Suite(&GoalStateSuite{})

var (
	goalStateTestResultYaml = `units:
  mysql/0:
    status: active
    since: 2200-11-05 15:29:12Z
relations:
  db:
    mysql/0:
      status: active
      since: 2200-11-05 15:29:12Z
  server:
    wordpress/0:
      status: active
      since: 2200-11-05 15:29:12Z
`
	goalStateTestResultJson = `{"units":{"mysql/0":{"status":"active","since":"2200-11-05 15:29:12Z"}},"relations":{"db":{"mysql/0":{"status":"active","since":"2200-11-05 15:29:12Z"}},"server":{"wordpress/0":{"status":"active","since":"2200-11-05 15:29:12Z"}}}}
`

	goalStateAllTests = []struct {
		args []string
		out  string
	}{
		{nil, goalStateTestResultYaml},
		{[]string{"--format", "yaml"}, goalStateTestResultYaml},
		{[]string{"--format", "json"}, goalStateTestResultJson},
	}

	goalStateAllOutPutFileTests = []struct {
		args []string
		out  string
	}{
		{[]string{"--output", "some-file"}, goalStateTestResultYaml},
		{[]string{"--format", "yaml", "--output", "some-file"}, goalStateTestResultYaml},
		{[]string{"--format", "json", "--output", "some-file"}, goalStateTestResultJson},
	}
)

func (s *GoalStateSuite) TestOutputFormatAll(c *gc.C) {
	for i, t := range goalStateAllTests {
		c.Logf("test %d: %#v", i, t.args)

		ctx, code := s.getGoalStateCommand(c, t.args)

		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, t.out)
	}
}

func (s *GoalStateSuite) TestOutputPath(c *gc.C) {

	for i, t := range goalStateAllOutPutFileTests {
		c.Logf("test %d: %#v", i, t.args)

		ctx, code := s.getGoalStateCommand(c, t.args)

		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")

		content, err := os.ReadFile(filepath.Join(ctx.Dir, "some-file"))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(string(content), gc.Equals, t.out)
	}
}

func (s *GoalStateSuite) getGoalStateCommand(c *gc.C, args []string) (*cmd.Context, int) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewHookCommand(hctx, "goal-state")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
	return ctx, code
}

func (s *GoalStateSuite) TestUnknownArg(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewHookCommand(hctx, "goal-state")
	c.Assert(err, jc.ErrorIsNil)
	cmdtesting.TestInit(c, jujuc.NewJujucCommandWrappedForTest(com), []string{}, "")
}
