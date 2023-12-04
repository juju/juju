// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/network/containerizer"
	"github.com/juju/juju/state"
)

// linkLayerDevicesStateSuite contains black-box tests for link-layer network
// devices, which include access to mongo.
type linkLayerDevicesStateSuite struct {
	ConnSuite

	machine           *state.Machine
	containerMachine  *state.Machine
	otherState        *state.State
	otherStateMachine *state.Machine

	spaces map[string]corenetwork.SpaceInfo

	bridgePolicy *containerizer.BridgePolicy
}

var _ = gc.Suite(&linkLayerDevicesStateSuite{})

func (s *linkLayerDevicesStateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	var err error
	s.machine, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.otherState = s.NewStateForModelNamed(c, "other-model")
	s.otherStateMachine, err = s.otherState.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.spaces = map[string]corenetwork.SpaceInfo{
		corenetwork.AlphaSpaceName: {ID: "0", Name: corenetwork.AlphaSpaceName},
	}

	s.bridgePolicy = &containerizer.BridgePolicy{}
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesNoArgs(c *gc.C) {
	err := s.machine.SetLinkLayerDevices() // takes varargs, which includes none.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) assertSetLinkLayerDevicesFailsValidationForArgs(c *gc.C, args state.LinkLayerDeviceArgs, errorCauseMatches string) error {
	expectedError := fmt.Sprintf("invalid device %q: %s", args.Name, errorCauseMatches)
	return s.assertSetLinkLayerDevicesFailsForArgs(c, args, expectedError)
}

func (s *linkLayerDevicesStateSuite) assertSetLinkLayerDevicesFailsForArgs(c *gc.C, args state.LinkLayerDeviceArgs, errorCauseMatches string) error {
	err := s.machine.SetLinkLayerDevices(args)
	expectedError := fmt.Sprintf("cannot set link-layer devices to machine %q: %s", s.machine.Id(), errorCauseMatches)
	c.Assert(err, gc.ErrorMatches, expectedError)
	return err
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesWhenMachineNotAliveOrGone(c *gc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	args := state.LinkLayerDeviceArgs{
		Name: "eth0",
		Type: corenetwork.EthernetDevice,
	}
	_ = s.assertSetLinkLayerDevicesFailsForArgs(c, args, `machine "0" not alive`)

	err = s.machine.Remove(nil)
	c.Assert(err, jc.ErrorIsNil)

	_ = s.assertSetLinkLayerDevicesFailsForArgs(c, args, `machine "0" not alive`)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesNoParentSuccess(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:        "eth0.42",
		MTU:         9000,
		ProviderID:  "eni-42",
		Type:        corenetwork.VLAN8021QDevice,
		MACAddress:  "aa:bb:cc:dd:ee:f0",
		IsAutoStart: true,
		IsUp:        true,
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)
}

func (s *linkLayerDevicesStateSuite) assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(
	c *gc.C,
	args state.LinkLayerDeviceArgs,
) *state.LinkLayerDevice {
	return s.assertMachineSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, s.machine, args, s.State.ModelUUID())
}

func (s *linkLayerDevicesStateSuite) assertMachineSetLinkLayerDevicesSucceedsAndResultMatchesArgs(
	c *gc.C,
	machine *state.Machine,
	args state.LinkLayerDeviceArgs,
	modelUUID string,
) *state.LinkLayerDevice {
	err := machine.SetLinkLayerDevices(args)
	c.Assert(err, jc.ErrorIsNil)
	result, err := machine.LinkLayerDevice(args.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)

	s.checkSetDeviceMatchesArgs(c, result, args)
	s.checkSetDeviceMatchesMachineIDAndModelUUID(c, result, s.machine.Id(), modelUUID)
	return result
}

func (s *linkLayerDevicesStateSuite) checkSetDeviceMatchesArgs(c *gc.C, setDevice *state.LinkLayerDevice, args state.LinkLayerDeviceArgs) {
	c.Check(setDevice.Name(), gc.Equals, args.Name)
	c.Check(setDevice.MTU(), gc.Equals, args.MTU)
	c.Check(setDevice.ProviderID(), gc.Equals, args.ProviderID)
	c.Check(setDevice.Type(), gc.Equals, args.Type)
	c.Check(setDevice.MACAddress(), gc.Equals, args.MACAddress)
	c.Check(setDevice.IsAutoStart(), gc.Equals, args.IsAutoStart)
	c.Check(setDevice.IsUp(), gc.Equals, args.IsUp)
	c.Check(setDevice.ParentName(), gc.Equals, args.ParentName)
}

func (s *linkLayerDevicesStateSuite) checkSetDeviceMatchesMachineIDAndModelUUID(c *gc.C, setDevice *state.LinkLayerDevice, machineID, modelUUID string) {
	globalKey := fmt.Sprintf("m#%s#d#%s", machineID, setDevice.Name())
	c.Check(setDevice.DocID(), gc.Equals, modelUUID+":"+globalKey)
	c.Check(setDevice.MachineID(), gc.Equals, machineID)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesNoProviderIDSuccess(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name: "eno0",
		Type: corenetwork.EthernetDevice,
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesWithDuplicateProviderIDFailsInSameModel(c *gc.C) {
	args1 := state.LinkLayerDeviceArgs{
		Name:       "eth0.42",
		Type:       corenetwork.EthernetDevice,
		ProviderID: "42",
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args1)

	args2 := args1
	args2.Name = "br-eth0"
	err := s.assertSetLinkLayerDevicesFailsValidationForArgs(c, args2, `provider IDs not unique: 42`)
	c.Assert(err, jc.Satisfies, state.IsProviderIDNotUniqueError)
}

func (s *linkLayerDevicesStateSuite) TestRemoveAllLinkLayerDevicesClearsProviderIDs(c *gc.C) {
	args1 := state.LinkLayerDeviceArgs{
		Name:       "eth0.42",
		Type:       corenetwork.EthernetDevice,
		ProviderID: "42",
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args1)

	c.Assert(s.machine.RemoveAllLinkLayerDevices(), jc.ErrorIsNil)

	// We can add the same device, with the same provider ID without error
	// because the global provider ID references were removed with the devices.
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args1)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesWithDuplicateNameAndProviderIDSucceedsInDifferentModels(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "eth0.42",
		Type:       corenetwork.EthernetDevice,
		ProviderID: "42",
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)

	s.assertMachineSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, s.otherStateMachine, args, s.otherState.ModelUUID())
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesUpdatesProviderIDWhenNotSetOriginally(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: corenetwork.EthernetDevice,
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)

	args.ProviderID = "42"
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesUpdateWithDuplicateProviderIDFails(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "foo",
		Type:       corenetwork.EthernetDevice,
		ProviderID: "42",
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)
	args.Name = "bar"
	args.ProviderID = ""
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)

	args.ProviderID = "42"
	err := s.assertSetLinkLayerDevicesFailsValidationForArgs(c, args, `provider IDs not unique: 42`)
	c.Assert(err, jc.Satisfies, state.IsProviderIDNotUniqueError)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesDoesNotClearProviderIDOnceSet(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "foo",
		Type:       corenetwork.EthernetDevice,
		ProviderID: "42",
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)

	args.ProviderID = ""
	err := s.machine.SetLinkLayerDevices(args)
	c.Assert(err, jc.ErrorIsNil)
	device, err := s.machine.LinkLayerDevice(args.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(device.ProviderID(), gc.Equals, corenetwork.Id("42"))
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesMultipleArgsWithSameNameFails(c *gc.C) {
	foo1 := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: corenetwork.BridgeDevice,
	}
	foo2 := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: corenetwork.EthernetDevice,
	}
	err := s.machine.SetLinkLayerDevices(foo1, foo2)
	c.Assert(err, gc.ErrorMatches, `.*invalid device "foo": Name specified more than once`)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *linkLayerDevicesStateSuite) setMultipleDevicesSucceedsAndCheckAllAdded(c *gc.C, allArgs []state.LinkLayerDeviceArgs) []*state.LinkLayerDevice {
	err := s.machine.SetLinkLayerDevices(allArgs...)
	c.Assert(err, jc.ErrorIsNil)

	var results []*state.LinkLayerDevice
	machineID, modelUUID := s.machine.Id(), s.State.ModelUUID()
	for _, args := range allArgs {
		device, err := s.machine.LinkLayerDevice(args.Name)
		c.Check(err, jc.ErrorIsNil)
		s.checkSetDeviceMatchesArgs(c, device, args)
		s.checkSetDeviceMatchesMachineIDAndModelUUID(c, device, machineID, modelUUID)
		results = append(results, device)
	}
	return results
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesMultipleChildrenOfExistingParentSucceeds(c *gc.C) {
	s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "parent", "child1", "child2")
}

