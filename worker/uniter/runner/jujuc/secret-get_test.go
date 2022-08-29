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

func (s *SecretGetSuite) TestSecretGetInit(c *gc.C) {

	for _, t := range []struct {
		args []string
		err  string
	}{{
		args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--peek", "--update"},
		err:  "ERROR specify one of --peek or --update but not both",
	}, {
		args: []string{"--metadata"},
		err:  "ERROR require either a secret URI or label to fetch metadata",
	}, {
		args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--label", "foo", "--metadata"},
		err:  "ERROR specify either a secret URI or label but not both to fetch metadata",
	}, {
		args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--metadata", "--update"},
		err:  "ERROR --peek and --update are not valid when fetching metadata",
	}, {
		args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--metadata", "--peek"},
		err:  "ERROR --peek and --update are not valid when fetching metadata",
	}} {
		hctx, _ := s.ContextSuite.NewHookContext()
		com, err := jujuc.NewCommand(hctx, "secret-get")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Check(code, gc.Equals, 2)
		c.Check(bufferString(ctx.Stderr), gc.Equals, t.err+"\n")
	}
}

func (s *SecretGetSuite) TestSecretGetJson(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"key": base64.StdEncoding.EncodeToString([]byte("s3cret!")),
	})

	com, err := jujuc.NewCommand(hctx, "secret-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g", "--format", "json"})
	c.Assert(code, gc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "", false, false}}})
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `{"key":"s3cret!"}`+"\n")
}

func (s *SecretGetSuite) TestSecretGet(c *gc.C) {
	s.assertSecretGet(c, false, false)
}

func (s *SecretGetSuite) TestSecretGetPeek(c *gc.C) {
	s.assertSecretGet(c, false, true)
}

func (s *SecretGetSuite) TestSecretGetUpdate(c *gc.C) {
	s.assertSecretGet(c, true, false)
}

func (s *SecretGetSuite) assertSecretGet(c *gc.C, update, peek bool) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"cert": base64.StdEncoding.EncodeToString([]byte("cert")),
		"key":  base64.StdEncoding.EncodeToString([]byte("key")),
	})

	com, err := jujuc.NewCommand(hctx, "secret-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	args := []string{"secret:9m4e2mr0ui3e8a215n4g", "--label", "label"}
	if update {
		args = append(args, "--update")
	}
	if peek {
		args = append(args, "--peek")
	}
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
	c.Assert(code, gc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "label", update, peek}}})
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
cert: cert
key: key
`[1:])
}

func (s *SecretGetSuite) TestSecretGetBinary(c *gc.C) {
	encodedValue := `R0lGODlhDAAMAIQAAP//9/X17unp5WZmZgAAAOfn515eXvPz7Y6OjuDg4J+fn5OTk6enp56enmlpaWNjY6Ojo4SEhP/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++SH+Dk1hZGUgd2l0aCBHSU1QACwAAAAADAAMAAAFLCAgjoEwnuNAFOhpEMTRiggcz4BNJHrv/zCFcLiwMWYNG84BwwEeECcgggoBADs=`
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"key": encodedValue,
	})

	com, err := jujuc.NewCommand(hctx, "secret-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g"})
	c.Assert(code, gc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "", false, false}}})
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
key: !!binary |
  R0lGODlhDAAMAIQAAP//9/X17unp5WZmZgAAAOfn515eXvPz7Y6OjuDg4J+fn5OTk6enp5
  6enmlpaWNjY6Ojo4SEhP/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/+
  +f/++SH+Dk1hZGUgd2l0aCBHSU1QACwAAAAADAAMAAAFLCAgjoEwnuNAFOhpEMTRiggcz4
  BNJHrv/zCFcLiwMWYNG84BwwEeECcgggoBADs=
`[1:])
}

func (s *SecretGetSuite) TestSecretGetKey(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"cert": base64.StdEncoding.EncodeToString([]byte("cert")),
		"key":  base64.StdEncoding.EncodeToString([]byte("key")),
	})

	com, err := jujuc.NewCommand(hctx, "secret-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g", "cert"})
	c.Assert(code, gc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "", false, false}}})
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
cert
`[1:])
}

func (s *SecretGetSuite) TestSecretGetKeyBase64(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"cert": base64.StdEncoding.EncodeToString([]byte("cert")),
		"key":  base64.StdEncoding.EncodeToString([]byte("key")),
	})

	com, err := jujuc.NewCommand(hctx, "secret-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g", "cert#base64"})
	c.Assert(code, gc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "", false, false}}})
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "Y2VydA==\n")
}

func (s *SecretGetSuite) TestSecretGetMetadata(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g", "--metadata"})
	c.Assert(code, gc.Equals, 0)

	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
9m4e2mr0ui3e8a215n4g:
  revision: 666
  label: label
  description: description
  rotation: hourly
`[1:])
}

func (s *SecretGetSuite) TestSecretGetMetadataByLabel(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--metadata", "--label", "label"})
	c.Assert(code, gc.Equals, 0)

	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
9m4e2mr0ui3e8a215n4g:
  revision: 666
  label: label
  description: description
  rotation: hourly
`[1:])
}
