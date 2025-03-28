// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type SecretAddSuite struct {
	ContextSuite
}

var _ = gc.Suite(&SecretAddSuite{})

func (s *SecretAddSuite) TestAddSecretInvalidArgs(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	for _, t := range []struct {
		args []string
		err  string
	}{
		{
			args: []string{},
			err:  "ERROR missing secret value or filename",
		}, {
			args: []string{"s3cret"},
			err:  `ERROR key value "s3cret" not valid`,
		}, {
			args: []string{"foo=bar", "--rotate", "foo"},
			err:  `ERROR rotate policy "foo" not valid`,
		}, {
			args: []string{"foo=bar", "--owner", "foo"},
			err:  `ERROR secret owner "foo" not valid`,
		}, {
			args: []string{"foo=bar", "--expire", "-1h"},
			err:  `ERROR negative expire duration "-1h" not valid`,
		}, {
			args: []string{"foo=bar", "--expire", "2022-01-01"},
			err:  `ERROR expire time or duration "2022-01-01" not valid`,
		},
	} {
		com, err := jujuc.NewHookCommand(hctx, "secret-add")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)

		c.Assert(code, gc.Equals, 2)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, t.err+"\n")
	}
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretAddSuite) TestAddSecretExpireDuration(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewHookCommand(hctx, "secret-add")
	c.Assert(err, jc.ErrorIsNil)

	expectedExpiry := time.Now().Add(time.Hour)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"--rotate", "daily", "--expire", "1h",
		"--description", "sssshhhh",
		"--label", "foobar",
		"data=secret",
	})

	c.Assert(code, gc.Equals, 0)
	val := coresecrets.NewSecretValue(map[string]string{"data": "c2VjcmV0"})
	expectedArgs := &jujuc.SecretCreateArgs{
		SecretUpdateArgs: jujuc.SecretUpdateArgs{
			Value:        val,
			RotatePolicy: ptr(coresecrets.RotateDaily),
			Description:  ptr("sssshhhh"),
			Label:        ptr("foobar"),
		},
		OwnerTag: names.NewApplicationTag("u"),
	}
	s.Stub.CheckCallNames(c, "UnitName", "CreateSecret")
	call := s.Stub.Calls()[1]
	c.Assert(call.Args, gc.HasLen, 1)
	args, ok := call.Args[0].(*jujuc.SecretCreateArgs)
	c.Assert(ok, jc.IsTrue)
	c.Assert(args.ExpireTime, gc.NotNil)
	c.Assert(args.ExpireTime.After(expectedExpiry), jc.IsTrue)
	args.ExpireTime = nil
	c.Assert(args, jc.DeepEquals, expectedArgs)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "secret:9m4e2mr0ui3e8a215n4g\n")
}

func (s *SecretAddSuite) TestAddSecretExpireTimestamp(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewHookCommand(hctx, "secret-add")
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"--rotate", "daily", "--expire", "2022-03-04T06:06:06",
		"--description", "sssshhhh",
		"--label", "foobar",
		"data=secret",
	})

	c.Assert(code, gc.Equals, 0)
	val := coresecrets.NewSecretValue(map[string]string{"data": "c2VjcmV0"})
	expectedExpiry, err := time.Parse("2006-01-02T15:04:05", "2022-03-04T06:06:06")
	c.Assert(err, jc.ErrorIsNil)
	args := &jujuc.SecretCreateArgs{
		SecretUpdateArgs: jujuc.SecretUpdateArgs{
			Value:        val,
			RotatePolicy: ptr(coresecrets.RotateDaily),
			Description:  ptr("sssshhhh"),
			Label:        ptr("foobar"),
			ExpireTime:   ptr(expectedExpiry),
		},
		OwnerTag: names.NewApplicationTag("u"),
	}
	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "UnitName"}, {FuncName: "CreateSecret", Args: []interface{}{args}}})
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "secret:9m4e2mr0ui3e8a215n4g\n")
}

func (s *SecretAddSuite) TestAddSecretBase64(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewHookCommand(hctx, "secret-add")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"token#base64=key=", "--owner", "unit"})

	c.Assert(code, gc.Equals, 0)
	val := coresecrets.NewSecretValue(map[string]string{"token": "key="})
	args := &jujuc.SecretCreateArgs{
		SecretUpdateArgs: jujuc.SecretUpdateArgs{
			Value: val,
		},
		OwnerTag: names.NewUnitTag("u/0"),
	}
	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "UnitName"}, {FuncName: "CreateSecret", Args: []interface{}{args}}})
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "secret:9m4e2mr0ui3e8a215n4g\n")
}

func (s *SecretAddSuite) TestAddSecretFromFile(c *gc.C) {
	data := `
    key: |-
      secret
    another-key: !!binary |
      R0lGODlhDAAMAIQAAP//9/X17unp5WZmZgAAAOfn515eXvPz7Y6OjuDg4J+fn5
      OTk6enp56enmlpaWNjY6Ojo4SEhP/++f/++f/++f/++f/++f/++f/++f/++f/+
      +f/++f/++f/++f/++f/++SH+Dk1hZGUgd2l0aCBHSU1QACwAAAAADAAMAAAFLC
      AgjoEwnuNAFOhpEMTRiggcz4BNJHrv/zCFcLiwMWYNG84BwwEeECcgggoBADs=`

	dir := c.MkDir()
	fileName := filepath.Join(dir, "secret.yaml")
	err := os.WriteFile(fileName, []byte(data), os.FileMode(0644))
	c.Assert(err, jc.ErrorIsNil)

	hctx, _ := s.ContextSuite.NewHookContext()
	com, err := jujuc.NewHookCommand(hctx, "secret-add")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"token#base64=key=", "--file", fileName})

	c.Assert(code, gc.Equals, 0)
	val := coresecrets.NewSecretValue(map[string]string{
		"token":       "key=",
		"key":         "c2VjcmV0",
		"another-key": `R0lGODlhDAAMAIQAAP//9/X17unp5WZmZgAAAOfn515eXvPz7Y6OjuDg4J+fn5OTk6enp56enmlpaWNjY6Ojo4SEhP/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++SH+Dk1hZGUgd2l0aCBHSU1QACwAAAAADAAMAAAFLCAgjoEwnuNAFOhpEMTRiggcz4BNJHrv/zCFcLiwMWYNG84BwwEeECcgggoBADs=`,
	})
	args := &jujuc.SecretCreateArgs{
		SecretUpdateArgs: jujuc.SecretUpdateArgs{
			Value: val,
		},
		OwnerTag: names.NewApplicationTag("u"),
	}
	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "UnitName"}, {FuncName: "CreateSecret", Args: []interface{}{args}}})
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "secret:9m4e2mr0ui3e8a215n4g\n")
}
