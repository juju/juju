// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineenvironmentworker_test

import (
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/environment"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/machineenvironmentworker"
)

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = 5 * time.Second

type MachineEnvironmentWatcherSuite struct {
	testing.JujuConnSuite

	apiRoot        *api.State
	environmentAPI *environment.Facade
	machine        *state.Machine
}

var _ = gc.Suite(&MachineEnvironmentWatcherSuite{})

func (s *MachineEnvironmentWatcherSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.apiRoot, s.machine = s.OpenAPIAsNewMachine(c)
	// Create the machiner API facade.
	s.environmentAPI = s.apiRoot.Environment()
	c.Assert(s.environmentAPI, gc.NotNil)
}

func (s *MachineEnvironmentWatcherSuite) waitProxySettings(c *gc.C, expected osenv.ProxySettings) {
	timeout := time.After(worstCase)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for proxy settings to change")
		case <-time.After(10 * time.Millisecond):
			obtained := osenv.DetectProxies()
			if obtained != expected {
				c.Logf("proxy settings are %#v, still waiting", obtained)
				continue
			}
			return
		}
	}
}

func (s *MachineEnvironmentWatcherSuite) makeWorker(c *gc.C) worker.Worker {
	worker := machineenvironmentworker.NewMachineEnvironmentWorker(s.environmentAPI)
	return worker
}

func (s *MachineEnvironmentWatcherSuite) TestRunStop(c *gc.C) {
	envWorker := s.makeWorker(c)
	c.Assert(worker.Stop(envWorker), gc.IsNil)
}

func (s *MachineEnvironmentWatcherSuite) TestInitialState(c *gc.C) {
	oldConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	proxySettings := osenv.ProxySettings{
		Http:  "http proxy",
		Https: "https proxy",
		Ftp:   "ftp proxy",
	}

	envConfig, err := oldConfig.Apply(config.ProxyConfigMap(proxySettings))
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(envConfig, oldConfig)
	c.Assert(err, gc.IsNil)

	envWorker := s.makeWorker(c)
	defer worker.Stop(envWorker)

	s.waitProxySettings(c, proxySettings)
}
