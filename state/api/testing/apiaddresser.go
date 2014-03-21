// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package testing

type APIAddresserSuite struct {
	st *state.State
	facade APIAddresserFacade
}

type APIAddresserFacade interface {
	APIAddresses() ([]string, error)
	CACert() ([]byte, error)
	APIHostPorts() ([][]instance.HostPort, error)
	WatchAPIHostPorts() (watcher.NotifyWatcher, error)
}

func NewAPIAddresserSuite(st *state.State, facade APIAddresserFacade) *APIAddresserSuite {
	return &APIAddresserSuite{
		st: st,
		facade: facade,
	}
}

func (s *APIAddresserSuite) SetUpSuite(c *gc.C) {
}

func (s *APIAddresserSuite) TearDownSuite(c *gc.C) {
}

func (s *APIAddresserSuite) SetUpTest(c *gc.C) {
}

func (s *APIAddresserSuite) TearDownTest(c *gc.C) {
}

func (s *APIAddresserSuite) TestAPIAddresses(c *gc.C) {
	apiAddresses, err := s.State.APIAddressesFromMachines()
	c.Assert(err, gc.IsNil)

	addresses, err := s.facade.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.DeepEquals, apiAddresses)
}

func (s *APIAddresserSuite) TestAPIHostPorts(c *gc.C) {
	apiHostPorts, err := s.State.APIHostPorts()
	c.Assert(err, gc.IsNil)

	
	c.Assert(apiHostPorts, gc.DeepEquals, apiH
}

	addrs := []instance.Address{
		instance.NewAddress("0.1.2.3"),
	}
	err := s.machine.SetAddresses(addrs)
	c.Assert(err, gc.IsNil)

	stateAddresses, err := s.State.APIAddressesFromMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(len(stateAddresses), gc.Equals, 1)

	addresses, err := s.st.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.DeepEquals, stateAddresses)
}

func (s *deployerSuite) TestCACert(c *gc.C) {
	caCert, err := s.st.CACert()
	c.Assert(err, gc.IsNil)
	c.Assert(caCert, gc.DeepEquals, s.State.CACert())
}
