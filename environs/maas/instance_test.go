// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	gc "launchpad.net/gocheck"
)

type instanceTest struct {
	providerSuite
}

var _ = gc.Suite(&instanceTest{})

func (s *instanceTest) TestId(c *gc.C) {
	jsonValue := `{"system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	resourceURI, _ := obj.GetField("resource_uri")
	instance := maasInstance{&obj, s.environ}

	c.Check(string(instance.Id()), gc.Equals, resourceURI)
}

func (s *instanceTest) TestRefreshInstance(c *gc.C) {
	jsonValue := `{"system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	s.testMAASObject.TestServer.ChangeNode("system_id", "test2", "test2")
	instance := maasInstance{&obj, s.environ}

	err := instance.refreshInstance()

	c.Check(err, gc.IsNil)
	testField, err := (*instance.maasObject).GetField("test2")
	c.Check(err, gc.IsNil)
	c.Check(testField, gc.Equals, "test2")
}

func (s *instanceTest) TestDNSName(c *gc.C) {
	jsonValue := `{"hostname": "DNS name", "system_id": "system_id"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	instance := maasInstance{&obj, s.environ}

	dnsName, err := instance.DNSName()

	c.Check(err, gc.IsNil)
	c.Check(dnsName, gc.Equals, "DNS name")

	// WaitDNSName() currently simply calls DNSName().
	dnsName, err = instance.WaitDNSName()

	c.Check(err, gc.IsNil)
	c.Check(dnsName, gc.Equals, "DNS name")
}
