// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type SecretRemoveSuite struct {
	ContextSuite
}

var _ = tc.Suite(&SecretRemoveSuite{})

func (s *SecretRemoveSuite) TestRemoveSecretInvalidArgs(c *tc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	for _, t := range []struct {
		args []string
		err  string
	}{
		{
			args: []string{},
			err:  "ERROR missing secret URI",
		}, {
			args: []string{"foo"},
			err:  `ERROR secret URI "foo" not valid`,
		},
	} {
		com, err := jujuc.NewCommand(hctx, "secret-remove")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)

		c.Assert(code, tc.Equals, 2)
		c.Assert(bufferString(ctx.Stderr), tc.Equals, t.err+"\n")
	}
}

func (s *SecretRemoveSuite) TestRemoveSecret(c *tc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-remove")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"secret:9m4e2mr0ui3e8a215n4g",
	})

	c.Assert(code, tc.Equals, 0)
	s.Stub.CheckCallNames(c, "RemoveSecret")
	call := s.Stub.Calls()[0]
	c.Assert(call.Args, tc.HasLen, 2)
	c.Assert(call.Args[0], tc.Equals, "secret:9m4e2mr0ui3e8a215n4g")
	c.Assert(call.Args[1], tc.IsNil)
}

func (s *SecretRemoveSuite) TestRemoveSecretRevision(c *tc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-remove")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"secret:9m4e2mr0ui3e8a215n4g", "--revision", "666",
	})

	c.Assert(code, tc.Equals, 0)
	s.Stub.CheckCallNames(c, "RemoveSecret")
	call := s.Stub.Calls()[0]
	c.Assert(call.Args, tc.HasLen, 2)
	c.Assert(call.Args[0], tc.Equals, "secret:9m4e2mr0ui3e8a215n4g")
	c.Assert(*(call.Args[1].(*int)), tc.Equals, 666)
}
