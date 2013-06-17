// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc_test

import (
	"fmt"
	stdtesting "testing"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/container/lxc"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type LxcSuite struct {
	testing.LoggingSuite
	testing.MgoSuite
	home               *testing.FakeHome
	oldContainerDir    string
	oldLxcContainerDir string
}

var _ = Suite(&LxcSuite{})

func (s *LxcSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *LxcSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *LxcSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.home = testing.MakeSampleHome(c)
	s.oldContainerDir = lxc.SetContainerDir(c.MkDir())
	s.oldLxcContainerDir = lxc.SetLxcContainerDir(c.MkDir())
}

func (s *LxcSuite) TearDownTest(c *C) {
	lxc.SetContainerDir(s.oldContainerDir)
	lxc.SetLxcContainerDir(s.oldLxcContainerDir)
	s.home.Restore()
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *LxcSuite) TestNewContainer(c *C) {
	factory := lxc.NewFactory(MockFactory())
	container, err := factory.NewContainer("2/lxc/0")
	c.Assert(err, IsNil)
	c.Assert(container.Id(), Equals, instance.Id("machine-2-lxc-0"))
	machineId, ok := lxc.GetMachineId(container)
	c.Assert(ok, Equals, true)
	c.Assert(machineId, Equals, "2/lxc/0")
}

func (s *LxcSuite) TestNewFromExisting(c *C) {
	mock := MockFactory()
	mockLxc := mock.New("machine-1-lxc-0")
	factory := lxc.NewFactory(mock)
	container, err := factory.NewFromExisting(mockLxc)
	c.Assert(err, IsNil)
	c.Assert(container.Id(), Equals, instance.Id("machine-1-lxc-0"))
	machineId, ok := lxc.GetMachineId(container)
	c.Assert(ok, Equals, true)
	c.Assert(machineId, Equals, "1/lxc/0")
}

func setupAuthentication(st *state.State, machine *state.Machine) (*state.Info, *api.Info, error) {
	stateAddresses, err := st.Addresses()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot get addresses from state: %v", err)
	}
	apiAddresses, err := st.APIAddresses()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot get api addresses from state: %v", err)
	}
	password, err := utils.RandomPassword()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot make password for machine %v: %v", machine, err)
	}
	if err := machine.SetMongoPassword(password); err != nil {
		return nil, nil, fmt.Errorf("cannot set password for machine %v: %v", machine, err)
	}
	cert := st.CACert()
	return &state.Info{
			Addrs:    stateAddresses,
			CACert:   cert,
			Tag:      machine.Tag(),
			Password: password,
		}, &api.Info{
			Addrs:    apiAddresses,
			CACert:   cert,
			Tag:      machine.Tag(),
			Password: password,
		}, nil
}

func (s *LxcSuite) TestContainerCreate(c *C) {

	machineId := "1/lxc/0"
	config := testing.EnvironConfig(c)
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)

	factory := lxc.NewFactory(MockFactory())
	container, err := factory.NewContainer(machineId)
	c.Assert(err, IsNil)

	series := "series"
	nonce := "fake-nonce"
	tools := &state.Tools{
		Binary: version.MustParseBinary("2.3.4-foo-bar"),
		URL:    "http://tools.example.com/2.3.4-foo-bar.tgz",
	}

	container.Create(series, nonce, tools, config, stateInfo, apiInfo)

}
