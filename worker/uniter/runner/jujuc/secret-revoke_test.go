// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type SecretRevokeSuite struct {
	ContextSuite
}

var _ = gc.Suite(&SecretRevokeSuite{})

func (s *SecretRevokeSuite) TestRevokeSecretInvalidArgs(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	for _, t := range []struct {
		args []string
		err  string
	}{
		{
			args: []string{},
			err:  "ERROR missing secret name",
		}, {
			args: []string{"password"},
			err:  `ERROR missing application or unit`,
		}, {
			args: []string{"password", "--app", "0/foo"},
			err:  `ERROR application "0/foo" not valid`,
		}, {
			args: []string{"password", "--unit", "foo"},
			err:  `ERROR unit "foo" not valid`,
		},
	} {
		com, err := jujuc.NewCommand(hctx, "secret-revoke")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)

		c.Assert(code, gc.Equals, 2)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, t.err+"\n")
	}
}

func (s *SecretRevokeSuite) TestRevokeSecret(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-revoke")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"password", "--app", "foo",
	})

	c.Assert(code, gc.Equals, 0)
	app := "foo"
	args := &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
	}
	s.Stub.CheckCallNames(c, "RevokeSecret")
	s.Stub.CheckCall(c, 0, "RevokeSecret", "password", args)
}
