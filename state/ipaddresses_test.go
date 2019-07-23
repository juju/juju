// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// ipAddressesStateSuite contains white-box tests for IP addresses of link-layer
// devices, which include access to mongo.
type ipAddressesStateSuite struct {
	ConnSuite

	machine *state.Machine

	otherState        *state.State
	otherStateMachine *state.Machine
}

var _ = gc.Suite(&ipAddressesStateSuite{})

type AddressSorter []*state.Address

func (a AddressSorter) Len() int           { return len(a) }
func (a AddressSorter) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a AddressSorter) Less(i, j int) bool { return a[i].DocID() < a[j].DocID() }

var _ sort.Interface = (AddressSorter)(nil)

func (s *ipAddressesStateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.otherState = s.NewStateForModelNamed(c, "other-model")
	s.otherStateMachine, err = s.otherState.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// Add the few subnets used by the tests into both models.
	subnetInfos := []corenetwork.SubnetInfo{{
		CIDR: "0.1.2.0/24",
	}, {
		CIDR: "fc00::/64",
	}, {
		CIDR: "10.20.0.0/16",
	}}
	for _, info := range subnetInfos {
		_, err = s.State.AddSubnet(info)
		c.Check(err, jc.ErrorIsNil)
		_, err = s.otherState.AddSubnet(info)
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *ipAddressesStateSuite) TestMachineMethodReturnsMachine(c *gc.C) {
	_, addresses := s.addNamedDeviceWithAddresses(c, "eth0", "0.1.2.3/24")

	result, err := addresses[0].Machine()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, s.machine)
}

func (s *ipAddressesStateSuite) addNamedDeviceWithAddresses(c *gc.C, name string, addresses ...string) (*state.LinkLayerDevice, []*state.Address) {
	device := s.addNamedDevice(c, name)

	addressesArgs := make([]state.LinkLayerDeviceAddress, len(addresses))
	for i, address := range addresses {
		addressesArgs[i] = state.LinkLayerDeviceAddress{
			DeviceName:   name,
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  address,
		}
	}
	err := s.machine.SetDevicesAddresses(addressesArgs...)
	c.Assert(err, jc.ErrorIsNil)
	deviceAddresses, err := device.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(deviceAddresses, gc.HasLen, len(addresses))
	return device, deviceAddresses
}

func (s *ipAddressesStateSuite) addNamedDevice(c *gc.C, name string) *state.LinkLayerDevice {
	return s.addNamedDeviceForMachine(c, name, s.machine)
}

func (s *ipAddressesStateSuite) addNamedDeviceForMachine(c *gc.C, name string, machine *state.Machine) *state.LinkLayerDevice {
	deviceArgs := state.LinkLayerDeviceArgs{
		Name: name,
		Type: state.EthernetDevice,
	}
	err := machine.SetLinkLayerDevices(deviceArgs)
	c.Assert(err, jc.ErrorIsNil)
	device, err := machine.LinkLayerDevice(name)
	c.Assert(err, jc.ErrorIsNil)
	return device
}

func (s *ipAddressesStateSuite) TestMachineMethodReturnsNotFoundErrorWhenMissing(c *gc.C) {
	_, addresses := s.addNamedDeviceWithAddresses(c, "eth0", "0.1.2.3/24")
	s.ensureMachineDeadAndRemove(c, s.machine)

	result, err := addresses[0].Machine()
	c.Assert(err, gc.ErrorMatches, "machine 0 not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(result, gc.IsNil)
}

func (s *ipAddressesStateSuite) ensureMachineDeadAndRemove(c *gc.C, machine *state.Machine) {
	s.ensureEntityDeadAndRemoved(c, machine)
}

type ensureDeaderRemover interface {
	state.EnsureDeader
	state.Remover
}

func (s *ipAddressesStateSuite) ensureEntityDeadAndRemoved(c *gc.C, entity ensureDeaderRemover) {
	err := entity.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = entity.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ipAddressesStateSuite) TestDeviceMethodReturnsLinkLayerDevice(c *gc.C) {
	addedDevice, addresses := s.addNamedDeviceWithAddresses(c, "eth0", "0.1.2.3/24")

	returnedDevice, err := addresses[0].Device()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(returnedDevice, jc.DeepEquals, addedDevice)
}

func (s *ipAddressesStateSuite) TestDeviceMethodReturnsNotFoundErrorWhenMissing(c *gc.C) {
	device, addresses := s.addNamedDeviceWithAddresses(c, "eth0", "0.1.2.3/24")
	err := device.Remove()
	c.Assert(err, jc.ErrorIsNil)

	result, err := addresses[0].Device()
	c.Assert(result, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `device "eth0" on machine "0" not found`)
}

func (s *ipAddressesStateSuite) TestSubnetMethodReturnsSubnet(c *gc.C) {
	_, addresses := s.addNamedDeviceWithAddresses(c, "eth0", "10.20.30.41/16")

	result, err := addresses[0].Subnet()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.CIDR(), gc.Equals, "10.20.0.0/16")
}

