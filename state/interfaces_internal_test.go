// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
)

// interfacesInternalSuite contains black-box tests for network interfaces
// internals, which do not actually access mongo. The rest of the logic is
// tested in interfacesStateSuite.
type interfacesInternalSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&interfacesInternalSuite{})

func (s *interfacesInternalSuite) TestNewInterfaceCreatesInterface(c *gc.C) {
	result := newInterface(nil, interfaceDoc{})
	c.Assert(result, gc.NotNil)
	c.Assert(result.st, gc.IsNil)
	c.Assert(result.doc, jc.DeepEquals, interfaceDoc{})
}

func (s *interfacesInternalSuite) TestDocIDIncludesModelUUID(c *gc.C) {
	const localDocID = "foo"
	globalDocID := coretesting.ModelTag.Id() + ":" + localDocID

	result := s.newInterfaceWithDummyState(interfaceDoc{DocID: localDocID})
	c.Assert(result.DocID(), gc.Equals, globalDocID)

	result = s.newInterfaceWithDummyState(interfaceDoc{DocID: globalDocID})
	c.Assert(result.DocID(), gc.Equals, globalDocID)
}

func (s *interfacesInternalSuite) newInterfaceWithDummyState(doc interfaceDoc) *Interface {
	// We only need the model UUID set for localID() and docID() to work.
	// The rest is tested in interfacesStateSuite.
	dummyState := &State{modelTag: coretesting.ModelTag}
	return newInterface(dummyState, doc)
}

func (s *interfacesInternalSuite) TestProviderIDIsEmptyWhenNotSet(c *gc.C) {
	result := s.newInterfaceWithDummyState(interfaceDoc{})
	c.Assert(result.ProviderID(), gc.Equals, network.Id(""))
}

func (s *interfacesInternalSuite) TestProviderIDDoesNotIncludeModelUUIDWhenSet(c *gc.C) {
	const localProviderID = "foo"
	globalProviderID := coretesting.ModelTag.Id() + ":" + localProviderID

	result := s.newInterfaceWithDummyState(interfaceDoc{ProviderID: localProviderID})
	c.Assert(result.ProviderID(), gc.Equals, network.Id(localProviderID))

	result = s.newInterfaceWithDummyState(interfaceDoc{ProviderID: globalProviderID})
	c.Assert(result.ProviderID(), gc.Equals, network.Id(localProviderID))
}

func (s *interfacesInternalSuite) TestParentInterfaceReturnsNoErrorWhenParentNameNotSet(c *gc.C) {
	result := s.newInterfaceWithDummyState(interfaceDoc{})
	parent, err := result.ParentInterface()
	c.Check(parent, gc.IsNil)
	c.Check(err, jc.ErrorIsNil)
}

func (s *interfacesInternalSuite) TestInterfaceGlobalKeyHelper(c *gc.C) {
	result := InterfaceGlobalKey("42", "eno1")
	c.Assert(result, gc.Equals, "m#42i#eno1")

	result = InterfaceGlobalKey("", "")
	c.Assert(result, gc.Equals, "")
}

func (s *interfacesInternalSuite) TestGlobalKeyMethod(c *gc.C) {
	doc := interfaceDoc{
		MachineID: "42",
		Name:      "foo",
	}
	nic := s.newInterfaceWithDummyState(doc)
	c.Check(nic.globalKey(), gc.Equals, "m#42i#foo")

	nic = s.newInterfaceWithDummyState(interfaceDoc{})
	c.Check(nic.globalKey(), gc.Equals, "")
}

func (s *interfacesInternalSuite) TestStringIncludesTypeNameAndMachineID(c *gc.C) {
	doc := interfaceDoc{
		MachineID: "42",
		Name:      "foo",
		Type:      BondInterface,
	}
	result := s.newInterfaceWithDummyState(doc)
	expectedString := `bond interface "foo" on machine "42"`

	c.Assert(result.String(), gc.Equals, expectedString)
}

