// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"encoding/base64"

	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type SecretGetSuite struct {
	ContextSuite
}

var _ = tc.Suite(&SecretGetSuite{})

func (s *SecretGetSuite) TestSecretGetInit(c *tc.C) {

	for _, t := range []struct {
		args []string
		err  string
	}{{
		args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--peek", "--refresh"},
		err:  "ERROR specify one of --peek or --refresh but not both",
	}, {
		args: []string{},
		err:  "ERROR require either a secret URI or label",
	}} {
		hctx, _ := s.ContextSuite.NewHookContext()
		com, err := jujuc.NewCommand(hctx, "secret-get")
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Check(code, tc.Equals, 2)
		c.Check(bufferString(ctx.Stderr), tc.Equals, t.err+"\n")
	}
}

func (s *SecretGetSuite) TestSecretGetJson(c *tc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"key": base64.StdEncoding.EncodeToString([]byte("s3cret!")),
	})

	com, err := jujuc.NewCommand(hctx, "secret-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g", "--format", "json"})
	c.Assert(code, tc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "", false, false}}})
	c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, `{"key":"s3cret!"}`+"\n")
}

func (s *SecretGetSuite) TestSecretGetViaURI(c *tc.C) {
	s.assertSecretGet(c, func() ([]string, testing.StubCall) {
		return []string{"secret:9m4e2mr0ui3e8a215n4g"},
			testing.StubCall{
				FuncName: "GetSecret",
				Args:     []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "", false, false},
			}
	})
}

func (s *SecretGetSuite) TestSecretGetViaLabel(c *tc.C) {
	s.assertSecretGet(c, func() ([]string, testing.StubCall) {
		return []string{"--label", "label"},
			testing.StubCall{
				FuncName: "GetSecret",
				Args:     []interface{}{"", "label", false, false},
			}
	})
}

func (s *SecretGetSuite) TestSecretGetPeekViaURI(c *tc.C) {
	s.assertSecretGet(c, func() ([]string, testing.StubCall) {
		return []string{"secret:9m4e2mr0ui3e8a215n4g", "--peek"},
			testing.StubCall{
				FuncName: "GetSecret",
				Args:     []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "", false, true},
			}
	})
}

func (s *SecretGetSuite) TestSecretGetPeekViaLabel(c *tc.C) {
	s.assertSecretGet(c, func() ([]string, testing.StubCall) {
		return []string{"--label", "label", "--peek"},
			testing.StubCall{
				FuncName: "GetSecret",
				Args:     []interface{}{"", "label", false, true},
			}
	})
}

func (s *SecretGetSuite) TestSecretGetUpdateWithURI(c *tc.C) {
	s.assertSecretGet(c, func() ([]string, testing.StubCall) {
		return []string{"secret:9m4e2mr0ui3e8a215n4g", "--refresh"},
			testing.StubCall{
				FuncName: "GetSecret",
				Args:     []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "", true, false},
			}
	})
}

func (s *SecretGetSuite) TestSecretGetUpdateWithLabel(c *tc.C) {
	s.assertSecretGet(c, func() ([]string, testing.StubCall) {
		return []string{"--label", "label", "--refresh"},
			testing.StubCall{
				FuncName: "GetSecret",
				Args:     []interface{}{"", "label", true, false},
			}
	})
}

func (s *SecretGetSuite) TestSecretGetUpdateWithBothURIAndLabel(c *tc.C) {
	s.assertSecretGet(c, func() ([]string, testing.StubCall) {
		return []string{"secret:9m4e2mr0ui3e8a215n4g", "--label", "label", "--refresh"},
			testing.StubCall{
				FuncName: "GetSecret",
				Args:     []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "label", true, false},
			}
	})
}

func (s *SecretGetSuite) assertSecretGet(c *tc.C, f func() ([]string, testing.StubCall)) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"cert": base64.StdEncoding.EncodeToString([]byte("cert")),
		"key":  base64.StdEncoding.EncodeToString([]byte("key")),
	})

	com, err := jujuc.NewCommand(hctx, "secret-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	args, checkCall := f()
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
	c.Assert(code, tc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{checkCall})
	c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, `
cert: cert
key: key
`[1:])
}

func (s *SecretGetSuite) TestSecretGetBinary(c *tc.C) {
	encodedValue := `R0lGODlhDAAMAIQAAP//9/X17unp5WZmZgAAAOfn515eXvPz7Y6OjuDg4J+fn5OTk6enp56enmlpaWNjY6Ojo4SEhP/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++SH+Dk1hZGUgd2l0aCBHSU1QACwAAAAADAAMAAAFLCAgjoEwnuNAFOhpEMTRiggcz4BNJHrv/zCFcLiwMWYNG84BwwEeECcgggoBADs=`
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"key": encodedValue,
	})

	com, err := jujuc.NewCommand(hctx, "secret-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g"})
	c.Assert(code, tc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "", false, false}}})
	c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, `
key: !!binary |
  R0lGODlhDAAMAIQAAP//9/X17unp5WZmZgAAAOfn515eXvPz7Y6OjuDg4J+fn5OTk6enp5
  6enmlpaWNjY6Ojo4SEhP/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/++f/+
  +f/++SH+Dk1hZGUgd2l0aCBHSU1QACwAAAAADAAMAAAFLCAgjoEwnuNAFOhpEMTRiggcz4
  BNJHrv/zCFcLiwMWYNG84BwwEeECcgggoBADs=
`[1:])
}

func (s *SecretGetSuite) TestSecretGetKey(c *tc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"cert": base64.StdEncoding.EncodeToString([]byte("cert")),
		"key":  base64.StdEncoding.EncodeToString([]byte("key")),
	})

	com, err := jujuc.NewCommand(hctx, "secret-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g", "cert"})
	c.Assert(code, tc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "", false, false}}})
	c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, `
cert
`[1:])
}

func (s *SecretGetSuite) TestSecretGetKeyBase64(c *tc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.SecretValue = secrets.NewSecretValue(map[string]string{
		"cert": base64.StdEncoding.EncodeToString([]byte("cert")),
		"key":  base64.StdEncoding.EncodeToString([]byte("key")),
	})

	com, err := jujuc.NewCommand(hctx, "secret-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g", "cert#base64"})
	c.Assert(code, tc.Equals, 0)

	s.Stub.CheckCalls(c, []testing.StubCall{{FuncName: "GetSecret", Args: []interface{}{"secret:9m4e2mr0ui3e8a215n4g", "", false, false}}})
	c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, "Y2VydA==\n")
}
