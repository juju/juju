// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type SecretRevokeSuite struct {
	relationSuite
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
			err:  "ERROR missing secret URI",
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g"},
			err:  `ERROR missing relation or application or unit`,
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--app", "0/foo"},
			err:  `ERROR application "0/foo" not valid`,
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--app", "foo", "--unit", "foo/0"},
			err:  `ERROR specify only one of application or unit`,
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--unit", "foo"},
			err:  `ERROR unit "foo" not valid`,
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--relation", "-666"},
			err:  `ERROR invalid value "-666" for option --relation: relation not found`,
		},
	} {
		com, err := jujuc.NewHookCommand(hctx, "secret-revoke")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)

		c.Assert(code, gc.Equals, 2)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, t.err+"\n")
	}
}

func (s *SecretRevokeSuite) TestRevokeSecretForApp(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewHookCommand(hctx, "secret-revoke")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"secret:9m4e2mr0ui3e8a215n4g", "--app", "mediawiki",
	})

	c.Assert(code, gc.Equals, 0)
	args := &jujuc.SecretGrantRevokeArgs{
		ApplicationName: ptr("mediawiki"),
	}
	s.Stub.CheckCallNames(c, "HookRelation", "RevokeSecret")
	s.Stub.CheckCall(c, 1, "RevokeSecret", "secret:9m4e2mr0ui3e8a215n4g", args)
}

func (s *SecretRevokeSuite) TestRevokeSecretForRelation(c *gc.C) {
	hctx, _ := s.newHookContext(1, "mediawiki/0", "mediawiki")

	com, err := jujuc.NewHookCommand(hctx, "secret-revoke")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"secret:9m4e2mr0ui3e8a215n4g", "--relation", "db:1",
	})

	c.Assert(code, gc.Equals, 0)
	args := &jujuc.SecretGrantRevokeArgs{
		ApplicationName: ptr("mediawiki"),
	}
	s.Stub.CheckCallNames(c, "HookRelation", "Id", "FakeId", "Relation", "Relation", "RemoteApplicationName", "RevokeSecret")
	s.Stub.CheckCall(c, 6, "RevokeSecret", "secret:9m4e2mr0ui3e8a215n4g", args)
}

func (s *SecretRevokeSuite) TestRevokeSecretForRelationUnit(c *gc.C) {
	hctx, _ := s.newHookContext(1, "mediawiki/0", "mediawiki")

	com, err := jujuc.NewHookCommand(hctx, "secret-revoke")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"secret:9m4e2mr0ui3e8a215n4g", "--relation", "db:1", "--unit", "mediawiki/0",
	})

	c.Assert(code, gc.Equals, 0)
	args := &jujuc.SecretGrantRevokeArgs{
		ApplicationName: ptr("mediawiki"),
		UnitName:        ptr("mediawiki/0"),
	}
	s.Stub.CheckCallNames(c, "HookRelation", "Id", "FakeId", "Relation", "Relation", "RemoteApplicationName", "RevokeSecret")
	s.Stub.CheckCall(c, 6, "RevokeSecret", "secret:9m4e2mr0ui3e8a215n4g", args)
}
