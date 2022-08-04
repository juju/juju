// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type SecretUpdateSuite struct {
	ContextSuite
}

var _ = gc.Suite(&SecretUpdateSuite{})

func (s *SecretUpdateSuite) TestUpdateSecretInvalidArgs(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	for _, t := range []struct {
		args []string
		err  string
	}{
		{
			args: []string{},
			err:  "ERROR missing secret URI",
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "s3cret", "foo=bar"},
			err:  `ERROR key value "foo=bar" not valid when a singular value has already been specified`,
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "foo=bar", "s3cret"},
			err:  `ERROR singular value "s3cret" not valid when other key values are specified`,
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "foo=bar", "--rotate", "-1h"},
			err:  `ERROR rotate interval "-1h0m0s" not valid`,
		},
	} {
		com, err := jujuc.NewCommand(hctx, "secret-update")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)

		c.Assert(code, gc.Equals, 2)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, t.err+"\n")
	}
}

func (s *SecretUpdateSuite) TestUpdateSecret(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-update")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"secret:9m4e2mr0ui3e8a215n4g", "secret", "--rotate", "1h",
		"--description", "sssshhhh",
		"--tag", "foo=bar", "--tag", "hello=world",
	})

	c.Assert(code, gc.Equals, 0)
	val := coresecrets.NewSecretValue(map[string]string{"data": "c2VjcmV0"})
	args := &jujuc.SecretUpsertArgs{
		Value:          val,
		RotateInterval: durationPtr(time.Hour),
		Description:    stringPtr("sssshhhh"),
		Tags:           tagPtr(map[string]string{"foo": "bar", "hello": "world"}),
	}
	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "UpdateSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", args}}})
}

func (s *SecretUpdateSuite) TestUpdateSecretBase64(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-update")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--base64", "secret:9m4e2mr0ui3e8a215n4g", "token=key="})

	c.Assert(code, gc.Equals, 0)
	val := coresecrets.NewSecretValue(map[string]string{"token": "key="})
	args := &jujuc.SecretUpsertArgs{
		Value: val,
		Tags:  tagPtr(nil),
	}
	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "UpdateSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", args}}})
}

func (s *SecretUpdateSuite) TestUpdateSecretRotateInterval(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-update")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--rotate", "5h", "secret:9m4e2mr0ui3e8a215n4g"})

	c.Assert(code, gc.Equals, 0)
	args := &jujuc.SecretUpsertArgs{
		Value:          coresecrets.NewSecretValue(nil),
		RotateInterval: durationPtr(5 * time.Hour),
		Tags:           tagPtr(nil),
	}
	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "UpdateSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", args}}})
}
