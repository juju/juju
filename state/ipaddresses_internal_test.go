// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
)

// ipAddressesInternalSuite contains black-box tests for IP addresses'
// internals, which do not actually access mongo. The rest of the logic is
// tested in ipAddressesStateSuite.
type ipAddressesInternalSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ipAddressesInternalSuite{})

func (s *ipAddressesInternalSuite) TestNewIPAddressCreatesAddress(c *gc.C) {
	result := newIPAddress(nil, ipAddressDoc{})
	c.Assert(result, gc.NotNil)
	c.Assert(result.st, gc.IsNil)
	c.Assert(result.doc, jc.DeepEquals, ipAddressDoc{})
}

func (s *ipAddressesInternalSuite) TestDocIDIncludesModelUUID(c *gc.C) {
	const localDocID = "foo"
	globalDocID := coretesting.ModelTag.Id() + ":" + localDocID

	result := s.newIPAddressWithDummyState(ipAddressDoc{DocID: localDocID})
	c.Assert(result.DocID(), gc.Equals, globalDocID)

	result = s.newIPAddressWithDummyState(ipAddressDoc{DocID: globalDocID})
	c.Assert(result.DocID(), gc.Equals, globalDocID)
}

func (s *ipAddressesInternalSuite) newIPAddressWithDummyState(doc ipAddressDoc) *Address {
	// We only need the model UUID set for localID() and docID() to work.
	// The rest is tested in ipAddressesStateSuite.
	dummyState := &State{modelTag: coretesting.ModelTag}
	return newIPAddress(dummyState, doc)
}

func (s *ipAddressesInternalSuite) TestProviderIDIsEmptyWhenNotSet(c *gc.C) {
	result := s.newIPAddressWithDummyState(ipAddressDoc{})
	c.Assert(result.ProviderID(), gc.Equals, corenetwork.Id(""))
}

func (s *ipAddressesInternalSuite) TestProviderID(c *gc.C) {
	result := s.newIPAddressWithDummyState(ipAddressDoc{ProviderID: "foo"})
	c.Assert(result.ProviderID(), gc.Equals, corenetwork.Id("foo"))
}

func (s *ipAddressesInternalSuite) TestIPAddressGlobalKeyHelper(c *gc.C) {
	result := ipAddressGlobalKey("42", "eth0", "0.1.2.3")
	c.Assert(result, gc.Equals, "m#42#d#eth0#ip#0.1.2.3")

	result = ipAddressGlobalKey("", "ignored", "anything")
	c.Assert(result, gc.Equals, "")

	result = ipAddressGlobalKey("ignored", "", "anything")
	c.Assert(result, gc.Equals, "")

	result = ipAddressGlobalKey("", "", "anything")
	c.Assert(result, gc.Equals, "")

	result = ipAddressGlobalKey("", "", "")
	c.Assert(result, gc.Equals, "")
}

func (s *ipAddressesInternalSuite) TestGlobalKeyMethod(c *gc.C) {
	doc := ipAddressDoc{
		MachineID:  "99",
		DeviceName: "br-eth1.250",
		Value:      "fc00:1234::/64",
	}
	address := s.newIPAddressWithDummyState(doc)
	c.Check(address.globalKey(), gc.Equals, "m#99#d#br-eth1.250#ip#fc00:1234::/64")

	address = s.newIPAddressWithDummyState(ipAddressDoc{})
	c.Check(address.globalKey(), gc.Equals, "")
}

func (s *ipAddressesInternalSuite) TestStringIncludesConfigMethodAndValue(c *gc.C) {
	doc := ipAddressDoc{
		ConfigMethod: ManualAddress,
		Value:        "0.1.2.3",
		MachineID:    "42",
		DeviceName:   "eno1",
	}
	result := s.newIPAddressWithDummyState(doc)
	expectedString := `manual address "0.1.2.3" of device "eno1" on machine "42"`

	c.Assert(result.String(), gc.Equals, expectedString)

	result = s.newIPAddressWithDummyState(ipAddressDoc{})
	c.Assert(result.String(), gc.Equals, ` address "" of device "" on machine ""`)
}

func (s *ipAddressesInternalSuite) TestRemainingSimpleGetterMethods(c *gc.C) {
	doc := ipAddressDoc{
		DeviceName:       "eth0",
		MachineID:        "42",
		SubnetCIDR:       "10.20.30.0/24",
		ConfigMethod:     StaticAddress,
		Value:            "10.20.30.40",
		DNSServers:       []string{"ns1.example.com", "ns2.example.org"},
		DNSSearchDomains: []string{"example.com", "example.org"},
		GatewayAddress:   "10.20.30.1",
	}
	result := s.newIPAddressWithDummyState(doc)

	c.Check(result.DeviceName(), gc.Equals, "eth0")
	c.Check(result.MachineID(), gc.Equals, "42")
	c.Check(result.SubnetCIDR(), gc.Equals, "10.20.30.0/24")
	c.Check(result.ConfigMethod(), gc.Equals, StaticAddress)
	c.Check(result.Value(), gc.Equals, "10.20.30.40")
	c.Check(result.DNSServers(), jc.DeepEquals, []string{"ns1.example.com", "ns2.example.org"})
	c.Check(result.DNSSearchDomains(), jc.DeepEquals, []string{"example.com", "example.org"})
	c.Check(result.GatewayAddress(), gc.Equals, "10.20.30.1")
	c.Check(result.NetworkAddress(), jc.DeepEquals, network.NewAddress(result.Value()))
}

func (s *ipAddressesInternalSuite) TestIsValidAddressConfigMethodWithValidValues(c *gc.C) {
	validTypes := []AddressConfigMethod{
		LoopbackAddress,
		StaticAddress,
		DynamicAddress,
		ManualAddress,
	}

	for _, value := range validTypes {
		result := IsValidAddressConfigMethod(string(value))
		c.Check(result, jc.IsTrue)
	}
}

func (s *ipAddressesInternalSuite) TestIsValidAddressConfigMethodWithInvalidValues(c *gc.C) {
	result := IsValidAddressConfigMethod("")
	c.Check(result, jc.IsFalse)

	result = IsValidAddressConfigMethod("anything")
	c.Check(result, jc.IsFalse)

	result = IsValidAddressConfigMethod(" ")
	c.Check(result, jc.IsFalse)
}
