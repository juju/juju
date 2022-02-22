// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"

	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/block"
	"github.com/juju/juju/core/model"
	jujutesting "github.com/juju/juju/juju/testing"
)

type blockSuite struct {
	jujutesting.JujuConnSuite
	blockClient *block.Client
}

func (s *blockSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.blockClient = block.NewClient(s.APIState)
	c.Assert(s.blockClient, gc.NotNil)
	s.AddCleanup(func(*gc.C) {
		s.blockClient.ClientFacade.Close()
	})
}

func (s *blockSuite) TestBlockFacadeCall(c *gc.C) {
	found, err := s.blockClient.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 0)
}

func (s *blockSuite) TestBlockedMessage(c *gc.C) {
	// Block operation
	s.blockClient.SwitchBlockOn(fmt.Sprintf("%v", model.BlockChange), "TestBlockedMessage")

	ctx, err := runCommand(c, "resolved", "multi-series/2")

	// Whenever Juju blocks are enabled, the operations that are blocked will be expected to err
	// out silently.
	c.Assert(err, gc.ErrorMatches, "cmd: error out silently")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
ERROR TestBlockedMessage (operation is blocked)

All operations that change model have been disabled for the current model.
To enable changes, run

    juju enable-command all


`[1:])
}
