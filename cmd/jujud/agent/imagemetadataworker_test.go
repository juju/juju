// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/imagemetadata"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

// MachineMockProviderSuite runs worker tests that depend
// on provider that has cloud region support.
type MachineMockProviderSuite struct {
	commonMachineSuite
}

var _ = gc.Suite(&MachineMockProviderSuite{})

func (s *MachineMockProviderSuite) SetUpTest(c *gc.C) {
	// Change to environ that supports HasRegion
	s.commonMachineSuite.SetUpTest(c)
}

func (s *MachineMockProviderSuite) TestMachineAgentRunsMetadataWorker(c *gc.C) {
	// Patch out the worker func before starting the agent.
	started := make(chan struct{})
	newWorker := func(cl *imagemetadata.Client) worker.Worker {
		close(started)
		return worker.NewNoOpWorker()
	}
	s.PatchValue(&newMetadataUpdater, newWorker)
	s.PatchValue(&newEnvirons, func(config *config.Config) (environs.Environ, error) {
		return &dummyEnviron{}, nil
	})

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, state.JobManageModel)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	s.assertChannelActive(c, started, "metadata update worker to start")
}

// dummyEnviron is an environment with region support.
type dummyEnviron struct {
	environs.Environ
}

// Region is specified in the HasRegion interface.
func (e *dummyEnviron) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   "dummy_region",
		Endpoint: "https://anywhere",
	}, nil
}
