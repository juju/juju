// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type SecretIdsSuite struct {
	ContextSuite
}

var _ = gc.Suite(&SecretIdsSuite{})

func (s *SecretIdsSuite) TestSecretIds(c *gc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-ids")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, nil)

	c.Assert(code, gc.Equals, 0)
	s.Stub.CheckCallNames(c, "SecretMetadata")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "9m4e2mr0ui3e8a215n4g\n")
}
