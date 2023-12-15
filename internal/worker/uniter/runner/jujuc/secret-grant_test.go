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

type SecretGrantSuite struct {
	relationSuite
}

var _ = gc.Suite(&SecretGrantSuite{})

func (s *SecretGrantSuite) TestGrantSecretInvalidArgs(c *gc.C) {
	hctx, _ := s.newHookContext(1, "mediawiki/0", "mediawiki")

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
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--unit", "foo"},
			err:  `ERROR unit "foo" not valid`,
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--relation", "foo"},
			err:  `ERROR invalid value "foo" for option --relation: invalid relation id`,
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--relation", "-666"},
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
	hctx, _ := s.newHookContext(1, "mediawiki/0", "mediawiki")

	com, err := jujuc.NewCommand(hctx, "secret-grant")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"secret:9m4e2mr0ui3e8a215n4g",
		"--relation", "db:1",
	})

	c.Assert(code, gc.Equals, 0)
	args := &jujuc.SecretGrantRevokeArgs{
		RelationKey:     ptr("wordpress:db mediawiki:db"),
		ApplicationName: ptr("mediawiki"),
	}
	s.Stub.CheckCallNames(c, "HookRelation", "Id", "FakeId", "Relation", "Relation", "RelationTag", "RemoteApplicationName", "GrantSecret")
	s.Stub.CheckCall(c, 7, "GrantSecret", "secret:9m4e2mr0ui3e8a215n4g", args)
}

func (s *SecretGrantSuite) TestGrantSecretUnit(c *gc.C) {
	hctx, _ := s.newHookContext(1, "mediawiki/0", "mediawiki")

	com, err := jujuc.NewCommand(hctx, "secret-grant")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"secret:9m4e2mr0ui3e8a215n4g",
		"--relation", "db:1",
		"--unit", "mediawiki/0",
	})

	c.Assert(code, gc.Equals, 0)
	args := &jujuc.SecretGrantRevokeArgs{
		RelationKey:     ptr("wordpress:db mediawiki:db"),
		ApplicationName: ptr("mediawiki"),
		UnitName:        ptr("mediawiki/0"),
	}
	s.Stub.CheckCallNames(c, "HookRelation", "Id", "FakeId", "Relation", "Relation", "RelationTag", "RemoteApplicationName", "GrantSecret")
	s.Stub.CheckCall(c, 7, "GrantSecret", "secret:9m4e2mr0ui3e8a215n4g", args)
}

func (s *SecretGrantSuite) TestGrantSecretWrongUnit(c *gc.C) {
	hctx, _ := s.newHookContext(1, "mediawiki/0", "mediawiki")

	com, err := jujuc.NewCommand(hctx, "secret-grant")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"secret:9m4e2mr0ui3e8a215n4g",
		"--relation", "db:1",
		"--unit", "foo/0",
	})

	c.Assert(code, gc.Equals, 2)
}
