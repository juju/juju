// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	. "launchpad.net/gocheck"
)

type InstanceTest struct {
	ProviderSuite
}

var _ = Suite(&InstanceTest{})

func (s *InstanceTest) TestId(c *C) {
	jsonValue := `{"system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	resourceURI, _ := obj.GetField("resource_uri")
	instance := maasInstance{&obj, s.environ}

	c.Check(string(instance.Id()), Equals, resourceURI)
}

func (s *InstanceTest) TestRefreshInstance(c *C) {
	jsonValue := `{"system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	s.testMAASObject.TestServer.ChangeNode("system_id", "test2", "test2")
	instance := maasInstance{&obj, s.environ}

	err := instance.refreshInstance()

	c.Check(err, IsNil)
	testField, err := (*instance.maasObject).GetField("test2")
	c.Check(err, IsNil)
	c.Check(testField, Equals, "test2")
}

func (s *InstanceTest) TestDNSName(c *C) {
	jsonValue := `{"hostname": "DNS name", "system_id": "system_id"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	instance := maasInstance{&obj, s.environ}

	dnsName, err := instance.DNSName()

	c.Check(err, IsNil)
	c.Check(dnsName, Equals, "DNS name")

	// WaitDNSName() currently simply calls DNSName().
	dnsName, err = instance.WaitDNSName()

	c.Check(err, IsNil)
	c.Check(dnsName, Equals, "DNS name")
}