func (s *linkLayerDevicesStateSuite) addNamedParentDeviceWithChildrenAndCheckAllAdded(c *gc.C, parentName string, childrenNames ...string) (
	parent *state.LinkLayerDevice,
	children []*state.LinkLayerDevice,
) {
	parent = s.addNamedDevice(c, parentName)
	childrenArgs := make([]state.LinkLayerDeviceArgs, len(childrenNames))
	for i, childName := range childrenNames {
		childrenArgs[i] = state.LinkLayerDeviceArgs{
			Name:       childName,
			Type:       corenetwork.EthernetDevice,
			ParentName: parentName,
		}
	}

	children = s.setMultipleDevicesSucceedsAndCheckAllAdded(c, childrenArgs)
	return parent, children
}

func (s *linkLayerDevicesStateSuite) addSimpleDevice(c *gc.C) *state.LinkLayerDevice {
	return s.addNamedDevice(c, "foo")
}

func (s *linkLayerDevicesStateSuite) addNamedDevice(c *gc.C, name string) *state.LinkLayerDevice {
	args := state.LinkLayerDeviceArgs{
		Name: name,
		Type: corenetwork.EthernetDevice,
	}
	ops, err := s.machine.AddLinkLayerDeviceOps(args)
	c.Assert(err, jc.ErrorIsNil)
	state.RunTransaction(c, s.State, ops)

	device, err := s.machine.LinkLayerDevice(name)
	c.Assert(err, jc.ErrorIsNil)
	return device
}

