// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineenvironmentworker_test

import (
	"io/ioutil"
	"os"
	"path"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju/osenv"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/environment"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/machineenvironmentworker"
)

type MachineEnvironmentWatcherSuite struct {
	jujutesting.JujuConnSuite

	apiRoot        *api.State
	environmentAPI *environment.Facade
	machine        *state.Machine

	proxyFile string
	started   bool
}

var _ = gc.Suite(&MachineEnvironmentWatcherSuite{})

func (s *MachineEnvironmentWatcherSuite) setStarted() {
	s.started = true
}

func (s *MachineEnvironmentWatcherSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.apiRoot, s.machine = s.OpenAPIAsNewMachine(c)
	// Create the machiner API facade.
	s.environmentAPI = s.apiRoot.Environment()
	c.Assert(s.environmentAPI, gc.NotNil)

	proxyDir := c.MkDir()
	s.PatchValue(&machineenvironmentworker.ProxyDirectory, proxyDir)
	s.started = false
	s.PatchValue(&machineenvironmentworker.Started, s.setStarted)

	s.proxyFile = path.Join(proxyDir, machineenvironmentworker.ProxyFile)
}

func (s *MachineEnvironmentWatcherSuite) waitForPostSetup(c *gc.C) {
	for {
		select {
		case <-time.After(testing.LongWait):
			c.Fatalf("timeout while waiting for setup")
		case <-time.After(10 * time.Millisecond):
			if s.started {
				return
			}
		}
	}
}

func (s *MachineEnvironmentWatcherSuite) waitProxySettings(c *gc.C, expected osenv.ProxySettings) {
	for {
		select {
		case <-time.After(testing.LongWait):
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

func (s *MachineEnvironmentWatcherSuite) waitForFile(c *gc.C, filename, expected string) {
	for {
		select {
		case <-time.After(testing.LongWait):
			c.Fatalf("timeout while waiting for proxy settings to change")
		case <-time.After(10 * time.Millisecond):
			fileContent, err := ioutil.ReadFile(filename)
			if os.IsNotExist(err) {
				continue
			}
			c.Assert(err, gc.IsNil)
			if string(fileContent) != expected {
				c.Logf("file content not matching, still waiting")
				continue
			}
			return
		}
	}
}

func (s *MachineEnvironmentWatcherSuite) makeWorker(c *gc.C) worker.Worker {
	return machineenvironmentworker.NewMachineEnvironmentWorker(s.environmentAPI)
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
	s.waitForFile(c, s.proxyFile, proxySettings.AsScriptEnvironment()+"\n")
}

func (s *MachineEnvironmentWatcherSuite) TestRespondsToEvents(c *gc.C) {
	oldConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)

	envWorker := s.makeWorker(c)
	defer worker.Stop(envWorker)
	s.waitForPostSetup(c)

	proxySettings := osenv.ProxySettings{
		Http:  "http proxy",
		Https: "https proxy",
		Ftp:   "ftp proxy",
	}

	envConfig, err := oldConfig.Apply(config.ProxyConfigMap(proxySettings))
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(envConfig, oldConfig)
	c.Assert(err, gc.IsNil)

	s.waitProxySettings(c, proxySettings)
	s.waitForFile(c, s.proxyFile, proxySettings.AsScriptEnvironment()+"\n")
}
