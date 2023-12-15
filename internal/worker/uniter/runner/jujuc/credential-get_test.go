// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type CredentialGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&CredentialGetSuite{})

// [TODO](externalreality): Many jujuc commands can be run through a processor
// much like the one below. This sort of thing should not have to be written
// more than once except for in special cases. A structure containing all of the
// relevant jujuc commands along with their supported format options would cut
// down on a great deal of test fluff. The juju/cmd test are a good example of how
// this might be done.
func runCredentialGetCommand(s *CredentialGetSuite, c *gc.C, args []string) (*cmd.Context, int) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "credential-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
	return ctx, code
}

func (s *CredentialGetSuite) TestCommandRun(c *gc.C) {
	_, exitCode := runCredentialGetCommand(s, c, []string{})
	exitSuccess := 0
	c.Assert(exitCode, gc.Equals, exitSuccess)
}