func (s *ipAddressesStateSuite) TestSubnetMethodReturnsNotFoundErrorWhenMissing(c *gc.C) {
	_, addresses := s.addNamedDeviceWithAddresses(c, "eth0", "10.20.30.41/16")
	subnet, err := s.State.Subnet("10.20.0.0/16")
	c.Assert(err, jc.ErrorIsNil)
	s.ensureEntityDeadAndRemoved(c, subnet)

	result, err := addresses[0].Subnet()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `subnet "10.20.0.0/16" not found`)
	c.Assert(result, gc.IsNil)
}

func (s *ipAddressesStateSuite) TestSubnetMethodReturnsNotFoundErrorWithUnknownOrLocalSubnet(c *gc.C) {
	cidrs := []string{"127.0.0.0/8", "::1/128", "8.8.0.0/16"}
	missingCIDRs := set.NewStrings(cidrs...)
	_, addresses := s.addNamedDeviceWithAddresses(c, "eth0", cidrs...)

	for _, address := range addresses {
		result, err := address.Subnet()
		c.Check(result, gc.IsNil)
		c.Check(err, jc.Satisfies, errors.IsNotFound)
		expectedError := fmt.Sprintf("subnet %q not found", address.SubnetCIDR())
		c.Check(err, gc.ErrorMatches, expectedError)
		missingCIDRs.Remove(address.SubnetCIDR())
	}
	c.Check(missingCIDRs.SortedValues(), gc.DeepEquals, []string{})
}

func (s *ipAddressesStateSuite) TestRemoveSuccess(c *gc.C) {
	_, existingAddresses := s.addNamedDeviceWithAddresses(c, "eth0", "0.1.2.3/24")

	s.removeAddressAndAssertSuccess(c, existingAddresses[0])
	s.assertNoAddressesOnMachine(c, s.machine)
}

func (s *ipAddressesStateSuite) removeAddressAndAssertSuccess(c *gc.C, givenAddress *state.Address) {
	err := givenAddress.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ipAddressesStateSuite) assertNoAddressesOnMachine(c *gc.C, machine *state.Machine) {
	s.assertAllAddressesOnMachineMatchCount(c, machine, 0)
}

func (s *ipAddressesStateSuite) assertAllAddressesOnMachineMatchCount(c *gc.C, machine *state.Machine, expectedCount int) {
	results, err := machine.AllAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, expectedCount, gc.Commentf("expected %d, got %d: %+v", expectedCount, len(results), results))
}

func (s *ipAddressesStateSuite) TestRemoveTwiceStillSucceeds(c *gc.C) {
	_, existingAddresses := s.addNamedDeviceWithAddresses(c, "eth0", "0.1.2.3/24")

	s.removeAddressAndAssertSuccess(c, existingAddresses[0])
	s.removeAddressAndAssertSuccess(c, existingAddresses[0])
	s.assertNoAddressesOnMachine(c, s.machine)
}

func (s *ipAddressesStateSuite) TestLinkLayerDeviceAddressesReturnsAllDeviceAddresses(c *gc.C) {
	device, addedAddresses := s.addNamedDeviceWithAddresses(c, "eth0", "0.1.2.3/24", "10.20.30.40/16", "fc00::/64")
	s.assertAllAddressesOnMachineMatchCount(c, s.machine, 3)

	resultAddresses, err := device.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultAddresses, jc.DeepEquals, addedAddresses)
}

