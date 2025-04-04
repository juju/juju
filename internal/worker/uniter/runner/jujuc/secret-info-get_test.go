// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type SecretInfoGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&SecretInfoGetSuite{})

func (s *SecretInfoGetSuite) TestSecretGetInit(c *gc.C) {

	for _, t := range []struct {
		args []string
		err  string
	}{{
		args: []string{},
		err:  "ERROR require either a secret URI or label",
	}, {
		args: []string{"secret:9m4e2mr0ui3e8a215n4g", "--label", "foo"},
		err:  "ERROR specify either a secret URI or label but not both",
	}} {
		hctx, _ := s.ContextSuite.NewHookContext()
		com, err := jujuc.NewHookCommand(hctx, "secret-info-get")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Check(code, gc.Equals, 2)
		c.Check(bufferString(ctx.Stderr), gc.Equals, t.err+"\n")
	}
}

func (s *SecretInfoGetSuite) TestSecretInfoGetURI(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewHookCommand(hctx, "secret-info-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g"})
	c.Assert(code, gc.Equals, 0)

	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
9m4e2mr0ui3e8a215n4g:
  revision: 666
  label: label
  owner: application
  description: description
  rotation: hourly
`[1:])
}

func (s *SecretInfoGetSuite) TestSecretInfoGetWithGrants(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()
	hctx.ContextSecrets.Access = []secrets.AccessInfo{
		{
			Target: "application-gitlab",
			Scope:  "relation-key",
			Role:   secrets.RoleView,
		},
	}

	com, err := jujuc.NewHookCommand(hctx, "secret-info-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:9m4e2mr0ui3e8a215n4g"})
	c.Assert(code, gc.Equals, 0)

	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
9m4e2mr0ui3e8a215n4g:
  revision: 666
  label: label
  owner: application
  description: description
  rotation: hourly
  access:
  - target: application-gitlab
    scope: relation-key
    role: view
`[1:])
}

func (s *SecretInfoGetSuite) TestSecretInfoGetFailedNotFound(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewHookCommand(hctx, "secret-info-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"secret:cd88u16ffbaql5kgmlh0"})
	c.Assert(code, gc.Equals, 1)

	c.Assert(bufferString(ctx.Stderr), gc.Matches, `ERROR secret "cd88u16ffbaql5kgmlh0" not found\n`)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, ``)
}

func (s *SecretInfoGetSuite) TestSecretInfoGetByLabelFailedNotFound(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewHookCommand(hctx, "secret-info-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--label", "not-found-label"})
	c.Assert(code, gc.Equals, 1)

	c.Assert(bufferString(ctx.Stderr), gc.Matches, `ERROR secret "not-found-label" not found\n`)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, ``)
}

func (s *SecretInfoGetSuite) TestSecretInfoGetByLabel(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewHookCommand(hctx, "secret-info-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--label", "label"})
	c.Assert(code, gc.Equals, 0)

	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
9m4e2mr0ui3e8a215n4g:
  revision: 666
  label: label
  owner: application
  description: description
  rotation: hourly
`[1:])
}
