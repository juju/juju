// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type SecretIdsSuite struct {
	ContextSuite
}

func TestSecretIdsSuite(t *testing.T) {
	tc.Run(t, &SecretIdsSuite{})
}

func (s *SecretIdsSuite) TestSecretIds(c *tc.C) {
	hctx, _ := s.ContextSuite.NewHookContext()

	com, err := jujuc.NewCommand(hctx, "secret-ids")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, nil)

	c.Assert(code, tc.Equals, 0)
	s.Stub.CheckCallNames(c, "SecretMetadata")
	c.Assert(bufferString(ctx.Stdout), tc.Equals, "9m4e2mr0ui3e8a215n4g\n")
}
