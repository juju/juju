// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type GoalStateSuite struct {
	ContextSuite
}

func TestGoalStateSuite(t *testing.T) {
	tc.Run(t, &GoalStateSuite{})
}

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

func (s *GoalStateSuite) TestOutputFormatAll(c *tc.C) {
	for i, t := range goalStateAllTests {
		c.Logf("test %d: %#v", i, t.args)

		ctx, code := s.getGoalStateCommand(c, t.args)

		c.Assert(code, tc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), tc.Equals, t.out)
	}
}

func (s *GoalStateSuite) TestOutputPath(c *tc.C) {

	for i, t := range goalStateAllOutPutFileTests {
		c.Logf("test %d: %#v", i, t.args)

		ctx, code := s.getGoalStateCommand(c, t.args)

		c.Assert(code, tc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), tc.Equals, "")

		content, err := os.ReadFile(filepath.Join(ctx.Dir, "some-file"))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(string(content), tc.Equals, t.out)
	}
}

func (s *GoalStateSuite) getGoalStateCommand(c *tc.C, args []string) (*cmd.Context, int) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "goal-state")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
	return ctx, code
}

func (s *GoalStateSuite) TestUnknownArg(c *tc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "goal-state")
	c.Assert(err, tc.ErrorIsNil)
	cmdtesting.TestInit(c, jujuc.NewJujucCommandWrappedForTest(com), []string{}, "")
}
