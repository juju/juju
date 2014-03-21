// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

type APIAddresserSuite struct {
	
}

type APIAddresserFacade interface {
	APIAddresses() ([]string, error)
}

func (s *APIAddresserSuite) SetUpSuite(c *gc.C, jcSuite testing.JujuConnSuite) {
}


func (s *deployerSuite) TestAPIAddresses(c *gc.C) {
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