func (s *ipAddressesStateSuite) TestLinkLayerDeviceRemoveAddressesSuccess(c *gc.C) {
	device, _ := s.addNamedDeviceWithAddresses(c, "eth0", "10.20.30.40/16", "fc00::/64")
	s.assertAllAddressesOnMachineMatchCount(c, s.machine, 2)

	s.removeDeviceAddressesAndAssertNoneRemainOnMacine(c, device)
}

func (s *ipAddressesStateSuite) removeDeviceAddressesAndAssertNoneRemainOnMacine(c *gc.C, device *state.LinkLayerDevice) {
	err := device.RemoveAddresses()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoAddressesOnMachine(c, s.machine)
}

func (s *ipAddressesStateSuite) TestLinkLayerDeviceRemoveAddressesTwiceStillSucceeds(c *gc.C) {
	device, _ := s.addNamedDeviceWithAddresses(c, "eth0", "10.20.30.40/16", "fc00::/64")
	s.assertAllAddressesOnMachineMatchCount(c, s.machine, 2)

	s.removeDeviceAddressesAndAssertNoneRemainOnMacine(c, device)
	s.removeDeviceAddressesAndAssertNoneRemainOnMacine(c, device)
}

func (s *ipAddressesStateSuite) TestMachineRemoveAllAddressesSuccess(c *gc.C) {
	s.addTwoDevicesWithTwoAddressesEach(c)
	s.removeAllAddressesOnMachineAndAssertNoneRemain(c)
}

func (s *ipAddressesStateSuite) TestMachineRemoveAllAddressesRemovesProviderIDReferences(c *gc.C) {
	s.addNamedDevice(c, "foo")
	addrArgs := state.LinkLayerDeviceAddress{
		DeviceName:   "foo",
		ConfigMethod: state.StaticAddress,
		CIDRAddress:  "0.1.2.3/24",
		ProviderID:   "bar",
	}
	err := s.machine.SetDevicesAddresses(addrArgs)
	c.Assert(err, jc.ErrorIsNil)
	s.removeAllAddressesOnMachineAndAssertNoneRemain(c)

	// Re-adding the same address to a new device should now succeed.
	err = s.machine.SetDevicesAddresses(addrArgs)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ipAddressesStateSuite) addTwoDevicesWithTwoAddressesEach(c *gc.C) []*state.Address {
	_, device1Addresses := s.addNamedDeviceWithAddresses(c, "eth1", "10.20.0.1/16", "10.20.0.2/16")
	_, device2Addresses := s.addNamedDeviceWithAddresses(c, "eth0", "10.20.100.2/16", "fc00::/64")
	s.assertAllAddressesOnMachineMatchCount(c, s.machine, 4)
	addresses := append(device1Addresses, device2Addresses...)
	sort.Sort(AddressSorter(addresses))
	return addresses
}

func (s *ipAddressesStateSuite) removeAllAddressesOnMachineAndAssertNoneRemain(c *gc.C) {
	err := s.machine.RemoveAllAddresses()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoAddressesOnMachine(c, s.machine)
}

func (s *ipAddressesStateSuite) TestMachineRemoveAllAddressesTwiceStillSucceeds(c *gc.C) {
	s.addTwoDevicesWithTwoAddressesEach(c)
	s.removeAllAddressesOnMachineAndAssertNoneRemain(c)
	s.removeAllAddressesOnMachineAndAssertNoneRemain(c)
}

func (s *ipAddressesStateSuite) TestMachineAllAddressesSuccess(c *gc.C) {
	addedAddresses := s.addTwoDevicesWithTwoAddressesEach(c)

	allAddresses, err := s.machine.AllAddresses()
	sort.Sort(AddressSorter(allAddresses))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allAddresses, jc.DeepEquals, addedAddresses)
}

func (s *ipAddressesStateSuite) TestMachineAllNetworkAddresses(c *gc.C) {
	addedAddresses := s.addTwoDevicesWithTwoAddressesEach(c)
	expected := make([]network.Address, len(addedAddresses))
	for i := range addedAddresses {
		expected[i] = addedAddresses[i].NetworkAddress()
	}
	sort.Slice(expected, func(i, j int) bool {
		return expected[i].Value < expected[j].Value
	})

	networkAddresses, err := s.machine.AllNetworkAddresses()
	c.Assert(err, jc.ErrorIsNil)
	sort.Slice(networkAddresses, func(i, j int) bool {
		return networkAddresses[i].Value < networkAddresses[j].Value
	})
	c.Assert(networkAddresses, jc.DeepEquals, expected)
}

