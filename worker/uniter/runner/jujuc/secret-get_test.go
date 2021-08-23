// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"encoding/base64"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type SecretGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&SecretGetSuite{})

func (s *SecretGetSuite) TestSecretGetSingularString(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"data": base64.StdEncoding.EncodeToString([]byte("s3cret!")),
	})

	com, err := jujuc.NewCommand(hctx, cmdString("secret-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret://v1/app.password"})
	c.Assert(code, gc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret://v1/app.password"}}})
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "s3cret!\n")
}

func (s *SecretGetSuite) TestSecretGetStringJson(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"key": base64.StdEncoding.EncodeToString([]byte("s3cret!")),
	})

	com, err := jujuc.NewCommand(hctx, cmdString("secret-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret://v1/app.password", "--format", "json"})
	c.Assert(code, gc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret://v1/app.password"}}})
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `{"key":"s3cret!"}`+"\n")
}

func (s *SecretGetSuite) TestSecretGetSingularEncoded(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"data": base64.StdEncoding.EncodeToString([]byte("s3cret!")),
	})

	com, err := jujuc.NewCommand(hctx, cmdString("secret-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret://v1/app.password", "--base64"})
	c.Assert(code, gc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret://v1/app.password"}}})
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "czNjcmV0IQ==\n")
}

func (s *SecretGetSuite) TestSecretGet(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"cert": base64.StdEncoding.EncodeToString([]byte("cert")),
		"key":  base64.StdEncoding.EncodeToString([]byte("key")),
	})

	com, err := jujuc.NewCommand(hctx, cmdString("secret-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret://v1/app.password"})
	c.Assert(code, gc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret://v1/app.password"}}})
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
cert: cert
key: key

`[1:])
}

func (s *SecretGetSuite) TestSecretGetEncoded(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"cert": base64.StdEncoding.EncodeToString([]byte("cert")),
		"key":  base64.StdEncoding.EncodeToString([]byte("key")),
	})

	com, err := jujuc.NewCommand(hctx, cmdString("secret-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret://v1/app.password", "--base64"})
	c.Assert(code, gc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret://v1/app.password"}}})
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
cert: Y2VydA==
key: a2V5

`[1:])
}

func (s *SecretGetSuite) TestSecretGetAttribute(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"cert": base64.StdEncoding.EncodeToString([]byte("cert")),
		"key":  base64.StdEncoding.EncodeToString([]byte("key")),
	})

	com, err := jujuc.NewCommand(hctx, cmdString("secret-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret://v1/app.password#cert"})
	c.Assert(code, gc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret://v1/app.password#cert"}}})
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "cert\n")
}
