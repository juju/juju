// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/golxc"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/container/lxc/mock"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker/provisioner"
)

type lxcBrokerSuite struct {
	testing.LoggingSuite
	golxc              golxc.ContainerFactory
	broker             provisioner.Broker
	containerDir       string
	removedDir         string
	lxcDir             string
	oldContainerDir    string
	oldRemovedDir      string
	oldLxcContainerDir string
}

var _ = Suite(&lxcBrokerSuite{})

func (s *lxcBrokerSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *lxcBrokerSuite) TearDownSuite(c *C) {
	s.LoggingSuite.TearDownSuite(c)
}

func (s *lxcBrokerSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.containerDir = c.MkDir()
	s.oldContainerDir = lxc.SetContainerDir(s.containerDir)
	s.removedDir = c.MkDir()
	s.oldRemovedDir = lxc.SetRemovedContainerDir(s.removedDir)
	s.lxcDir = c.MkDir()
	s.oldLxcContainerDir = lxc.SetLxcContainerDir(s.lxcDir)
	s.golxc = mock.MockFactory()
	tools := &state.Tools{
		Binary: version.MustParseBinary("2.3.4-foo-bar"),
		URL:    "http://tools.example.com/2.3.4-foo-bar.tgz",
	}
	s.broker = provisioner.NewLxcBroker(s.golxc, testing.EnvironConfig(c), tools)
}

func (s *lxcBrokerSuite) TearDownTest(c *C) {
	lxc.SetContainerDir(s.oldContainerDir)
	lxc.SetLxcContainerDir(s.oldLxcContainerDir)
	lxc.SetRemovedContainerDir(s.oldRemovedDir)
	s.LoggingSuite.TearDownTest(c)
}

func (s *lxcBrokerSuite) startInstance(c *C, machineId string) instance.Instance {
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)

	series := "series"
	nonce := "fake-nonce"
	cons := constraints.Value{}
	lxc, err := s.broker.StartInstance(machineId, nonce, series, cons, stateInfo, apiInfo)
	c.Assert(err, IsNil)
	return lxc
}

func (s *lxcBrokerSuite) TestStartInstance(c *C) {
	machineId := "1/lxc/0"
	lxc := s.startInstance(c, machineId)
	c.Assert(lxc.Id(), Equals, instance.Id("machine-1-lxc-0"))
}

func (s *lxcBrokerSuite) TestStopInstance(c *C) {
	lxc0 := s.startInstance(c, "1/lxc/0")
	lxc1 := s.startInstance(c, "1/lxc/1")
	lxc2 := s.startInstance(c, "1/lxc/2")
	err := s.broker.StopInstances([]instance.Instance{lxc0, lxc1, lxc2})
	c.Assert(err, IsNil)
}

func (s *lxcBrokerSuite) matchInstances(c *C, result []instance.Instance, expected ...instance.Instance) {
	resultMap := make(map[instance.Id]instance.Instance)
	for _, i := range result {
		resultMap[i.Id()] = i
	}

	expectedMap := make(map[instance.Id]instance.Instance)
	for _, i := range expected {
		expectedMap[i.Id()] = i
	}
	c.Assert(resultMap, DeepEquals, expectedMap)
}

func (s *lxcBrokerSuite) TestAllInstances(c *C) {
	lxc0 := s.startInstance(c, "1/lxc/0")
	lxc1 := s.startInstance(c, "1/lxc/1")
	results, err := s.broker.AllInstances()
	c.Assert(err, IsNil)
	s.matchInstances(c, results, lxc0, lxc1)

	err = s.broker.StopInstances([]instance.Instance{lxc1})
	c.Assert(err, IsNil)
	lxc2 := s.startInstance(c, "1/lxc/2")
	results, err = s.broker.AllInstances()
	c.Assert(err, IsNil)
	s.matchInstances(c, results, lxc0, lxc2)
}
