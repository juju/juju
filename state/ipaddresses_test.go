// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

func (s *ipAddressesStateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.otherState = s.NewStateForModelNamed(c, "other-model")
	s.otherStateMachine, err = s.otherState.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// Add the few subnets used by the tests into both models.
	subnetInfos := []state.SubnetInfo{{
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

func (s *ipAddressesStateSuite) TestSubnetMethodReturnsNoErrorWithEmptySubnetID(c *gc.C) {
	_, addresses := s.addNamedDeviceWithAddresses(c, "eth0", "127.0.1.1/8", "::1/128")

	result, err := addresses[0].Subnet()
	c.Assert(result, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)

	result, err = addresses[1].Subnet()
	c.Assert(result, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *ipAddressesStateSuite) addTwoDevicesWithTwoAddressesEach(c *gc.C) []*state.Address {
	_, device1Addresses := s.addNamedDeviceWithAddresses(c, "eth1", "10.20.0.1/16", "10.20.0.2/16")
	_, device2Addresses := s.addNamedDeviceWithAddresses(c, "eth0", "10.20.100.2/16", "fc00::/64")
	s.assertAllAddressesOnMachineMatchCount(c, s.machine, 4)
	return append(device1Addresses, device2Addresses...)
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allAddresses, jc.DeepEquals, addedAddresses)
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

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWithEmptyArgs(c *gc.C) {
	err := s.machine.SetDevicesAddresses() // takes varargs, which includes none.
	c.Assert(err, gc.ErrorMatches, `.*no addresses to set`)
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

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWithInvalidDeviceName(c *gc.C) {
	args := state.LinkLayerDeviceAddress{
		CIDRAddress: "0.1.2.3/24",
		DeviceName:  "bad#name",
	}
	s.assertSetDevicesAddressesFailsValidationForArgs(c, args, `DeviceName "bad#name" not valid`)
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

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWhenCIDRAddressDoesNotMatchKnownSubnet(c *gc.C) {
	s.addNamedDevice(c, "eth0")
	args := state.LinkLayerDeviceAddress{
		CIDRAddress:  "192.168.123.42/16",
		DeviceName:   "eth0",
		ConfigMethod: state.StaticAddress,
	}

	inferredSubnetCIDR := "192.168.0.0/16"
	expectedError := fmt.Sprintf(
		"invalid address %q: subnet %q not found or not alive",
		args.CIDRAddress, inferredSubnetCIDR,
	)
	s.assertSetDevicesAddressesFailsForArgs(c, args, expectedError)
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
		"invalid address %q: subnet %q not found or not alive",
		args.CIDRAddress, subnetCIDR,
	)
	s.assertSetDevicesAddressesFailsForArgs(c, args, expectedError)
}

func (s *ipAddressesStateSuite) TestSetDevicesAddressesFailsWhenModelNotAlive(c *gc.C) {
	s.addNamedDeviceForMachine(c, "eth0", s.otherStateMachine)
	otherModel, err := s.otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = otherModel.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	args := state.LinkLayerDeviceAddress{
		CIDRAddress:  "10.20.30.40/16",
		DeviceName:   "eth0",
		ConfigMethod: state.StaticAddress,
	}
	err = s.otherStateMachine.SetDevicesAddresses(args)
	c.Assert(err, gc.ErrorMatches, `.*: model "other-model" is no longer alive`)
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
		// Test deletes work for DNS settings, also change method, provider id, and gateway.
		DeviceName:       "eth0",
		ConfigMethod:     state.DynamicAddress,
		CIDRAddress:      "0.1.2.3/24",
		ProviderID:       "id-xxxx", // last change wins
		DNSServers:       nil,
		DNSSearchDomains: nil,
		GatewayAddress:   "0.1.2.2",
	}}
	err := s.machine.SetDevicesAddresses(setArgs...)
	c.Assert(err, jc.ErrorIsNil)
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
		ProviderID:   network.Id(providerID),
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