func (s *interfacesInternalSuite) TestRemainingSimpleGetterMethods(c *gc.C) {
	doc := interfaceDoc{
		Name:            "bond0",
		MachineID:       "99",
		Index:           uint(42),
		MTU:             uint(9000),
		Type:            BondInterface,
		HardwareAddress: "aa:bb:cc:dd:ee:f0",
		IsAutoStart:     true,
		IsActive:        true,
		ParentName:      "br-bond0",
		DNSServers:      []string{"ns1.example.com", "127.0.1.1"},
		DNSDomain:       "fake.example.com",
		GatewayAddress:  "0.1.2.3",
	}
	result := s.newInterfaceWithDummyState(doc)

	c.Check(result.Name(), gc.Equals, "bond0")
	c.Check(result.MachineID(), gc.Equals, "99")
	c.Check(result.Index(), gc.Equals, uint(42))
	c.Check(result.MTU(), gc.Equals, uint(9000))
	c.Check(result.Type(), gc.Equals, BondInterface)
	c.Check(result.HardwareAddress(), gc.Equals, "aa:bb:cc:dd:ee:f0")
	c.Check(result.IsAutoStart(), jc.IsTrue)
	c.Check(result.IsActive(), jc.IsTrue)
	c.Check(result.ParentName(), gc.Equals, "br-bond0")
	c.Check(result.DNSServers(), jc.DeepEquals, []string{"ns1.example.com", "127.0.1.1"})
	c.Check(result.DNSDomain(), gc.Equals, "fake.example.com")
	c.Check(result.GatewayAddress(), gc.Equals, "0.1.2.3")
}

func (s *interfacesInternalSuite) TestDNSSettingsAndGatewayAreOptional(c *gc.C) {
	result := s.newInterfaceWithDummyState(interfaceDoc{})

	c.Check(result.DNSServers(), gc.IsNil)
	c.Check(result.DNSDomain(), gc.Equals, "")
	c.Check(result.GatewayAddress(), gc.Equals, "")
}

func (s *interfacesInternalSuite) TestIsValidInterfaceTypeWithValidValue(c *gc.C) {
	validTypes := []InterfaceType{
		UnknownInterface,
		LoopbackInterface,
		EthernetInterface,
		VLANInterface,
		BondInterface,
		BridgeInterface,
	}

	for _, value := range validTypes {
		result := IsValidInterfaceType(string(value))
		c.Check(result, jc.IsTrue)
	}
}

func (s *interfacesInternalSuite) TestIsValidInterfaceTypeWithInvalidValue(c *gc.C) {
	result := IsValidInterfaceType("")
	c.Check(result, jc.IsFalse)

	result = IsValidInterfaceType("anything")
	c.Check(result, jc.IsFalse)

	result = IsValidInterfaceType(" ")
	c.Check(result, jc.IsFalse)
}

func (s *interfacesInternalSuite) TestIsValidInterfaceNameWithUnpatchedGOOS(c *gc.C) {
	result := IsValidInterfaceName("valid")
	c.Check(result, jc.IsTrue)
}

func (s *interfacesInternalSuite) TestIsValidInterfaceNameWithValidNamesWhenGOOSIsinux(c *gc.C) {
	s.PatchValue(&runtimeGOOS, "linux") // isolate the test from the host machine OS.

	for i, name := range validUnixInterfaceNames {
		c.Logf("test #%d: %q -> valid", i, name)
		result := IsValidInterfaceName(name)
		c.Check(result, jc.IsTrue)
	}
}

var validUnixInterfaceNames = []string{
	"eth0", "eno1", "br-eth0.123", "tun:1", "bond0.42",
}

