package maas

import (
	. "launchpad.net/gocheck"
)

func (s *_MAASProviderTestSuite) TestId(c *C) {
	obj := s.environ._MAASServerUnlocked.GetSubObject("nodes").GetSubObject("system_id")
	resourceURI, _ := obj.GetField("resource_uri")
	instance := maasInstance{&obj, s.environ}

	c.Check(string(instance.Id()), Equals, resourceURI)
}

func (s *_MAASProviderTestSuite) TestRefreshInstance(c *C) {
	jsonValue := `{"system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	s.testMAASObject.TestServer.ChangeNode("system_id", "test2", "test2")
	instance := maasInstance{&obj, s.environ}

	err := instance.refreshInstance()

	c.Check(err, IsNil)
	testField, err := (*instance.maasobject).GetField("test2")
	c.Check(err, IsNil)
	c.Check(testField, Equals, "test2")
}

func (s *_MAASProviderTestSuite) TestDNSName(c *C) {
	jsonValue := `{"hostname": "old DNS name", "system_id": "system_id"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	s.testMAASObject.TestServer.ChangeNode("system_id", "hostname", "new DNS name")
	instance := maasInstance{&obj, s.environ}

	dnsName, err := instance.DNSName()

	c.Check(err, IsNil)
	c.Check(dnsName, Equals, "new DNS name")

	// WaitDNSName() currently simply calls DNSName().
	s.testMAASObject.TestServer.ChangeNode("system_id", "hostname", "new DNS name 2")

	dnsName, err = instance.WaitDNSName()

	c.Check(err, IsNil)
	c.Check(dnsName, Equals, "new DNS name 2")

}
