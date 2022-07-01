// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/worker/uniter/runner/jujuc"
)

type SecretGrantSuite struct {
	ContextSuite
}

var _ = gc.Suite(&SecretGrantSuite{})

func (s *SecretGrantSuite) TestGrantSecretInvalidArgs(c *gc.C) {
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
		}, {
			args: []string{"password", "--app", "foo", "--relation", "foo"},
			err:  `ERROR invalid value "foo" for option --relation: invalid relation id`,
		}, {
			args: []string{"password", "--app", "foo", "--relation", "-666"},
			err:  `ERROR invalid value "-666" for option --relation: relation not found`,
		},
	} {
		com, err := jujuc.NewCommand(hctx, "secret-grant")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)

		c.Assert(code, gc.Equals, 2)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, t.err+"\n")
	}
}

func (s *SecretGrantSuite) TestGrantSecret(c *gc.C) {
	hctx, info := s.ContextSuite.NewHookContext()
	info.SetNewRelation(1, "db", s.Stub)
	info.SetAsRelationHook(1, "mediawiki")

	com, err := jujuc.NewCommand(hctx, "secret-grant")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"password", "--app", "foo",
		"--relation", "db:1",
	})

	c.Assert(code, gc.Equals, 0)
	app := "foo"
	relId := 1
	args := &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationId:      &relId,
	}
	s.Stub.CheckCallNames(c, "HookRelation", "Id", "FakeId", "Relation", "GrantSecret")
	s.Stub.CheckCall(c, 4, "GrantSecret", "password", args)
}
