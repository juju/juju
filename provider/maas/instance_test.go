// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"

	"github.com/juju/gomaasapi"
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
	instance := maasInstance{&obj}

	c.Check(string(instance.Id()), gc.Equals, resourceURI)
}

func (s *instanceTest) TestString(c *gc.C) {
	jsonValue := `{"hostname": "thethingintheplace", "system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	instance := &maasInstance{&obj}
	hostname, err := instance.hostname()
	c.Assert(err, jc.ErrorIsNil)
	expected := hostname + ":" + string(instance.Id())
	c.Assert(fmt.Sprint(instance), gc.Equals, expected)
}

func (s *instanceTest) TestStringWithoutHostname(c *gc.C) {
	// For good measure, test what happens if we don't have a hostname.
	jsonValue := `{"system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	instance := &maasInstance{&obj}
	_, err := instance.hostname()
	c.Assert(err, gc.NotNil)
	expected := fmt.Sprintf("<DNSName failed: %q>", err) + ":" + string(instance.Id())
	c.Assert(fmt.Sprint(instance), gc.Equals, expected)
}

func (s *instanceTest) TestAddressesLegacy(c *gc.C) {
	// We simulate an older MAAS (1.8-) which returns ip_addresses, but no
	// interface_set for a node. We also verify we don't get the space of an
	// address.
	jsonValue := `{
			"hostname": "testing.invalid",
			"system_id": "system_id",
			"ip_addresses": [ "1.2.3.4", "fe80::d806:dbff:fe23:1199" ]
		}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	inst := maasInstance{&obj}

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

func (s *instanceTest) TestAddressesViaInterfaces(c *gc.C) {
	// We simulate an newer MAAS (1.9+) which returns both ip_addresses and
	// interface_set for a node. To verify we use interfaces we deliberately put
	// different items in ip_addresses
	jsonValue := `{
			"hostname": "-testing.invalid",
			"system_id": "system_id",
            "interface_set" : [
              { "name": "eth0", "links": [
                  { "subnet": { "space": "bar" }, "ip_address": "8.7.6.5" },
                  { "subnet": { "space": "bar" }, "ip_address": "8.7.6.6" }
              ] },
              { "name": "eth1", "links": [
                  { "subnet": { "space": "storage" }, "ip_address": "10.0.1.1" }
               ] },
              { "name": "eth3", "links": [
                  { "subnet": { "space": "db" }, "ip_address": "fc00::123" }
               ] },
              { "name": "eth4" },
              { "name": "eth5", "links": [
                  { "mode": "link-up" }
               ] }
           ],
			"ip_addresses": [ "anything", "foo", "0.1.2.3" ]
		}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	inst := maasInstance{&obj}
	// Since gomaasapi treats "interface_set" specially and the only way to
	// change it is via SetNodeNetworkLink(), which in turn does not allow you
	// to specify ip_address, we need to patch the call which gets a fresh copy
	// of the node details from the test server to avoid manging the
	// interface_set we used above.
	s.PatchValue(&refreshMAASObject, func(mo *gomaasapi.MAASObject) (gomaasapi.MAASObject, error) {
		return *mo, nil
	})

	expected := []network.Address{
		network.NewAddressOnSpace("bar", "8.7.6.5"),
		network.NewAddressOnSpace("bar", "8.7.6.6"),
		network.NewAddressOnSpace("storage", "10.0.1.1"),
		network.NewAddressOnSpace("db", "fc00::123"),
	}

	addr, err := inst.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(addr, jc.DeepEquals, expected)
}

func (s *instanceTest) TestAddressesMissing(c *gc.C) {
	// Older MAAS versions do not have ip_addresses returned, for these
	// just the DNS name should be returned without error.
	jsonValue := `{
		"hostname": "testing.invalid",
		"system_id": "system_id"
		}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	inst := maasInstance{&obj}

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
	inst := maasInstance{&obj}

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
	inst := maasInstance{&obj}

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
	inst := maasInstance{&obj}
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
	inst := maasInstance{&obj}
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
	inst := maasInstance{&obj}
	_, err := inst.hardwareCharacteristics()
	c.Assert(err, gc.ErrorMatches, expect)
}