func (s *interfacesInternalSuite) TestIsValidInterfaceNameWithInvalidNamesWhenGOOIsLinux(c *gc.C) {
	s.PatchValue(&runtimeGOOS, "linux") // isolate the test from the host machine OS.

	result := IsValidInterfaceName("")
	c.Check(result, jc.IsFalse)

	const tooLongLength = 16
	result = IsValidInterfaceName(strings.Repeat("x", tooLongLength))
	c.Check(result, jc.IsFalse)

	result = IsValidInterfaceName("with-hash#")
	c.Check(result, jc.IsFalse)

	result = IsValidInterfaceName("has spaces")
	c.Check(result, jc.IsFalse)

	result = IsValidInterfaceName("has\tabs")
	c.Check(result, jc.IsFalse)

	result = IsValidInterfaceName("has\newline")
	c.Check(result, jc.IsFalse)

	result = IsValidInterfaceName("has\r")
	c.Check(result, jc.IsFalse)

	result = IsValidInterfaceName("has\vtab")
	c.Check(result, jc.IsFalse)

	result = IsValidInterfaceName(".")
	c.Check(result, jc.IsFalse)

	result = IsValidInterfaceName("..")
	c.Check(result, jc.IsFalse)
}

func (s *interfacesInternalSuite) TestIsValidInterfaceNameWithValidNamesWhenGOOSNonLinux(c *gc.C) {
	s.PatchValue(&runtimeGOOS, "non-linux") // isolate the test from the host machine OS.
	validInterfaceNames := append(validUnixInterfaceNames,
		// Windows interfaces as friendly name and as underlying UUID.
		"Local Area Connection", "{4a62b748-43d0-4136-92e4-22ce7ee31938}",
	)

	for i, name := range validInterfaceNames {
		c.Logf("test #%d: %q -> valid", i, name)
		result := IsValidInterfaceName(name)
		c.Check(result, jc.IsTrue)
	}
}

func (s *interfacesInternalSuite) TestIsValidInterfaceNameWithInvalidNamesWhenGOOSNonLinux(c *gc.C) {
	s.PatchValue(&runtimeGOOS, "non-linux") // isolate the test from the host machine OS.

	result := IsValidInterfaceName("")
	c.Check(result, jc.IsFalse)

	const wayTooLongLength = 1024
	result = IsValidInterfaceName(strings.Repeat("x", wayTooLongLength))
	c.Check(result, jc.IsFalse)

	result = IsValidInterfaceName("hash# not allowed")
	c.Check(result, jc.IsFalse)
}

func (s *interfacesInternalSuite) TestStringLengthBetweenWhenTooShort(c *gc.C) {
	result := stringLengthBetween("", 1, 2)
	c.Check(result, jc.IsFalse)

	result = stringLengthBetween("", 1, 1)
	c.Check(result, jc.IsFalse)

	result = stringLengthBetween("1", 2, 3)
	c.Check(result, jc.IsFalse)

	result = stringLengthBetween("12", 3, 3)
	c.Check(result, jc.IsFalse)
}

func (s *interfacesInternalSuite) TestStringLengthBetweenWhenTooLong(c *gc.C) {
	result := stringLengthBetween("1", 0, 0)
	c.Check(result, jc.IsFalse)

	result = stringLengthBetween("12", 1, 1)
	c.Check(result, jc.IsFalse)

	result = stringLengthBetween("123", 1, 2)
	c.Check(result, jc.IsFalse)

	result = stringLengthBetween("123", 0, 1)
	c.Check(result, jc.IsFalse)
}

func (s *interfacesInternalSuite) TestStringLengthBetweenWhenWithinLimit(c *gc.C) {
	const (
		minLength = 1
		maxLength = 255
	)
	for i := minLength; i <= maxLength; i++ {
		input := strings.Repeat("x", i)
		result := s.checkStringLengthBetweenSameResultWithSwappedMinMax(c, input, minLength, maxLength)
		c.Check(result, jc.IsTrue)
	}
}

func (s *interfacesInternalSuite) checkStringLengthBetweenSameResultWithSwappedMinMax(c *gc.C, input string, minLength, maxLength uint) bool {
	result := stringLengthBetween(input, minLength, maxLength)
	resultWithSwappedMinMaxLengths := stringLengthBetween(input, maxLength, minLength)
	c.Check(result, gc.Equals, resultWithSwappedMinMaxLengths)
	return result
}
