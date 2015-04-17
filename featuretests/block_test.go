// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/block"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type blockSuite struct {
	jujutesting.JujuConnSuite
	blockClient *block.Client
}

func (s *blockSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.blockClient = block.NewClient(s.APIState)
	c.Assert(s.blockClient, gc.NotNil)
}

func (s *blockSuite) TearDownTest(c *gc.C) {
	s.blockClient.ClientFacade.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *blockSuite) TestBlockFacadeCall(c *gc.C) {
	found, err := s.blockClient.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 0)
}

func (s *blockSuite) TestBlockFacadeCallGettingErrors(c *gc.C) {
	err := s.blockClient.SwitchBlockOff(state.DestroyBlock.String())
	c.Assert(errors.Cause(err), gc.ErrorMatches, `.*is already OFF.*`)
}
