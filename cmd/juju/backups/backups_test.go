// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

var expectedSubCommmandNames = []string{
	"create",
	"download",
	"help",
	"info",
	"list",
	"remove",
	"restore",
	"upload",
}

type backupsSuite struct {
	BaseBackupsSuite
}

var _ = gc.Suite(&backupsSuite{})

func (s *backupsSuite) TestHelp(c *gc.C) {
	// Check the help output
	ctx, err := testing.RunCommand(c, s.command, "--help")
	c.Assert(err, jc.ErrorIsNil)
	namesFound := testing.ExtractCommandsFromHelpOutput(ctx)
	c.Assert(namesFound, gc.DeepEquals, expectedSubCommmandNames)
}
