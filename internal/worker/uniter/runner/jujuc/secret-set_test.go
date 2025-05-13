// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/juju/tc"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type SecretUpdateSuite struct {
	ContextSuite
}

var _ = tc.Suite(&SecretUpdateSuite{})

func (s *SecretUpdateSuite) TestUpdateSecretInvalidArgs(c *tc.C) {
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
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "s3cret"},
			err:  `ERROR key value "s3cret" not valid`,
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "foo=bar", "--rotate", "foo"},
			err:  `ERROR rotate policy "foo" not valid`,
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "foo=bar", "--expire", "-1h"},
			err:  `ERROR negative expire duration "-1h" not valid`,
		}, {
			args: []string{"secret:9m4e2mr0ui3e8a215n4g", "foo=bar", "--expire", "2022-01-01"},
			err:  `ERROR expire time or duration "2022-01-01" not valid`,
		},
	} {
		com, err := jujuc.NewCommand(hctx, "secret-set")
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)

		c.Assert(code, tc.Equals, 2)
		c.Assert(bufferString(ctx.Stderr), tc.Equals, t.err+"\n")
	}
}

func (s *SecretUpdateSuite) TestUpdateSecret(c *tc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	expectedExpiry := time.Now().Add(time.Hour)
	com, err := jujuc.NewCommand(hctx, "secret-set")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"secret:9m4e2mr0ui3e8a215n4g", "data=secret",
		"--rotate", "daily", "--expire", "1h",
		"--description", "sssshhhh",
		"--label", "foobar",
	})

	c.Assert(code, tc.Equals, 0)
	val := coresecrets.NewSecretValue(map[string]string{"data": "c2VjcmV0"})
	expectedArgs := &jujuc.SecretUpdateArgs{
		Value:        val,
		RotatePolicy: ptr(coresecrets.RotateDaily),
		Description:  ptr("sssshhhh"),
		Label:        ptr("foobar"),
	}
	s.Stub.CheckCallNames(c, "UpdateSecret")
	call := s.Stub.Calls()[0]
	c.Assert(call.Args, tc.HasLen, 2)
	c.Assert(call.Args[0], tc.Equals, "secret:9m4e2mr0ui3e8a215n4g")
	args, ok := call.Args[1].(*jujuc.SecretUpdateArgs)
	c.Assert(ok, tc.IsTrue)
	c.Assert(args.ExpireTime, tc.NotNil)
	c.Assert(args.ExpireTime.After(expectedExpiry), tc.IsTrue)
	args.ExpireTime = nil
	c.Assert(args, tc.DeepEquals, expectedArgs)
}

func (s *SecretUpdateSuite) TestUpdateSecretBase64(c *tc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-set")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g", "token#base64=key="})

	c.Assert(code, tc.Equals, 0)
	val := coresecrets.NewSecretValue(map[string]string{"token": "key="})
	args := &jujuc.SecretUpdateArgs{
		Value: val,
	}
	s.Stub.CheckCalls(c, []testhelpers.StubCall{{FuncName: "UpdateSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", args}}})
}

func (s *SecretUpdateSuite) TestUpdateSecretRotateInterval(c *tc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-set")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--rotate", "daily", "secret:9m4e2mr0ui3e8a215n4g"})

	c.Assert(code, tc.Equals, 0)
	args := &jujuc.SecretUpdateArgs{
		Value:        coresecrets.NewSecretValue(nil),
		RotatePolicy: ptr(coresecrets.RotateDaily),
	}
	s.Stub.CheckCalls(c, []testhelpers.StubCall{{FuncName: "UpdateSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", args}}})
}

func (s *SecretUpdateSuite) TestUpdateSecretFromFile(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

	hctx, _ := s.ContextSuite.NewHookContext()
	com, err := jujuc.NewCommand(hctx, "secret-set")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g", "token#base64=key=", "--file", fileName})

	c.Assert(code, tc.Equals, 0)
	val := coresecrets.NewSecretValue(map[string]string{
		"token":       "key=",
		"key":         "c2VjcmV0",
		"another-key": `R0lGODlhDAAMAIQAAP//9/X17unp5WZmZgAAAOfn515eXvPz7Y6OjuDg4J+fn5OTk6enp56enmlpaWNjY6Ojo4SEhP/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++SH+Dk1hZGUgd2l0aCBHSU1QACwAAAAADAAMAAAFLCAgjoEwnuNAFOhpEMTRiggcz4BNJHrv/zCFcLiwMWYNG84BwwEeECcgggoBADs=`,
	})
	args := &jujuc.SecretUpdateArgs{
		Value: val,
	}
	s.Stub.CheckCalls(c, []testhelpers.StubCall{{FuncName: "UpdateSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", args}}})
}
