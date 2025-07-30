// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttest

import (
	"github.com/juju/tc"

	"github.com/juju/juju/juju/testing"
)

// AgentSuite is a fixture to be used by agent test suites.
type AgentSuite struct {
	testing.ApiServerSuite

	DataDir string
	LogDir  string
}

func (s *AgentSuite) SetUpTest(c *tc.C) {
	s.ApiServerSuite.SetUpTest(c)

	s.DataDir = c.MkDir()
	s.LogDir = c.MkDir()
}
