// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
)

type instanceTest struct {
	providerSuite
}

var _ = gc.Suite(&instanceTest{})

func defaultSubnet() gomaasapi.CreateSubnet {
	var s gomaasapi.CreateSubnet
	s.DNSServers = []string{"192.168.1.2"}
	s.Name = "maas-eth0"
	s.Space = "space-0"
	s.GatewayIP = "192.168.1.1"
	s.CIDR = "192.168.1.0/24"
	s.ID = 1
	return s
}

func (s *instanceTest) newSubnet(cidr, space string, id uint) *bytes.Buffer {
	var sub gomaasapi.CreateSubnet
	sub.DNSServers = []string{"192.168.1.2"}
	sub.Name = cidr
	sub.Space = space
	sub.GatewayIP = "192.168.1.1"
	sub.CIDR = cidr
	sub.ID = id
	return s.subnetJSON(sub)
}

func (s *instanceTest) subnetJSON(subnet gomaasapi.CreateSubnet) *bytes.Buffer {
	var out bytes.Buffer
	err := json.NewEncoder(&out).Encode(subnet)
	if err != nil {
		panic(err)
	}
	return &out
}

func (s *instanceTest) SetUpTest(c *gc.C) {
	s.providerSuite.SetUpTest(c)

	// Create a subnet so that the spaces cache will be populated.
	s.testMAASObject.TestServer.NewSubnet(s.subnetJSON(defaultSubnet()))
}

func (s *instanceTest) TestId(c *gc.C) {
	jsonValue := `{"system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	resourceURI, _ := obj.GetField("resource_uri")
	// TODO(perrito666) make a decent mock status getter
	statusGetter := func(context.ProviderCallContext, instance.Id) (string, string) {
		return "unknown", "FAKE"
	}
	instance := maas1Instance{&obj, nil, statusGetter}

	c.Check(string(instance.Id()), gc.Equals, resourceURI)
}

func (s *instanceTest) TestString(c *gc.C) {
	jsonValue := `{"hostname": "thethingintheplace", "system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	statusGetter := func(context.ProviderCallContext, instance.Id) (string, string) {
		return "unknown", "FAKE"
	}

	instance := &maas1Instance{&obj, nil, statusGetter}
	hostname, err := instance.hostname()
	c.Assert(err, jc.ErrorIsNil)
	expected := hostname + ":" + string(instance.Id())
	c.Assert(fmt.Sprint(instance), gc.Equals, expected)
}

func (s *instanceTest) TestStringWithoutHostname(c *gc.C) {
	// For good measure, test what happens if we don't have a hostname.
	jsonValue := `{"system_id": "system_id", "test": "test"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	statusGetter := func(context.ProviderCallContext, instance.Id) (string, string) {
		return "unknown", "FAKE"
	}

	instance := &maas1Instance{&obj, nil, statusGetter}
	_, err := instance.hostname()
	c.Assert(err, gc.NotNil)
	expected := fmt.Sprintf("<DNSName failed: %q>", err) + ":" + string(instance.Id())
	c.Assert(fmt.Sprint(instance), gc.Equals, expected)
}

func (s *instanceTest) TestDisplayNameIsHostname(c *gc.C) {
	jsonValue := `{"system_id": "system_id", "test": "test", "hostname": "abc.internal"}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)

	instance := &maas1Instance{&obj, nil, nil}
	displayName, err := instance.displayName()
	c.Assert(err, gc.IsNil)
	c.Assert(displayName, gc.Equals, "abc.internal")
}

// Note: no need to test displayName() without hostname, as Juju defaults to presenting the
// instance.Id() when a display name is unavailable

func (s *instanceTest) TestAddressesViaInterfaces(c *gc.C) {
	server := s.testMAASObject.TestServer
	server.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	// We simulate an newer MAAS (1.9+) which returns both ip_addresses and
	// interface_set for a node. To verify we use interfaces we deliberately put
	// different items in ip_addresses
	jsonValue := `{
    "hostname": "-testing.invalid",
    "system_id": "system_id",
    "interface_set" : [
	{ "name": "eth0", "links": [
	    { "subnet": { "space": "bar", "cidr": "8.7.6.0/24" }, "ip_address": "8.7.6.5" },
	    { "subnet": { "space": "bar", "cidr": "8.7.6.0/24"  }, "ip_address": "8.7.6.6" }
	] },
	{ "name": "eth1", "links": [
	    { "subnet": { "space": "storage", "cidr": "10.0.1.1/24" }, "ip_address": "10.0.1.1" }
	] },
	{ "name": "eth3", "links": [
	    { "subnet": { "space": "db", "cidr": "fc00::/64" }, "ip_address": "fc00::123" }
	] },
	{ "name": "eth4" },
	{ "name": "eth5", "links": [
	    { "mode": "link-up" }
	] }
    ],
    "ip_addresses": [ "anything", "foo", "0.1.2.3" ]
}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	statusGetter := func(context.ProviderCallContext, instance.Id) (string, string) {
		return "unknown", "FAKE"
	}

	barSpace := server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "bar"}))
	storageSpace := server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "storage"}))
	dbSpace := server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "db"}))
	server.NewSubnet(s.newSubnet("8.7.6.0/24", "bar", 2))
	server.NewSubnet(s.newSubnet("10.0.1.1/24", "storage", 3))
	server.NewSubnet(s.newSubnet("fc00::/64", "db", 4))
	inst := maas1Instance{&obj, s.makeEnviron(), statusGetter}

	// Since gomaasapi treats "interface_set" specially and the only way to
	// change it is via SetNodeNetworkLink(), which in turn does not allow you
	// to specify ip_address, we need to patch the call which gets a fresh copy
	// of the node details from the test server to avoid mangling the
	// interface_set we used above.
	s.PatchValue(&refreshMAASObject, func(mo *gomaasapi.MAASObject) (gomaasapi.MAASObject, error) {
		return *mo, nil
	})

	idFromUint := func(u uint) corenetwork.Id {
		return corenetwork.Id(fmt.Sprintf("%d", u))
	}
	expected := []network.Address{
		newAddressOnSpaceWithId("bar", idFromUint(barSpace.ID), "8.7.6.5"),
		newAddressOnSpaceWithId("bar", idFromUint(barSpace.ID), "8.7.6.6"),
		newAddressOnSpaceWithId("storage", idFromUint(storageSpace.ID), "10.0.1.1"),
		newAddressOnSpaceWithId("db", idFromUint(dbSpace.ID), "fc00::123"),
	}

	addr, err := inst.Addresses(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(addr, jc.DeepEquals, expected)
}

func (s *instanceTest) TestAddressesInvalid(c *gc.C) {
	jsonValue := `{
		"hostname": "testing.invalid",
		"system_id": "system_id",
		"ip_addresses": "incompatible"
		}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	statusGetter := func(context.ProviderCallContext, instance.Id) (string, string) {
		return "unknown", "FAKE"
	}

	inst := maas1Instance{&obj, s.makeEnviron(), statusGetter}

	_, err := inst.Addresses(s.callCtx)
	c.Assert(err, gc.NotNil)
}