func (s *ipAddressesStateSuite) TestLinkLayerDeviceRemoveAlsoRemovesDeviceAddresses(c *gc.C) {
	device, _ := s.addNamedDeviceWithAddresses(c, "eth0", "10.20.30.40/16", "fc00::/64")
	s.assertAllAddressesOnMachineMatchCount(c, s.machine, 2)

	err := device.Remove()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoAddressesOnMachine(c, s.machine)
}

func (s *ipAddressesStateSuite) TestMachineRemoveAlsoRemoveAllAddresses(c *gc.C) {
	s.addTwoDevicesWithTwoAddressesEach(c)
	s.ensureMachineDeadAndRemove(c, s.machine)

	s.assertNoAddressesOnMachine(c, s.machine)
}

func (s *ipAddressesStateSuite) TestAllSpacesHandlesUnknownSubnets(c *gc.C) {
	// This is not one of the registered subnets
	s.addNamedDeviceWithAddresses(c, "eth0", "172.12.0.10/24")
	spaces, err := s.machine.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces.SortedValues(), gc.DeepEquals, []string{})
}

func resetSubnet(c *gc.C, st *state.State, subnetInfo corenetwork.SubnetInfo) {
	// We currently don't allow updating a subnet's information, so remove it
	// and add it with the new value.
	// XXX(jam): We should add mutation operations instead of this ugly hack
	subnet, err := st.Subnet(subnetInfo.CIDR)
	c.Assert(err, jc.ErrorIsNil)
	err = subnet.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = subnet.Remove()
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.AddSubnet(subnetInfo)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ipAddressesStateSuite) TestAllSpacesOneSpace(c *gc.C) {
	s.addTwoDevicesWithTwoAddressesEach(c)
	_, err := s.State.AddSpace("default", "default", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	resetSubnet(c, s.State, corenetwork.SubnetInfo{
		CIDR:      "10.20.0.0/16",
		SpaceName: "default",
	})
	c.Assert(err, jc.ErrorIsNil)
	resetSubnet(c, s.State, corenetwork.SubnetInfo{
		CIDR:      "fc00::/64",
		SpaceName: "default",
	})
	c.Assert(err, jc.ErrorIsNil)
	spaces, err := s.machine.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces.SortedValues(), gc.DeepEquals, []string{"default"})
}

func (s *ipAddressesStateSuite) TestAllSpacesMultiSpace(c *gc.C) {
	s.addTwoDevicesWithTwoAddressesEach(c)
	_, err := s.State.AddSpace("default", "default", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	resetSubnet(c, s.State, corenetwork.SubnetInfo{
		CIDR:      "10.20.0.0/16",
		SpaceName: "default",
	})
	c.Assert(err, jc.ErrorIsNil)
	resetSubnet(c, s.State, corenetwork.SubnetInfo{
		CIDR:      "fc00::/64",
		SpaceName: "dmz-ipv6",
	})
	c.Assert(err, jc.ErrorIsNil)
	spaces, err := s.machine.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces.SortedValues(), gc.DeepEquals, []string{"default", "dmz-ipv6"})
}

