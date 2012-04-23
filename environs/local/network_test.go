package local

import . "launchpad.net/gocheck"

func (s *S) TestStartNetwork(c *C) {
	n := network{Name: "default"}
	err := n.start()
	c.Assert(err, IsNil)
}

func (s *S) TestLoadAttributes(c *C) {
	n := network{Name: "default"}
	i := ip{Address: "192.168.122.1", Netmask: "255.255.255.0"}
	b := bridge{Name: "virbr0"}

	err := n.loadAttributes()
	c.Assert(err, IsNil)
	c.Assert(n.Name, Equals, "default")
	c.Assert(n.Bridge.Name, Equals, b.Name)
	c.Assert(n.Ip.Address, Equals, i.Address)
	c.Assert(n.Ip.Netmask, Equals, i.Netmask)
}

func (s *S) TestRunning(c *C) {
	n := network{Name: "default"}
	running := n.running()
	c.Assert(running, Equals, true)

	n = network{Name: "fakeName"}
	running = n.running()
	c.Assert(running, Equals, false)
}

func (s *S) TestNetworkExists(c *C) {
	n := network{Name: "default"}
	exists := n.exists()
	c.Assert(exists, Equals, true)

	n = network{Name: "fakeName"}
	exists = n.exists()
	c.Assert(exists, Equals, false)
}

func (s *S) TestListNetworks(c *C) {
	expected := map[string]bool{"default": true}
	networks, err := listNetworks()
	c.Assert(err, IsNil)
	c.Assert(networks, DeepEquals, expected)
}
