package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type CredentialGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&CredentialGetSuite{})

func runCredentialGetCommand(s *CredentialGetSuite, c *gc.C, args []string) (*cmd.Context, int) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, cmdString("credential-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, args)
	return ctx, code
}

func (s *CredentialGetSuite) TestHelp(c *gc.C) {
	_, exitCode := runCredentialGetCommand(s, c, []string{})
	exitSuccess := 0
	c.Assert(exitCode, gc.Equals, exitSuccess)
}
