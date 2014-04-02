// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater_test

import (
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/apiaddressupdater"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type APIAddressUpdaterSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&APIAddressUpdaterSuite{})

type apiAddressSetter struct {
	servers chan [][]instance.HostPort
	err     error
}

func (s *apiAddressSetter) SetAPIHostPorts(servers [][]instance.HostPort) error {
	s.servers <- servers
	return s.err
}

func (s *APIAddressUpdaterSuite) TestStartStop(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	worker := apiaddressupdater.NewAPIAddressUpdater(st.Machiner(), &apiAddressSetter{})
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func (s *APIAddressUpdaterSuite) TestAddressInitialUpdate(c *gc.C) {
	updatedServers := [][]instance.HostPort{instance.AddressesWithPort(
		instance.NewAddresses("localhost", "127.0.0.1"),
		1234,
	)}
	err := s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, gc.IsNil)

	setter := &apiAddressSetter{servers: make(chan [][]instance.HostPort, 1)}
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	worker := apiaddressupdater.NewAPIAddressUpdater(st.Machiner(), setter)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	// SetAPIHostPorts should be called with the initial value.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called")
	case servers := <-setter.servers:
		c.Assert(servers, gc.DeepEquals, updatedServers)
	}
}

func (s *APIAddressUpdaterSuite) TestAddressChange(c *gc.C) {
	setter := &apiAddressSetter{servers: make(chan [][]instance.HostPort, 1)}
	st, _ := s.OpenAPIAsNewMachine(c, state.JobHostUnits)
	worker := apiaddressupdater.NewAPIAddressUpdater(st.Machiner(), setter)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()
	s.BackingState.StartSync()
	updatedServers := [][]instance.HostPort{instance.AddressesWithPort(
		instance.NewAddresses("localhost", "127.0.0.1"),
		1234,
	)}
	// SetAPIHostPorts should be called with the initial value (empty),
	// and then the updated value.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called first")
	case servers := <-setter.servers:
		c.Assert(servers, gc.HasLen, 0)
	}
	err := s.State.SetAPIHostPorts(updatedServers)
	c.Assert(err, gc.IsNil)
	s.BackingState.StartSync()
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for SetAPIHostPorts to be called second")
	case servers := <-setter.servers:
		c.Assert(servers, gc.DeepEquals, updatedServers)
	}
}