func (s *instanceTest) TestAddressesInvalidContents(c *gc.C) {
	jsonValue := `{
		"hostname": "testing.invalid",
		"system_id": "system_id",
		"ip_addresses": [42]
		}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	statusGetter := func(context.ProviderCallContext, instance.Id) (string, string) {
		return "unknown", "FAKE"
	}

	inst := maas1Instance{&obj, s.makeEnviron(), statusGetter}

	_, err := inst.Addresses(s.callCtx)
	c.Assert(err, gc.NotNil)
}

func (s *instanceTest) TestHardwareCharacteristics(c *gc.C) {
	jsonValue := `{
		"system_id": "system_id",
        "architecture": "amd64/generic",
        "cpu_count": 6,
        "zone": {"name": "tst"},
        "memory": 16384
	}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	statusGetter := func(context.ProviderCallContext, instance.Id) (string, string) {
		return "unknown", "FAKE"
	}

	inst := maas1Instance{&obj, nil, statusGetter}
	hc, err := inst.hardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hc, gc.NotNil)
	c.Assert(hc.String(), gc.Equals, `arch=amd64 cores=6 mem=16384M availability-zone=tst`)
}

func (s *instanceTest) TestHardwareCharacteristicsWithTags(c *gc.C) {
	jsonValue := `{
		"system_id": "system_id",
        "architecture": "amd64/generic",
        "cpu_count": 6,
        "memory": 16384,
        "zone": {"name": "tst"},
        "tag_names": ["a", "b"]
	}`
	obj := s.testMAASObject.TestServer.NewNode(jsonValue)
	statusGetter := func(context.ProviderCallContext, instance.Id) (string, string) {
		return "unknown", "FAKE"
	}

	inst := maas1Instance{&obj, nil, statusGetter}
	hc, err := inst.hardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hc, gc.NotNil)
	c.Assert(hc.String(), gc.Equals, `arch=amd64 cores=6 mem=16384M tags=a,b availability-zone=tst`)
}

func (s *instanceTest) TestHardwareCharacteristicsMissing(c *gc.C) {
	s.testHardwareCharacteristicsMissing(c, `{"system_id": "id", "cpu_count": 6, "memory": 16384}`,
		`error determining architecture: Requested string, got <nil>.`)
	s.testHardwareCharacteristicsMissing(c, `{"system_id": "id", "architecture": "amd64", "memory": 16384}`,
		`error determining cpu count: Requested float64, got <nil>.`)
	s.testHardwareCharacteristicsMissing(c, `{"system_id": "id", "architecture": "armhf", "cpu_count": 6}`,
		`error determining available memory: Requested float64, got <nil>.`)
	s.testHardwareCharacteristicsMissing(c, `{"system_id": "id", "architecture": "armhf", "cpu_count": 6, "memory": 1}`,
		`error determining availability zone: zone property not set on maas`)
	s.testHardwareCharacteristicsMissing(c, `{"system_id": "id", "architecture": "armhf", "cpu_count": 6, "memory": 1, "zone": ""}`,
		`error determining availability zone: zone property is not an expected type`)
	s.testHardwareCharacteristicsMissing(c, `{"system_id": "id", "architecture": "armhf", "cpu_count": 6, "memory": 1, "zone": {}}`,
		`error determining availability zone: zone property is not set correctly: name is missing`)
	s.testHardwareCharacteristicsMissing(c, `{"system_id": "id", "architecture": "armhf", "cpu_count": 6, "memory": 1, "zone": {"name": "tst"}, "tag_names": "wot"}`,
		`error determining tag names: Requested array, got string.`)
}

func (s *instanceTest) testHardwareCharacteristicsMissing(c *gc.C, json, expect string) {
	obj := s.testMAASObject.TestServer.NewNode(json)
	statusGetter := func(context.ProviderCallContext, instance.Id) (string, string) {
		return "unknown", "FAKE"
	}

	inst := maas1Instance{&obj, nil, statusGetter}
	_, err := inst.hardwareCharacteristics()
	c.Assert(err, gc.ErrorMatches, expect)
}
