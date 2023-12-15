// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type SecretRemoveSuite struct {
	ContextSuite
}

var _ = gc.Suite(&SecretRemoveSuite{})

func (s *SecretRemoveSuite) TestRemoveSecretInvalidArgs(c *gc.C) {
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

		c.Assert(code, gc.Equals, 2)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, t.err+"\n")
	}
}

func (s *SecretRemoveSuite) TestRemoveSecret(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-remove")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"secret:9m4e2mr0ui3e8a215n4g",
	})

	c.Assert(code, gc.Equals, 0)
	s.Stub.CheckCallNames(c, "RemoveSecret")
	call := s.Stub.Calls()[0]
	c.Assert(call.Args, gc.HasLen, 2)
	c.Assert(call.Args[0], gc.Equals, "secret:9m4e2mr0ui3e8a215n4g")
	c.Assert(call.Args[1], gc.IsNil)
}

func (s *SecretRemoveSuite) TestRemoveSecretRevision(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-remove")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"secret:9m4e2mr0ui3e8a215n4g", "--revision", "666",
	})

	c.Assert(code, gc.Equals, 0)
	s.Stub.CheckCallNames(c, "RemoveSecret")
	call := s.Stub.Calls()[0]
	c.Assert(call.Args, gc.HasLen, 2)
	c.Assert(call.Args[0], gc.Equals, "secret:9m4e2mr0ui3e8a215n4g")
	c.Assert(*(call.Args[1].(*int)), gc.Equals, 666)
}
