// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

type instanceTest struct {
	providerSuite
}

var _ = gc.Suite(&instanceTest{})

func (s *instanceTest) TestId(c *gc.C) {
	jsonValue := `{"system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	resourceURI, _ := obj.GetField("resource_uri")
	instance := maasInstance{maasObject: &obj, environ: s.makeEnviron()}

	c.Check(string(instance.Id()), gc.Equals, resourceURI)
}

func (s *instanceTest) TestString(c *gc.C) {
	jsonValue := `{"hostname": "thethingintheplace", "system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	instance := &maasInstance{maasObject: &obj, environ: s.makeEnviron()}
	hostname, err := instance.hostname()
	c.Assert(err, jc.ErrorIsNil)
	expected := hostname + ":" + string(instance.Id())
	c.Assert(fmt.Sprint(instance), gc.Equals, expected)
}

func (s *instanceTest) TestStringWithoutHostname(c *gc.C) {
	// For good measure, test what happens if we don't have a hostname.
	jsonValue := `{"system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	instance := &maasInstance{maasObject: &obj, environ: s.makeEnviron()}
	_, err := instance.hostname()
	c.Assert(err, gc.NotNil)
	expected := fmt.Sprintf("<DNSName failed: %q>", err) + ":" + string(instance.Id())
	c.Assert(fmt.Sprint(instance), gc.Equals, expected)
}

func (s *instanceTest) TestRefreshInstance(c *gc.C) {
	jsonValue := `{"system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	s.testMAASObject.TestServer.ChangeNode("system_id", "test2", "test2")
	instance := maasInstance{maasObject: &obj, environ: s.makeEnviron()}

	err := instance.Refresh()

	c.Check(err, jc.ErrorIsNil)
	testField, err := (*instance.maasObject).GetField("test2")
	c.Check(err, jc.ErrorIsNil)
	c.Check(testField, gc.Equals, "test2")
}

func (s *instanceTest) TestAddresses(c *gc.C) {
	jsonValue := `{
			"hostname": "testing.invalid",
			"system_id": "system_id",
			"ip_addresses": [ "1.2.3.4", "fe80::d806:dbff:fe23:1199" ]
		}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	inst := maasInstance{maasObject: &obj, environ: s.makeEnviron()}

	expected := []network.Address{
		network.NewScopedAddress("testing.invalid", network.ScopePublic),
		network.NewScopedAddress("testing.invalid", network.ScopeCloudLocal),
		network.NewAddress("1.2.3.4"),
		network.NewAddress("fe80::d806:dbff:fe23:1199"),
	}

	addr, err := inst.Addresses()

	c.Assert(err, jc.ErrorIsNil)
	c.Check(addr, gc.DeepEquals, expected)
}

func (s *instanceTest) TestAddressesMissing(c *gc.C) {
	// Older MAAS versions do not have ip_addresses returned, for these
	// just the DNS name should be returned without error.
	jsonValue := `{
		"hostname": "testing.invalid",
		"system_id": "system_id"
		}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	inst := maasInstance{maasObject: &obj, environ: s.makeEnviron()}

	addr, err := inst.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(addr, gc.DeepEquals, []network.Address{
		{Value: "testing.invalid", Type: network.HostName, Scope: network.ScopePublic},
		{Value: "testing.invalid", Type: network.HostName, Scope: network.ScopeCloudLocal},
	})
}

func (s *instanceTest) TestAddressesInvalid(c *gc.C) {
	jsonValue := `{
		"hostname": "testing.invalid",
		"system_id": "system_id",
		"ip_addresses": "incompatible"
		}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	inst := maasInstance{maasObject: &obj, environ: s.makeEnviron()}

	_, err := inst.Addresses()
	c.Assert(err, gc.NotNil)
}

func (s *instanceTest) TestAddressesInvalidContents(c *gc.C) {
	jsonValue := `{
		"hostname": "testing.invalid",
		"system_id": "system_id",
		"ip_addresses": [42]
		}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	inst := maasInstance{maasObject: &obj, environ: s.makeEnviron()}

	_, err := inst.Addresses()
	c.Assert(err, gc.NotNil)
}

func (s *instanceTest) TestHardwareCharacteristics(c *gc.C) {
	jsonValue := `{
		"system_id": "system_id",
        "architecture": "amd64/generic",
        "cpu_count": 6,
        "memory": 16384
	}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	inst := maasInstance{maasObject: &obj, environ: s.makeEnviron()}
	hc, err := inst.hardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hc, gc.NotNil)
	c.Assert(hc.String(), gc.Equals, `arch=amd64 cpu-cores=6 mem=16384M`)
}

func (s *instanceTest) TestHardwareCharacteristicsWithTags(c *gc.C) {
	jsonValue := `{
		"system_id": "system_id",
        "architecture": "amd64/generic",
        "cpu_count": 6,
        "memory": 16384,
        "tag_names": ["a", "b"]
	}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	inst := maasInstance{maasObject: &obj, environ: s.makeEnviron()}
	hc, err := inst.hardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hc, gc.NotNil)
	c.Assert(hc.String(), gc.Equals, `arch=amd64 cpu-cores=6 mem=16384M tags=a,b`)
}

func (s *instanceTest) TestHardwareCharacteristicsMissing(c *gc.C) {
	s.testHardwareCharacteristicsMissing(c, `{"system_id": "id", "cpu_count": 6, "memory": 16384}`,
		`error determining architecture: Requested string, got <nil>.`)
	s.testHardwareCharacteristicsMissing(c, `{"system_id": "id", "architecture": "amd64", "memory": 16384}`,
		`error determining cpu count: Requested float64, got <nil>.`)
	s.testHardwareCharacteristicsMissing(c, `{"system_id": "id", "architecture": "armhf", "cpu_count": 6}`,
		`error determining available memory: Requested float64, got <nil>.`)
	s.testHardwareCharacteristicsMissing(c, `{"system_id": "id", "architecture": "armhf", "cpu_count": 6, "memory": 1, "tag_names": "wot"}`,
		`error determining tag names: Requested array, got string.`)
}

func (s *instanceTest) testHardwareCharacteristicsMissing(c *gc.C, json, expect string) {
	obj := s.testMAASObject.TestServer.NewNode(json)
	inst := maasInstance{maasObject: &obj, environ: s.makeEnviron()}
	_, err := inst.hardwareCharacteristics()
	c.Assert(err, gc.ErrorMatches, expect)
}