func (s *ipAddressesStateSuite) TestAllSpacesIgnoresEmptySpaceNames(c *gc.C) {
	s.addTwoDevicesWithTwoAddressesEach(c)
	spaces, err := s.machine.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(spaces.SortedValues(), gc.DeepEquals, []string{})
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesDoesNothingWithEmptyArgs(c *gc.C) {
	err := s.machine.SetDevicesAddresses() // takes varargs, which includes none.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWithEmptyCIDRAddress(c *gc.C) {
	args := state.LinkLayerDeviceAddress{}
	s.assertSetDevicesAddressesFailsValidationForArgs(c, args, "empty CIDRAddress not valid")
}

func (s *ipAddressesStateSuite) assertSetDevicesAddressesFailsValidationForArgs(c *gc.C, args state.LinkLayerDeviceAddress, errorCauseMatches string) {
	invalidAddressPrefix := fmt.Sprintf("invalid address %q: ", args.CIDRAddress)
	err := s.assertSetDevicesAddressesFailsForArgs(c, args, invalidAddressPrefix+errorCauseMatches)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *ipAddressesStateSuite) assertSetDevicesAddressesFailsForArgs(c *gc.C, args state.LinkLayerDeviceAddress, errorCauseMatches string) error {
	err := s.machine.SetDevicesAddresses(args)
	expectedError := fmt.Sprintf("cannot set link-layer device addresses of machine %q: %s", s.machine.Id(), errorCauseMatches)
	c.Assert(err, gc.ErrorMatches, expectedError)
	return err
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWithInvalidCIDRAddress(c *gc.C) {
	args := state.LinkLayerDeviceAddress{
		CIDRAddress: "bad CIDR",
	}
	s.assertSetDevicesAddressesFailsValidationForArgs(c, args, "CIDRAddress: invalid CIDR address: bad CIDR")
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWithCIDRAddressWithoutMask(c *gc.C) {
	args := state.LinkLayerDeviceAddress{
		CIDRAddress: "10.10.10.10",
	}
	s.assertSetDevicesAddressesFailsValidationForArgs(c, args, "CIDRAddress: invalid CIDR address: 10.10.10.10")
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWithEmptyDeviceName(c *gc.C) {
	args := state.LinkLayerDeviceAddress{
		CIDRAddress: "0.1.2.3/24",
	}
	s.assertSetDevicesAddressesFailsValidationForArgs(c, args, "empty DeviceName not valid")
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWithUnknownDeviceName(c *gc.C) {
	args := state.LinkLayerDeviceAddress{
		CIDRAddress:  "0.1.2.3/24",
		ConfigMethod: state.StaticAddress,
		DeviceName:   "missing",
	}
	expectedError := `invalid address "0.1.2.3/24": DeviceName "missing" on machine "0" not found`
	err := s.assertSetDevicesAddressesFailsForArgs(c, args, expectedError)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWithInvalidConfigMethod(c *gc.C) {
	s.addNamedDevice(c, "eth0")
	args := state.LinkLayerDeviceAddress{
		CIDRAddress:  "0.1.2.3/24",
		DeviceName:   "eth0",
		ConfigMethod: "something else",
	}
	s.assertSetDevicesAddressesFailsValidationForArgs(c, args, `ConfigMethod "something else" not valid`)
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWithInvalidGatewayAddress(c *gc.C) {
	s.addNamedDevice(c, "eth0")
	args := state.LinkLayerDeviceAddress{
		CIDRAddress:    "0.1.2.3/24",
		DeviceName:     "eth0",
		ConfigMethod:   state.StaticAddress,
		GatewayAddress: "boo hoo",
	}
	s.assertSetDevicesAddressesFailsValidationForArgs(c, args, `GatewayAddress "boo hoo" not valid`)
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesOKWhenCIDRAddressDoesNotMatchKnownSubnet(c *gc.C) {
	device := s.addNamedDevice(c, "eth0")
	args := state.LinkLayerDeviceAddress{
		CIDRAddress:  "192.168.123.42/16",
		DeviceName:   "eth0",
		ConfigMethod: state.StaticAddress,
	}
	err := s.machine.SetDevicesAddresses(args)
	c.Assert(err, jc.ErrorIsNil)

	assertDeviceHasOneAddressWithSubnetCIDREquals := func(subnetCIDR string) {
		addresses, err := device.Addresses()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(addresses, gc.HasLen, 1)
		c.Assert(addresses[0].SubnetCIDR(), gc.Equals, subnetCIDR)
	}
	assertDeviceHasOneAddressWithSubnetCIDREquals("192.168.0.0/16")

	// Add the subnet so it's known and retry setting the same address to verify
	// SubnetID gets updated.
	_, err = s.State.AddSubnet(corenetwork.SubnetInfo{CIDR: "192.168.0.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetDevicesAddresses(args)
	c.Assert(err, jc.ErrorIsNil)

	assertDeviceHasOneAddressWithSubnetCIDREquals("192.168.0.0/16")
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWhenCIDRAddressMatchesDeadSubnet(c *gc.C) {
	s.addNamedDevice(c, "eth0")
	subnetCIDR := "10.20.0.0/16"
	subnet, err := s.State.Subnet(subnetCIDR)
	c.Assert(err, jc.ErrorIsNil)
	err = subnet.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	args := state.LinkLayerDeviceAddress{
		CIDRAddress:  "10.20.30.40/16",
		DeviceName:   "eth0",
		ConfigMethod: state.StaticAddress,
	}
	expectedError := fmt.Sprintf(
		"invalid address %q: subnet %q is not alive",
		args.CIDRAddress, subnetCIDR,
	)
	s.assertSetDevicesAddressesFailsForArgs(c, args, expectedError)
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWhenMachineNotAliveOrGone(c *gc.C) {
	s.addNamedDeviceForMachine(c, "eth0", s.otherStateMachine)
	err := s.otherStateMachine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	args := state.LinkLayerDeviceAddress{
		CIDRAddress:  "10.20.30.40/16",
		DeviceName:   "eth0",
		ConfigMethod: state.StaticAddress,
	}
	err = s.otherStateMachine.SetDevicesAddresses(args)
	c.Assert(err, gc.ErrorMatches, `.*: machine not found or not alive`)

	err = s.otherStateMachine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	// Check it fails with a different error, as eth0 was removed along with
	// otherStateMachine above.
	err = s.otherStateMachine.SetDevicesAddresses(args)
	c.Assert(err, gc.ErrorMatches, `.*: DeviceName "eth0" on machine "0" not found`)
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesUpdatesExistingDocs(c *gc.C) {
	device, initialAddresses := s.addNamedDeviceWithAddresses(c, "eth0", "0.1.2.3/24", "10.20.30.42/16")

	setArgs := []state.LinkLayerDeviceAddress{{
		// All fields that can be set are included below.
		DeviceName:       "eth0",
		ConfigMethod:     state.ManualAddress,
		CIDRAddress:      "0.1.2.3/24",
		ProviderID:       "id-0123",
		DNSServers:       []string{"ns1.example.com", "ns2.example.org"},
		DNSSearchDomains: []string{"example.com", "example.org"},
		GatewayAddress:   "0.1.2.1",
	}, {
		// No changed fields, just the required values are set: CIDRAddress +
		// DeviceName (and s.machine.Id) are used to construct the DocID.
		DeviceName:   "eth0",
		ConfigMethod: state.StaticAddress,
		CIDRAddress:  "10.20.30.42/16",
	}}
	err := s.machine.SetDevicesAddresses(setArgs...)
	c.Assert(err, jc.ErrorIsNil)
	updatedAddresses, err := device.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updatedAddresses, gc.HasLen, len(initialAddresses))
	if updatedAddresses[0].Value() != "0.1.2.3" {
		// Swap the results if they arrive in different order.
		updatedAddresses[1], updatedAddresses[0] = updatedAddresses[0], updatedAddresses[1]
	}

	for i, address := range updatedAddresses {
		s.checkAddressMatchesArgs(c, address, setArgs[i])
	}
}

func (s *ipAddressesStateSuite) TestRemoveAddressRemovesProviderID(c *gc.C) {
	device := s.addNamedDevice(c, "eth0")
	addrArgs := state.LinkLayerDeviceAddress{
		DeviceName:   "eth0",
		ConfigMethod: state.ManualAddress,
		CIDRAddress:  "0.1.2.3/24",
		ProviderID:   "id-0123",
	}
	err := s.machine.SetDevicesAddresses(addrArgs)
	c.Assert(err, jc.ErrorIsNil)
	addresses, err := device.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.HasLen, 1)
	addr := addresses[0]
	err = addr.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetDevicesAddresses(addrArgs)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ipAddressesStateSuite) TestUpdateAddressFailsToChangeProviderID(c *gc.C) {
	s.addNamedDevice(c, "eth0")
	addrArgs := state.LinkLayerDeviceAddress{
		DeviceName:   "eth0",
		ConfigMethod: state.ManualAddress,
		CIDRAddress:  "0.1.2.3/24",
		ProviderID:   "id-0123",
	}
	err := s.machine.SetDevicesAddresses(addrArgs)
	c.Assert(err, jc.ErrorIsNil)
	addrArgs.ProviderID = "id-0124"
	err = s.machine.SetDevicesAddresses(addrArgs)
	c.Assert(err, gc.ErrorMatches, `.*cannot change ProviderID of link address "0.1.2.3"`)
}

func (s *ipAddressesStateSuite) TestUpdateAddressPreventsDuplicateProviderID(c *gc.C) {
	s.addNamedDevice(c, "eth0")
	addrArgs := state.LinkLayerDeviceAddress{
		DeviceName:   "eth0",
		ConfigMethod: state.ManualAddress,
		CIDRAddress:  "0.1.2.3/24",
	}
	err := s.machine.SetDevicesAddresses(addrArgs)
	c.Assert(err, jc.ErrorIsNil)

	// Set the provider id through an update.
	addrArgs.ProviderID = "id-0123"
	err = s.machine.SetDevicesAddresses(addrArgs)
	c.Assert(err, jc.ErrorIsNil)

	// Adding a new address with the same provider id should now fail.
	addrArgs.CIDRAddress = "0.1.2.4/24"
	err = s.machine.SetDevicesAddresses(addrArgs)
	c.Assert(err, gc.ErrorMatches, `.*invalid address "0.1.2.4/24": ProviderID\(s\) not unique: id-0123`)
}

func (s *ipAddressesStateSuite) checkAddressMatchesArgs(c *gc.C, address *state.Address, args state.LinkLayerDeviceAddress) {
	c.Check(address.DeviceName(), gc.Equals, args.DeviceName)
	c.Check(address.MachineID(), gc.Equals, s.machine.Id())
	c.Check(args.CIDRAddress, jc.HasPrefix, address.Value())
	c.Check(address.ConfigMethod(), gc.Equals, args.ConfigMethod)
	c.Check(address.ProviderID(), gc.Equals, args.ProviderID)
	c.Check(address.DNSServers(), jc.DeepEquals, args.DNSServers)
	c.Check(address.DNSSearchDomains(), jc.DeepEquals, args.DNSSearchDomains)
	c.Check(address.GatewayAddress(), gc.Equals, args.GatewayAddress)
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesWithMultipleUpdatesOfSameDocLastUpdateWins(c *gc.C) {
	device, initialAddresses := s.addNamedDeviceWithAddresses(c, "eth0", "0.1.2.3/24")

	setArgs := []state.LinkLayerDeviceAddress{{
		// No changes - same args as used by addNamedDeviceWithAddresses, so
		// this is testing a no-op case.
		DeviceName:   "eth0",
		ConfigMethod: state.StaticAddress,
		CIDRAddress:  "0.1.2.3/24",
	}, {
		// Change all fields that can change.
		DeviceName:       "eth0",
		ConfigMethod:     state.ManualAddress,
		CIDRAddress:      "0.1.2.3/24",
		ProviderID:       "id-0123",
		DNSServers:       []string{"ns1.example.com", "ns2.example.org"},
		DNSSearchDomains: []string{"example.com", "example.org"},
		GatewayAddress:   "0.1.2.1",
	}, {
		// Test deletes work for DNS settings, also change method, and gateway.
		DeviceName:       "eth0",
		ConfigMethod:     state.DynamicAddress,
		CIDRAddress:      "0.1.2.3/24",
		ProviderID:       "id-0123", // not allowed to change ProviderID once set
		DNSServers:       nil,
		DNSSearchDomains: nil,
		GatewayAddress:   "0.1.2.2",
	}}
	for _, arg := range setArgs {
		err := s.machine.SetDevicesAddresses(arg)
		c.Assert(err, jc.ErrorIsNil)
	}
	updatedAddresses, err := device.Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updatedAddresses, gc.HasLen, len(initialAddresses))

	var lastArgsIndex = len(setArgs) - 1
	s.checkAddressMatchesArgs(c, updatedAddresses[0], state.LinkLayerDeviceAddress{
		DeviceName:       setArgs[lastArgsIndex].DeviceName,
		ConfigMethod:     setArgs[lastArgsIndex].ConfigMethod,
		CIDRAddress:      setArgs[lastArgsIndex].CIDRAddress,
		ProviderID:       setArgs[lastArgsIndex].ProviderID,
		DNSServers:       setArgs[lastArgsIndex].DNSServers,
		DNSSearchDomains: setArgs[lastArgsIndex].DNSSearchDomains,
		GatewayAddress:   setArgs[lastArgsIndex].GatewayAddress,
	})
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesWithDuplicateProviderIDFailsInSameModel(c *gc.C) {
	_, firstAddressArgs := s.addDeviceWithAddressAndProviderIDForMachine(c, "42", s.machine)
	secondAddressArgs := firstAddressArgs
	secondAddressArgs.CIDRAddress = "10.20.30.40/16"

	err := s.machine.SetDevicesAddresses(secondAddressArgs)
	c.Assert(err, jc.Satisfies, state.IsProviderIDNotUniqueError)
	c.Assert(err, gc.ErrorMatches, `.*invalid address "10.20.30.40/16": ProviderID\(s\) not unique: 42`)
}

func (s *ipAddressesStateSuite) addDeviceWithAddressAndProviderIDForMachine(c *gc.C, providerID string, machine *state.Machine) (
	*state.LinkLayerDevice,
	state.LinkLayerDeviceAddress,
) {
	device := s.addNamedDeviceForMachine(c, "eth0", machine)
	addressArgs := state.LinkLayerDeviceAddress{
		DeviceName:   "eth0",
		ConfigMethod: state.StaticAddress,
		CIDRAddress:  "0.1.2.3/24",
		ProviderID:   corenetwork.Id(providerID),
	}
	err := machine.SetDevicesAddresses(addressArgs)
	c.Assert(err, jc.ErrorIsNil)
	return device, addressArgs
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesWithDuplicateProviderIDSucceedsInDifferentModel(c *gc.C) {
	_, firstAddressArgs := s.addDeviceWithAddressAndProviderIDForMachine(c, "42", s.otherStateMachine)
	secondAddressArgs := firstAddressArgs
	secondAddressArgs.CIDRAddress = "10.20.30.40/16"

	s.addNamedDevice(c, firstAddressArgs.DeviceName) // for s.machine
	err := s.machine.SetDevicesAddresses(secondAddressArgs)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ipAddressesStateSuite) TestMachineSetDevicesAddressesIdempotentlyOnce(c *gc.C) {
	s.testMachineSetDevicesAddressesIdempotently(c)
}

func (s *ipAddressesStateSuite) TestMachineSetDevicesAddressesIdempotentlyTwice(c *gc.C) {
	s.testMachineSetDevicesAddressesIdempotently(c)
	s.testMachineSetDevicesAddressesIdempotently(c)
}

func (s *ipAddressesStateSuite) testMachineSetDevicesAddressesIdempotently(c *gc.C) {
	err := s.machine.SetParentLinkLayerDevicesBeforeTheirChildren(nestedDevicesArgs)
	c.Assert(err, jc.ErrorIsNil)

	args := []state.LinkLayerDeviceAddress{{
		DeviceName:   "lo",
		CIDRAddress:  "127.0.0.1/8",
		ConfigMethod: state.LoopbackAddress,
	}, {
		DeviceName:   "br-bond0",
		CIDRAddress:  "10.20.0.100/16",
		ConfigMethod: state.StaticAddress,
		ProviderID:   "200",
	}, {
		DeviceName:   "br-bond0.12",
		CIDRAddress:  "0.1.2.112/24",
		ConfigMethod: state.StaticAddress,
		ProviderID:   "201",
	}, {
		DeviceName:   "br-bond0.34",
		CIDRAddress:  "0.1.2.134/24",
		ConfigMethod: state.StaticAddress,
		ProviderID:   "202",
	}}
	err = s.machine.SetDevicesAddressesIdempotently(args)
	c.Assert(err, jc.ErrorIsNil)
	allAddresses, err := s.machine.AllAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allAddresses, gc.HasLen, len(args))
	for _, address := range allAddresses {
		if address.ConfigMethod() != state.LoopbackAddress && address.ConfigMethod() != state.ManualAddress {
			c.Check(address.ProviderID(), gc.Not(gc.Equals), corenetwork.Id(""))
		}
	}
}