func (s *linkLayerDevicesStateSuite) TestMachineMethodReturnsNotFoundErrorWhenMissing(c *gc.C) {
	device := s.addSimpleDevice(c)

	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove(nil)
	c.Assert(err, jc.ErrorIsNil)

	result, err := device.Machine()
	c.Assert(err, gc.ErrorMatches, "machine 0 not found")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(result, gc.IsNil)
}

func (s *linkLayerDevicesStateSuite) TestMachineMethodReturnsMachine(c *gc.C) {
	device := s.addSimpleDevice(c)

	result, err := device.Machine()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, s.machine)
}

func (s *linkLayerDevicesStateSuite) TestParentDeviceReturnsLinkLayerDevice(c *gc.C) {
	parent, children := s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "br-eth0", "eth0")

	child := children[0]
	parentCopy, err := child.ParentDevice()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(parentCopy, jc.DeepEquals, parent)
}

func (s *linkLayerDevicesStateSuite) TestMachineLinkLayerDeviceReturnsNotFoundErrorWhenMissing(c *gc.C) {
	result, err := s.machine.LinkLayerDevice("missing")
	c.Assert(result, gc.IsNil)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(err, gc.ErrorMatches, "device with ID .+ not found")
}

func (s *linkLayerDevicesStateSuite) TestMachineLinkLayerDeviceReturnsLinkLayerDevice(c *gc.C) {
	existingDevice := s.addSimpleDevice(c)

	result, err := s.machine.LinkLayerDevice(existingDevice.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, existingDevice)
}

func (s *linkLayerDevicesStateSuite) TestMachineAllLinkLayerDevices(c *gc.C) {
	s.assertNoDevicesOnMachine(c, s.machine)
	topParent, secondLevelParents := s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "br-bond0", "bond0")
	secondLevelParent := secondLevelParents[0]

	secondLevelChildrenArgs := []state.LinkLayerDeviceArgs{{
		Name:       "eth0",
		Type:       corenetwork.EthernetDevice,
		ParentName: secondLevelParent.Name(),
	}, {
		Name:       "eth1",
		Type:       corenetwork.EthernetDevice,
		ParentName: secondLevelParent.Name(),
	}}
	s.setMultipleDevicesSucceedsAndCheckAllAdded(c, secondLevelChildrenArgs)

	results, err := s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 4)
	for _, result := range results {
		c.Check(result, gc.NotNil)
		c.Check(result.MachineID(), gc.Equals, s.machine.Id())
		c.Check(result.Name(), gc.Matches, `(br-bond0|bond0|eth0|eth1)`)
		if result.Name() == topParent.Name() {
			c.Check(result.ParentName(), gc.Equals, "")
			continue
		}
		c.Check(result.ParentName(), gc.Matches, `(br-bond0|bond0)`)
	}
}

func (s *linkLayerDevicesStateSuite) TestMachineAllProviderInterfaceInfos(c *gc.C) {
	err := s.machine.SetLinkLayerDevices(state.LinkLayerDeviceArgs{
		Name:       "sara-lynn",
		MACAddress: "ab:cd:ef:01:23:45",
		ProviderID: "thing1",
		Type:       corenetwork.EthernetDevice,
	}, state.LinkLayerDeviceArgs{
		Name:       "bojack",
		MACAddress: "ab:cd:ef:01:23:46",
		ProviderID: "thing2",
		Type:       corenetwork.EthernetDevice,
	})
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.machine.AllProviderInterfaceInfos()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.SameContents, []corenetwork.ProviderInterfaceInfo{{
		InterfaceName:   "sara-lynn",
		HardwareAddress: "ab:cd:ef:01:23:45",
		ProviderId:      "thing1",
	}, {
		InterfaceName:   "bojack",
		HardwareAddress: "ab:cd:ef:01:23:46",
		ProviderId:      "thing2",
	}})
}

func (s *linkLayerDevicesStateSuite) assertNoDevicesOnMachine(c *gc.C, machine *state.Machine) {
	s.assertAllLinkLayerDevicesOnMachineMatchCount(c, machine, 0)
}

func (s *linkLayerDevicesStateSuite) assertAllLinkLayerDevicesOnMachineMatchCount(c *gc.C, machine *state.Machine, expectedCount int) {
	results, err := machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, expectedCount)
}

