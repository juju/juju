// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/tc"
)

type controllerStateSuite struct {
	schematesting.ControllerSuite

	state *ControllerState
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &controllerStateSuite{})
}

func (s *controllerStateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.state = NewControllerState(s.TxnRunnerFactory())
}

func (s *controllerStateSuite) TestGetAgentBinarySHA256Exists(c *tc.C) {
	objStoreUUID, _ := addObjectStore(c, s.TxnRunner())
	sha := getObjectSHA256(c, s.DB(), objStoreUUID)
	err := s.state.RegisterAgentBinary(c.Context(), agentbinary.RegisterAgentBinaryArg{
		Version:         "4.0.0",
		Arch:            "amd64",
		ObjectStoreUUID: objStoreUUID,
	})
	c.Assert(err, tc.ErrorIsNil)

	num, err := semversion.Parse("4.0.0")
	c.Assert(err, tc.ErrorIsNil)

	ver := coreagentbinary.Version{Number: num, Arch: "amd64"}
	exists, shaRes, err := s.state.GetAgentBinarySHA256(c.Context(), ver, coreagentbinary.AgentStreamProposed)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.Equals, true)
	c.Check(shaRes, tc.Equals, sha)
}
