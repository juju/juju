// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttest

import (
	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/stateenvirons"
)

// AgentSuite is a fixture to be used by agent test suites.
type AgentSuite struct {
	testing.ApiServerSuite

	Environ environs.Environ
	DataDir string
	LogDir  string
}

func (s *AgentSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)

	domainServices := s.ControllerDomainServices(c)

	var err error
	s.Environ, err = stateenvirons.GetNewEnvironFunc(environs.New)(
		s.ControllerModel(c),
		domainServices.Cloud(),
		domainServices.Credential(),
		domainServices.Config(),
	)
	c.Assert(err, tc.ErrorIsNil)

	s.DataDir = c.MkDir()
	s.LogDir = c.MkDir()
}