func (s *linkLayerDevicesStateSuite) TestMachineAllLinkLayerDevicesOnlyReturnsSameModelDevices(c *gc.C) {
	s.assertNoDevicesOnMachine(c, s.machine)
	s.assertNoDevicesOnMachine(c, s.otherStateMachine)

	s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "foo", "foo.42")

	results, err := s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)

	deviceNames := make([]string, 2)
	for i, res := range results {
		deviceNames[i] = res.Name()
	}
	c.Assert(deviceNames, jc.SameContents, []string{"foo", "foo.42"})

	s.assertNoDevicesOnMachine(c, s.otherStateMachine)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDeviceRemoveFailsWithExistingChildren(c *gc.C) {
	parent, _ := s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "parent", "one-child", "another-child")

	err := parent.Remove()
	expectedError := fmt.Sprintf(
		"cannot remove %s: parent device %q has 2 children",
		parent, parent.Name(),
	)
	c.Assert(err, gc.ErrorMatches, expectedError)
	c.Assert(err, jc.Satisfies, state.IsParentDeviceHasChildrenError)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerParentRemoveOKAfterChangingChildrensToNewParent(c *gc.C) {
	originalParent, children := s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "parent", "one-child", "another-child")
	newParent := s.addNamedDevice(c, "new-parent")

	updateArgs := []state.LinkLayerDeviceArgs{{
		Name:       children[0].Name(),
		Type:       children[0].Type(),
		ParentName: newParent.Name(),
	}, {
		Name:       children[1].Name(),
		Type:       children[1].Type(),
		ParentName: newParent.Name(),
	}}
	err := s.machine.SetLinkLayerDevices(updateArgs...)
	c.Assert(err, jc.ErrorIsNil)

	err = originalParent.Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = newParent.Remove()
	expectedError := fmt.Sprintf(
		"cannot remove %s: parent device %q has 2 children",
		newParent, newParent.Name(),
	)
	c.Assert(err, gc.ErrorMatches, expectedError)
	c.Assert(err, jc.Satisfies, state.IsParentDeviceHasChildrenError)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDeviceRemoveSuccess(c *gc.C) {
	existingDevice := s.addSimpleDevice(c)

	s.removeDeviceAndAssertSuccess(c, existingDevice)
	s.assertNoDevicesOnMachine(c, s.machine)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDeviceRemoveRemovesProviderID(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "foo",
		Type:       corenetwork.EthernetDevice,
		ProviderID: "bar",
	}
	err := s.machine.SetLinkLayerDevices(args)
	c.Assert(err, jc.ErrorIsNil)
	device, err := s.machine.LinkLayerDevice("foo")
	c.Assert(err, jc.ErrorIsNil)

	s.removeDeviceAndAssertSuccess(c, device)
	// Re-adding the same device should now succeed.
	err = s.machine.SetLinkLayerDevices(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesNoop(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: corenetwork.EthernetDevice,
	}
	err := s.machine.SetLinkLayerDevices(args)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetLinkLayerDevices(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesWithVirtualPort(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:            "foo",
		Type:            corenetwork.EthernetDevice,
		VirtualPortType: corenetwork.OvsPort,
	}
	err := s.machine.SetLinkLayerDevices(args)
	c.Assert(err, jc.ErrorIsNil)

	devs, err := s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(devs, gc.HasLen, 1)
	c.Assert(devs[0].VirtualPortType(), gc.Equals, corenetwork.OvsPort, gc.Commentf("virtual port type field was not persisted"))
}

func (s *linkLayerDevicesStateSuite) removeDeviceAndAssertSuccess(c *gc.C, givenDevice *state.LinkLayerDevice) {
	err := givenDevice.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDeviceRemoveTwiceStillSucceeds(c *gc.C) {
	existingDevice := s.addSimpleDevice(c)

	s.removeDeviceAndAssertSuccess(c, existingDevice)
	s.removeDeviceAndAssertSuccess(c, existingDevice)
	s.assertNoDevicesOnMachine(c, s.machine)
}

func (s *linkLayerDevicesStateSuite) TestMachineRemoveAllLinkLayerDevicesSuccess(c *gc.C) {
	s.assertNoDevicesOnMachine(c, s.machine)
	s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "foo", "bar")

	err := s.machine.RemoveAllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoDevicesOnMachine(c, s.machine)
}

func (s *linkLayerDevicesStateSuite) TestMachineRemoveAllLinkLayerDevicesNoErrorIfNoDevicesExist(c *gc.C) {
	s.assertNoDevicesOnMachine(c, s.machine)

	err := s.machine.RemoveAllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) TestSetProviderIDOps(c *gc.C) {
	dev1 := s.addNamedDevice(c, "foo")

	ops, err := dev1.SetProviderIDOps("p1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.Not(gc.HasLen), 0)

	state.RunTransaction(c, s.State, ops)

	dev1, err = s.machine.LinkLayerDevice("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dev1.ProviderID().String(), gc.Equals, "p1")

	// No-op if already set.
	ops, err = dev1.SetProviderIDOps("p1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.HasLen, 0)

	// Error if ID already used.
	dev2 := s.addNamedDevice(c, "bar")
	_, err = dev2.SetProviderIDOps("p1")
	c.Assert(err, gc.ErrorMatches, "provider IDs not unique: p1")

	// Unset the ID.
	ops, err = dev1.SetProviderIDOps("")
	c.Assert(err, jc.ErrorIsNil)
	state.RunTransaction(c, s.State, ops)

	dev1, err = s.machine.LinkLayerDevice("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dev1.ProviderID().String(), gc.Equals, "")

	// The global ID is unregistered, so we should be able to reset it.
	ops, err = dev1.SetProviderIDOps("p1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.Not(gc.HasLen), 0)

	// We should be able to change the ID, provided the new ID is unused.
	ops, err = dev1.SetProviderIDOps("p2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ops, gc.Not(gc.HasLen), 0)
}

func (s *linkLayerDevicesStateSuite) TestRemoveOps(c *gc.C) {
	dev := s.addNamedDevice(c, "eth0")

	state.RunTransaction(c, s.State, dev.RemoveOps())

	_, err := s.State.LinkLayerDevice(dev.DocID())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *linkLayerDevicesStateSuite) TestUpdateOps(c *gc.C) {
	dev := s.addNamedDevice(c, "eth0")

	ops := dev.UpdateOps(state.LinkLayerDeviceArgs{
		Name: "eth0",
		Type: corenetwork.EthernetDevice,
	})
	c.Check(ops, gc.HasLen, 0)

	mac := corenetwork.GenerateVirtualMACAddress()
	ops = dev.UpdateOps(state.LinkLayerDeviceArgs{
		Name:       "eth0",
		Type:       corenetwork.EthernetDevice,
		MACAddress: mac,
	})
	c.Assert(ops, gc.HasLen, 1)

	state.RunTransaction(c, s.State, ops)

	dev, err := s.machine.LinkLayerDevice("eth0")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(dev.MACAddress(), gc.Equals, mac)
}

func (s *linkLayerDevicesStateSuite) TestEthernetDeviceForBridge(c *gc.C) {
	_, err := s.State.AddSubnet(corenetwork.SubnetInfo{
		CIDR:       "10.0.0.0/24",
		ProviderId: "ps-01",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.createBridgeWithIP(c, s.machine, "br0", "10.0.0.9/24")

	dev, err := s.machine.LinkLayerDevice("br0")
	c.Assert(err, jc.ErrorIsNil)

	child, err := dev.EthernetDeviceForBridge("eth0", true)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(child.InterfaceName, gc.Equals, "eth0")
	c.Check(child.ConfigType, gc.Equals, corenetwork.ConfigStatic)
	c.Check(child.ParentInterfaceName, gc.Equals, "br0")
	c.Check(child.PrimaryAddress().CIDR, gc.Equals, "10.0.0.0/24")
	c.Check(child.ProviderSubnetId, gc.Equals, corenetwork.Id("ps-01"))
	c.Check(child.MTU, gc.Equals, int(dev.MTU()))

	child, err = dev.EthernetDeviceForBridge("eth0", false)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(child.ConfigType, gc.Equals, corenetwork.ConfigDHCP)
	c.Check(child.ProviderSubnetId, gc.Equals, corenetwork.Id(""))

	dev = s.addNamedDevice(c, "bond0")
	_, err = dev.EthernetDeviceForBridge("eth0", false)
	c.Assert(err, gc.NotNil)
}

func (s *linkLayerDevicesStateSuite) TestEthernetDeviceForBridgeFanMTU(c *gc.C) {
	_, err := s.State.AddSubnet(corenetwork.SubnetInfo{
		CIDR:       "10.0.0.0/24",
		ProviderId: "ps-01",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddSubnet(corenetwork.SubnetInfo{
		CIDR: "250.0.0.0/8",
		FanInfo: &corenetwork.FanCIDRs{
			FanOverlay:       "240.0.0.0/4",
			FanLocalUnderlay: "10.0.0.0/24",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	fanBridgeName := "fan-250"

	// Both of these devices are created with MTU=1450.
	s.createNICWithIP(c, s.machine, "enp5s0", "10.0.0.6/24")
	s.createBridgeWithIP(c, s.machine, fanBridgeName, "250.0.0.9/8")

	// Create the VXLAN device used by the Fan.
	err = s.machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       "ftun0",
			Type:       corenetwork.VXLANDevice,
			ParentName: fanBridgeName,
			IsUp:       true,
			MTU:        1400,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	dev, err := s.machine.LinkLayerDevice("fan-250")
	c.Assert(err, jc.ErrorIsNil)

	child, err := dev.EthernetDeviceForBridge("eth0", true)
	c.Assert(err, jc.ErrorIsNil)

	// A child device of the fan should get an MTU equal to the VXLAN.
	c.Assert(child.MTU, gc.Equals, 1400)
}

func (s *linkLayerDevicesStateSuite) TestAddAddressOps(c *gc.C) {
	dev := s.addNamedDevice(c, "eth0")

	ops, err := dev.AddAddressOps(state.LinkLayerDeviceAddress{
		DeviceName:  "", // Not required.
		CIDRAddress: "10.1.1.1/24",
		Origin:      corenetwork.OriginMachine,
		IsSecondary: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	state.RunTransaction(c, s.State, ops)

	dev, err = s.machine.LinkLayerDevice("eth0")
	c.Assert(err, jc.ErrorIsNil)

	addrs, err := dev.Addresses()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(addrs, gc.HasLen, 1)
	c.Check(addrs[0].Value(), gc.Equals, "10.1.1.1")
	c.Check(addrs[0].IsSecondary(), gc.Equals, true)
}

func (s *linkLayerDevicesStateSuite) TestAddDeviceOpsWithAddresses(c *gc.C) {
	devName := "eth0"

	devArgs := state.LinkLayerDeviceArgs{
		Name: devName,
		Type: corenetwork.EthernetDevice,
	}

	addrArgs := state.LinkLayerDeviceAddress{
		DeviceName:  devName,
		CIDRAddress: "10.1.1.1/24",
		Origin:      corenetwork.OriginMachine,
	}

	ops, err := s.machine.AddLinkLayerDeviceOps(devArgs, addrArgs)
	c.Assert(err, jc.ErrorIsNil)

	state.RunTransaction(c, s.State, ops)

	_, err = s.machine.LinkLayerDevice(devName)
	c.Assert(err, jc.ErrorIsNil)

	dev, err := s.machine.LinkLayerDevice("eth0")
	c.Assert(err, jc.ErrorIsNil)

	addrs, err := dev.Addresses()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(addrs, gc.HasLen, 1)
	c.Assert(addrs[0].Value(), gc.Equals, "10.1.1.1")
}

func (s *linkLayerDevicesStateSuite) createSpaceAndSubnetWithProviderID(c *gc.C, spaceName, CIDR, providerSubnetID string) {
	space, err := s.State.AddSpace(spaceName, corenetwork.Id(spaceName), nil)
	c.Assert(err, jc.ErrorIsNil)
	spaceInfo, err := space.NetworkSpace()
	c.Assert(err, gc.IsNil)
	s.spaces[spaceName] = spaceInfo

	_, err = s.State.AddSubnet(corenetwork.SubnetInfo{
		CIDR:       CIDR,
		SpaceID:    space.Id(),
		ProviderId: corenetwork.Id(providerSubnetID),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) createNICWithIP(c *gc.C, machine *state.Machine, deviceName, cidrAddress string) {
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       deviceName,
			Type:       corenetwork.EthernetDevice,
			ParentName: "",
			IsUp:       true,
			MTU:        1450,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   deviceName,
			CIDRAddress:  cidrAddress,
			ConfigMethod: corenetwork.ConfigStatic,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) createBridgeWithIP(c *gc.C, machine *state.Machine, bridgeName, cidrAddress string) {
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       bridgeName,
			Type:       corenetwork.BridgeDevice,
			ParentName: "",
			IsUp:       true,
			MTU:        1450,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   bridgeName,
			CIDRAddress:  cidrAddress,
			ConfigMethod: corenetwork.ConfigStatic,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesWithLightStateChurn(c *gc.C) {
	childArgs, churnHook := s.prepareSetLinkLayerDevicesWithStateChurn(c)
	defer state.SetTestHooks(c, s.State, churnHook).Check()
	s.assertAllLinkLayerDevicesOnMachineMatchCount(c, s.machine, 1) // parent only

	err := s.machine.SetLinkLayerDevices(childArgs)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAllLinkLayerDevicesOnMachineMatchCount(c, s.machine, 2) // both parent and child remain
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesUpdatesExistingDocs(c *gc.C) {
	s.assertNoDevicesOnMachine(c, s.machine)
	parent, children := s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "foo", "bar")

	// Change everything that's possible to change for both existing devices,
	// except for ProviderID and ParentName (tested separately).
	updateArgs := []state.LinkLayerDeviceArgs{{
		Name:        parent.Name(),
		Type:        corenetwork.BondDevice,
		MTU:         1234,
		MACAddress:  "aa:bb:cc:dd:ee:f0",
		IsAutoStart: true,
		IsUp:        true,
	}, {
		Name:        children[0].Name(),
		Type:        corenetwork.VLAN8021QDevice,
		MTU:         4321,
		MACAddress:  "aa:bb:cc:dd:ee:f1",
		IsAutoStart: true,
		IsUp:        true,
		ParentName:  parent.Name(),
	}}
	err := s.machine.SetLinkLayerDevices(updateArgs...)
	c.Assert(err, jc.ErrorIsNil)

	allDevices, err := s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allDevices, gc.HasLen, 2)

	for _, device := range allDevices {
		if device.Name() == parent.Name() {
			s.checkSetDeviceMatchesArgs(c, device, updateArgs[0])
		} else {
			s.checkSetDeviceMatchesArgs(c, device, updateArgs[1])
		}
		s.checkSetDeviceMatchesMachineIDAndModelUUID(c, device, s.machine.Id(), s.State.ModelUUID())
	}
}

func (s *linkLayerDevicesStateSuite) prepareSetLinkLayerDevicesWithStateChurn(c *gc.C) (state.LinkLayerDeviceArgs, jujutxn.TestHook) {
	parent := s.addNamedDevice(c, "parent")
	childArgs := state.LinkLayerDeviceArgs{
		Name:       "child",
		Type:       corenetwork.EthernetDevice,
		ParentName: parent.Name(),
	}

	churnHook := jujutxn.TestHook{
		Before: func() {
			s.assertAllLinkLayerDevicesOnMachineMatchCount(c, s.machine, 1) // just the parent
			err := s.machine.SetLinkLayerDevices(childArgs)
			c.Assert(err, jc.ErrorIsNil)
		},
		After: func() {
			s.assertAllLinkLayerDevicesOnMachineMatchCount(c, s.machine, 2) // parent and child
			child, err := s.machine.LinkLayerDevice("child")
			c.Assert(err, jc.ErrorIsNil)
			err = child.Remove()
			c.Assert(err, jc.ErrorIsNil)
		},
	}

	return childArgs, churnHook
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesWithModerateStateChurn(c *gc.C) {
	childArgs, churnHook := s.prepareSetLinkLayerDevicesWithStateChurn(c)
	defer state.SetTestHooks(c, s.State, churnHook, churnHook).Check()
	s.assertAllLinkLayerDevicesOnMachineMatchCount(c, s.machine, 1) // parent only

	err := s.machine.SetLinkLayerDevices(childArgs)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAllLinkLayerDevicesOnMachineMatchCount(c, s.machine, 2) // both parent and child remain
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesWithTooMuchStateChurn(c *gc.C) {
	childArgs, churnHook := s.prepareSetLinkLayerDevicesWithStateChurn(c)
	state.SetMaxTxnAttempts(c, s.State, 3)
	defer state.SetTestHooks(c, s.State, churnHook, churnHook, churnHook).Check()
	s.assertAllLinkLayerDevicesOnMachineMatchCount(c, s.machine, 1) // parent only

	err := s.machine.SetLinkLayerDevices(childArgs)
	c.Assert(errors.Cause(err), gc.Equals, jujutxn.ErrExcessiveContention)
	s.assertAllLinkLayerDevicesOnMachineMatchCount(c, s.machine, 1) // only the parent remains
}

func (s *linkLayerDevicesStateSuite) addContainerMachine(c *gc.C) {
	// Add a container machine with s.machine as its host.
	containerTemplate := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(containerTemplate, s.machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	s.containerMachine = container
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesAllowsParentBridgeDeviceForContainerDevice(c *gc.C) {
	// Add default bridges per container type to ensure they will be skipped
	// when deciding which host bridges to use for the container NICs.
	s.addParentBridgeDeviceWithContainerDevicesAsChildren(c, network.DefaultLXDBridge, "vethX", 1)
	s.addParentBridgeDeviceWithContainerDevicesAsChildren(c, network.DefaultKVMBridge, "vethY", 1)
	parentDevice, _ := s.addParentBridgeDeviceWithContainerDevicesAsChildren(c, "br-eth1.250", "eth", 1)
	childDevice, err := s.containerMachine.LinkLayerDevice("eth0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(childDevice.Name(), gc.Equals, "eth0")
	c.Check(childDevice.ParentName(), gc.Equals, "m#0#d#br-eth1.250")
	c.Check(childDevice.MachineID(), gc.Equals, s.containerMachine.Id())
	parentOfChildDevice, err := childDevice.ParentDevice()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(parentOfChildDevice, jc.DeepEquals, parentDevice)
}

func (s *linkLayerDevicesStateSuite) addParentBridgeDeviceWithContainerDevicesAsChildren(
	c *gc.C,
	parentName string,
	childDevicesNamePrefix string,
	numChildren int,
) (parentDevice *state.LinkLayerDevice, childrenDevices []*state.LinkLayerDevice) {
	parentArgs := state.LinkLayerDeviceArgs{
		Name: parentName,
		Type: corenetwork.BridgeDevice,
	}
	parentDevice = s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, parentArgs)
	parentDeviceGlobalKey := "m#" + s.machine.Id() + "#d#" + parentName

	childrenArgsTemplate := state.LinkLayerDeviceArgs{
		Type:       corenetwork.EthernetDevice,
		ParentName: parentDeviceGlobalKey,
	}
	childrenArgs := make([]state.LinkLayerDeviceArgs, numChildren)
	for i := 0; i < numChildren; i++ {
		childrenArgs[i] = childrenArgsTemplate
		childrenArgs[i].Name = fmt.Sprintf("%s%d", childDevicesNamePrefix, i)
	}
	s.addContainerMachine(c)
	err := s.containerMachine.SetLinkLayerDevices(childrenArgs...)
	c.Assert(err, jc.ErrorIsNil)
	childrenDevices, err = s.containerMachine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	return parentDevice, childrenDevices
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDeviceRemoveFailsWithExistingChildrenOnContainerMachine(c *gc.C) {
	parent, children := s.addParentBridgeDeviceWithContainerDevicesAsChildren(c, "br-eth1", "eth", 2)

	err := parent.Remove()
	expectedErrorPrefix := fmt.Sprintf("cannot remove %s: parent device %q has ", parent, parent.Name())
	c.Assert(err, gc.ErrorMatches, expectedErrorPrefix+"2 children")
	c.Assert(err, jc.Satisfies, state.IsParentDeviceHasChildrenError)

	err = children[0].Remove()
	c.Assert(err, jc.ErrorIsNil)

	err = parent.Remove()
	c.Assert(err, gc.ErrorMatches, expectedErrorPrefix+"1 children")
	c.Assert(err, jc.Satisfies, state.IsParentDeviceHasChildrenError)

	err = children[1].Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = parent.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesUpdatesBothExistingAndNewParents(c *gc.C) {
	parent1, children1 := s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "parent1", "child1", "child2")
	parent2, children2 := s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "parent2", "child3", "child4")

	// Swap the parents of all children.
	updateArgs := make([]state.LinkLayerDeviceArgs, 0, len(children1)+len(children2))
	for _, child := range children1 {
		updateArgs = append(updateArgs, state.LinkLayerDeviceArgs{
			Name:       child.Name(),
			Type:       child.Type(),
			ParentName: parent2.Name(),
		})
	}
	for _, child := range children2 {
		updateArgs = append(updateArgs, state.LinkLayerDeviceArgs{
			Name:       child.Name(),
			Type:       child.Type(),
			ParentName: parent1.Name(),
		})
	}
	err := s.machine.SetLinkLayerDevices(updateArgs...)
	c.Assert(err, jc.ErrorIsNil)

	allDevices, err := s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allDevices, gc.HasLen, len(updateArgs)+2) // 4 children updated and 2 parents unchanged.

	for _, device := range allDevices {
		switch device.Name() {
		case children1[0].Name(), children1[1].Name():
			c.Check(device.ParentName(), gc.Equals, parent2.Name())
		case children2[0].Name(), children2[1].Name():
			c.Check(device.ParentName(), gc.Equals, parent1.Name())
		}
	}
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesUpdatesParentWhenNotSet(c *gc.C) {
	parent := s.addNamedDevice(c, "parent")
	child := s.addNamedDevice(c, "child")

	updateArgs := state.LinkLayerDeviceArgs{
		Name:       child.Name(),
		Type:       child.Type(),
		ParentName: parent.Name(), // make "child" a child of "parent"
	}
	err := s.machine.SetLinkLayerDevices(updateArgs)
	c.Assert(err, jc.ErrorIsNil)

	err = parent.Remove()
	c.Assert(err, gc.ErrorMatches,
		`cannot remove ethernet device "parent" on machine "0": parent device "parent" has 1 children`,
	)
	c.Assert(err, jc.Satisfies, state.IsParentDeviceHasChildrenError)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesUpdatesParentWhenSet(c *gc.C) {
	parent, children := s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "parent", "child")
	err := parent.Remove()
	c.Assert(err, jc.Satisfies, state.IsParentDeviceHasChildrenError)

	updateArgs := state.LinkLayerDeviceArgs{
		Name: children[0].Name(),
		Type: children[0].Type(),
		// make "child" no longer a child of "parent"
	}
	err = s.machine.SetLinkLayerDevices(updateArgs)
	c.Assert(err, jc.ErrorIsNil)

	err = parent.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesToContainerWhenContainerDeadBeforehand(c *gc.C) {
	beforeHook := func() {
		// Make the container Dead but keep it around.
		err := s.containerMachine.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
	}

	s.assertSetLinkLayerDevicesToContainerFailsWithBeforeHook(c, beforeHook, `.*machine "0/lxd/0" not alive`)
}

func (s *linkLayerDevicesStateSuite) assertSetLinkLayerDevicesToContainerFailsWithBeforeHook(c *gc.C, beforeHook func(), expectedError string) {
	_, children := s.addParentBridgeDeviceWithContainerDevicesAsChildren(c, "br-eth1", "eth", 1)
	defer state.SetBeforeHooks(c, s.State, beforeHook).Check()

	newChildArgs := state.LinkLayerDeviceArgs{
		Name:       "eth1",
		Type:       corenetwork.EthernetDevice,
		ParentName: children[0].ParentName(),
	}
	err := s.containerMachine.SetLinkLayerDevices(newChildArgs)
	c.Assert(err, gc.ErrorMatches, expectedError)
}

func (s *linkLayerDevicesStateSuite) TestMachineRemoveAlsoRemoveAllLinkLayerDevices(c *gc.C) {
	s.assertNoDevicesOnMachine(c, s.machine)
	s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "foo", "bar")

	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove(nil)
	c.Assert(err, jc.ErrorIsNil)

	s.assertNoDevicesOnMachine(c, s.machine)
}

func (s *linkLayerDevicesStateSuite) TestSetDeviceAddressesWithSubnetID(c *gc.C) {
	s.createSpaceAndSubnetWithProviderID(c, "public", "10.0.0.0/24", "prov-0000")
	s.createSpaceAndSubnetWithProviderID(c, "private", "10.20.0.0/24", "prov-ffff")
	s.createSpaceAndSubnetWithProviderID(c, "dmz", "10.30.0.0/24", "prov-abcd")
	s.createNICWithIP(c, s.machine, "eth0", "10.0.0.11/24")
	s.createNICWithIP(c, s.machine, "eth1", "10.20.0.42/24")
	// Create eth2 NIC but don't assign an IP yet. This allows us to
	// exercise the both the insert and update code-paths when calling
	// SetDevicesAddresses.
	err := s.machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       "eth2",
			Type:       corenetwork.EthernetDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:        "eth1",
			ConfigMethod:      corenetwork.ConfigStatic,
			ProviderNetworkID: "vpc-abcd",
			ProviderSubnetID:  "prov-ffff",
			CIDRAddress:       "10.20.0.42/24",
		},
		state.LinkLayerDeviceAddress{
			DeviceName:        "eth2",
			ConfigMethod:      corenetwork.ConfigStatic,
			ProviderNetworkID: "vpc-abcd",
			ProviderSubnetID:  "prov-abcd",
			CIDRAddress:       "10.30.0.99/24",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	allAddr, err := s.machine.AllDeviceAddresses()
	c.Assert(err, gc.IsNil)

	expSubnetID := map[string]corenetwork.Id{
		"eth1": "prov-ffff",
		"eth2": "prov-abcd",
	}
nextDev:
	for devName, expID := range expSubnetID {
		for _, addr := range allAddr {
			if addr.DeviceName() != devName {
				continue
			}

			c.Assert(addr.ProviderSubnetID(), gc.Equals, expID, gc.Commentf("subnetID for device %q", devName))
			continue nextDev
		}
		c.Fatalf("unable to locate device %q while enumerating machine addresses", devName)
	}
}
