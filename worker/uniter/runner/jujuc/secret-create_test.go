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

	coresecrets "github.com/juju/juju/v3/core/secrets"

	"github.com/juju/juju/v3/worker/uniter/runner/jujuc"
)

type SecretCreateSuite struct {
	ContextSuite
}

var _ = gc.Suite(&SecretCreateSuite{})

func (s *SecretCreateSuite) TestCreateSecretInvalidArgs(c *gc.C) {
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
			err:  "ERROR missing secret value",
		}, {
			args: []string{"password", "s3cret", "foo=bar"},
			err:  `ERROR key value "foo=bar" not valid when a singular value has already been specified`,
		}, {
			args: []string{"password", "foo=bar", "s3cret"},
			err:  `ERROR singular value "s3cret" not valid when other key values are specified`,
		}, {
			args: []string{"password", "foo=bar", "--rotate", "-1h"},
			err:  `ERROR rotate interval "-1h0m0s" not valid`,
		},
	} {
		com, err := jujuc.NewCommand(hctx, "secret-create")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)

		c.Assert(code, gc.Equals, 2)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, t.err+"\n")
	}
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}

func stringPtr(s string) *string {
	return &s
}

func statusPtr(s coresecrets.SecretStatus) *coresecrets.SecretStatus {
	return &s
}

func tagPtr(t map[string]string) *map[string]string {
	return &t
}

func (s *SecretCreateSuite) TestCreateSecret(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-create")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{
		"password", "secret", "--rotate", "1h",
		"--description", "sssshhhh",
		"--tag", "foo=bar", "--tag", "hello=world",
		"--staged",
	})

	c.Assert(code, gc.Equals, 0)
	val := coresecrets.NewSecretValue(map[string]string{"data": "c2VjcmV0"})
	args := &jujuc.SecretUpsertArgs{
		Type:           coresecrets.TypeBlob,
		Value:          val,
		RotateInterval: durationPtr(time.Hour),
		Status:         statusPtr(coresecrets.StatusStaged),
		Description:    stringPtr("sssshhhh"),
		Tags:           tagPtr(map[string]string{"foo": "bar", "hello": "world"}),
	}
	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "CreateSecret", Args: []interface{}{"password", args}}})
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "secret://app.password\n")
}

func (s *SecretCreateSuite) TestCreateSecretBase64(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-create")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--base64", "apikey", "token=key="})

	c.Assert(code, gc.Equals, 0)
	val := coresecrets.NewSecretValue(map[string]string{"token": "key="})
	args := &jujuc.SecretUpsertArgs{
		Type:           coresecrets.TypeBlob,
		Value:          val,
		RotateInterval: durationPtr(0),
		Description:    stringPtr(""),
		Status:         statusPtr(coresecrets.StatusActive),
		Tags:           tagPtr(nil),
	}
	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "CreateSecret", Args: []interface{}{"apikey", args}}})
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "secret://app.apikey\n")
}
