// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	"github.com/juju/juju/network/containerizer"
	"github.com/juju/juju/state"
)

// linkLayerDevicesStateSuite contains white-box tests for link-layer network
// devices, which include access to mongo.
type linkLayerDevicesStateSuite struct {
	ConnSuite

	machine          *state.Machine
	containerMachine *state.Machine

	otherState        *state.State
	otherStateMachine *state.Machine

	bridgePolicy *containerizer.BridgePolicy
}

var _ = gc.Suite(&linkLayerDevicesStateSuite{})

func (s *linkLayerDevicesStateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.otherState = s.NewStateForModelNamed(c, "other-model")
	s.otherStateMachine, err = s.otherState.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	s.bridgePolicy = &containerizer.BridgePolicy{}
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesNoArgs(c *gc.C) {
	err := s.machine.SetLinkLayerDevices() // takes varargs, which includes none.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesEmptyArgs(c *gc.C) {
	args := state.LinkLayerDeviceArgs{}
	s.assertSetLinkLayerDevicesReturnsNotValidError(c, args, "empty Name not valid")
}

func (s *linkLayerDevicesStateSuite) assertSetLinkLayerDevicesReturnsNotValidError(c *gc.C, args state.LinkLayerDeviceArgs, errorCauseMatches string) {
	err := s.assertSetLinkLayerDevicesFailsValidationForArgs(c, args, errorCauseMatches)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
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

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesSameNameAndParentName(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "foo",
		ParentName: "foo",
	}
	s.assertSetLinkLayerDevicesReturnsNotValidError(c, args, `Name and ParentName must be different`)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesInvalidType(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name: "bar",
		Type: "bad type",
	}
	s.assertSetLinkLayerDevicesReturnsNotValidError(c, args, `Type "bad type" not valid`)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesParentNameAsInvalidGlobalKey(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "eth0",
		ParentName: "x#foo#y#bar", // contains the right amount of # but is invalid otherwise.
	}
	s.assertSetLinkLayerDevicesReturnsNotValidError(c, args, `ParentName "x#foo#y#bar" format not valid`)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesParentNameAsGlobalKeyFailsForNonContainerMachine(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "eth0",
		ParentName: "m#42#d#foo", // any non-container ID here will cause the same error.
	}
	s.assertSetLinkLayerDevicesReturnsNotValidError(c, args, `ParentName "m#42#d#foo" for non-container machine "0" not valid`)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesParentNameAsGlobalKeyFailsForContainerOnDifferentHost(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "eth0",
		ParentName: "m#42#d#foo", // any ID other than s.containerMachine's parent ID here will cause the same error.
	}
	s.addContainerMachine(c)
	err := s.containerMachine.SetLinkLayerDevices(args)
	errorPrefix := fmt.Sprintf("cannot set link-layer devices to machine %q: invalid device %q: ", s.containerMachine.Id(), args.Name)
	c.Assert(err, gc.ErrorMatches, errorPrefix+`ParentName "m#42#d#foo" on non-host machine "42" not valid`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesParentNameAsGlobalKeyFailsForContainerIfParentMissing(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "eth0",
		ParentName: "m#0#d#missing",
	}
	s.addContainerMachine(c)
	err := s.containerMachine.SetLinkLayerDevices(args)
	c.Assert(err, gc.ErrorMatches, `.*parent device "missing" on host machine "0" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesInvalidMACAddress(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "eth0",
		Type:       state.EthernetDevice,
		MACAddress: "bad mac",
	}
	s.assertSetLinkLayerDevicesReturnsNotValidError(c, args, `MACAddress "bad mac" not valid`)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesWhenMachineNotAliveOrGone(c *gc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	args := state.LinkLayerDeviceArgs{
		Name: "eth0",
		Type: state.EthernetDevice,
	}
	s.assertSetLinkLayerDevicesFailsForArgs(c, args, `machine "0" not alive`)

	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertSetLinkLayerDevicesFailsForArgs(c, args, `machine "0" not alive`)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesWithMissingParentSameMachine(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "eth0",
		Type:       state.EthernetDevice,
		ParentName: "br-eth0",
	}
	s.assertSetLinkLayerDevicesReturnsNotValidError(c, args, `ParentName not valid: device "br-eth0" on machine "0" not found`)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesNoParentSuccess(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:        "eth0.42",
		MTU:         9000,
		ProviderID:  "eni-42",
		Type:        state.VLAN_8021QDevice,
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
		Type: state.EthernetDevice,
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesWithDuplicateProviderIDFailsInSameModel(c *gc.C) {
	args1 := state.LinkLayerDeviceArgs{
		Name:       "eth0.42",
		Type:       state.EthernetDevice,
		ProviderID: "42",
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args1)

	args2 := args1
	args2.Name = "br-eth0"
	err := s.assertSetLinkLayerDevicesFailsValidationForArgs(c, args2, `ProviderID\(s\) not unique: 42`)
	c.Assert(err, jc.Satisfies, state.IsProviderIDNotUniqueError)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesWithDuplicateNameAndProviderIDSucceedsInDifferentModels(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "eth0.42",
		Type:       state.EthernetDevice,
		ProviderID: "42",
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)

	s.assertMachineSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, s.otherStateMachine, args, s.otherState.ModelUUID())
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesUpdatesProviderIDWhenNotSetOriginally(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: state.EthernetDevice,
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)

	args.ProviderID = "42"
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesFailsForProviderIDChange(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "foo",
		Type:       state.EthernetDevice,
		ProviderID: "42",
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)

	args.ProviderID = "43"
	s.assertSetLinkLayerDevicesFailsForArgs(c, args, `cannot change ProviderID of link layer device "foo"`)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesUpdateWithDuplicateProviderIDFails(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "foo",
		Type:       state.EthernetDevice,
		ProviderID: "42",
	}
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)
	args.Name = "bar"
	args.ProviderID = ""
	s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)

	args.ProviderID = "42"
	err := s.assertSetLinkLayerDevicesFailsValidationForArgs(c, args, `ProviderID\(s\) not unique: 42`)
	c.Assert(err, jc.Satisfies, state.IsProviderIDNotUniqueError)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesDoesNotClearProviderIDOnceSet(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "foo",
		Type:       state.EthernetDevice,
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
		Type: state.BridgeDevice,
	}
	foo2 := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: state.EthernetDevice,
	}
	err := s.machine.SetLinkLayerDevices(foo1, foo2)
	c.Assert(err, gc.ErrorMatches, `.*invalid device "foo": Name specified more than once`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesRefusesToAddParentAndChildrenInTheSameCall(c *gc.C) {
	allArgs := []state.LinkLayerDeviceArgs{{
		Name:       "child1",
		Type:       state.EthernetDevice,
		ParentName: "parent1",
	}, {
		Name: "parent1",
		Type: state.BridgeDevice,
	}}

	err := s.machine.SetLinkLayerDevices(allArgs...)
	c.Assert(err, gc.ErrorMatches, `cannot set link-layer devices to machine "0": `+
		`invalid device "child1": `+
		`ParentName not valid: `+
		`device "parent1" on machine "0" not found`,
	)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
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
			Type:       state.EthernetDevice,
			ParentName: parentName,
		}
	}

	children = s.setMultipleDevicesSucceedsAndCheckAllAdded(c, childrenArgs)
	return parent, children
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesMultipleChildrenOfExistingParentIdempotent(c *gc.C) {
	s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "parent", "child1", "child2")
	s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "parent", "child1", "child2")
}

func (s *linkLayerDevicesStateSuite) addSimpleDevice(c *gc.C) *state.LinkLayerDevice {
	return s.addNamedDevice(c, "foo")
}

func (s *linkLayerDevicesStateSuite) addNamedDevice(c *gc.C, name string) *state.LinkLayerDevice {
	args := state.LinkLayerDeviceArgs{
		Name: name,
		Type: state.EthernetDevice,
	}
	err := s.machine.SetLinkLayerDevices(args)
	c.Assert(err, jc.ErrorIsNil)
	device, err := s.machine.LinkLayerDevice(name)
	c.Assert(err, jc.ErrorIsNil)
	return device
}

func (s *linkLayerDevicesStateSuite) TestMachineMethodReturnsNotFoundErrorWhenMissing(c *gc.C) {
	device := s.addSimpleDevice(c)

	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	result, err := device.Machine()
	c.Assert(err, gc.ErrorMatches, "machine 0 not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
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
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `device "missing" on machine "0" not found`)
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
		Type:       state.EthernetDevice,
		ParentName: secondLevelParent.Name(),
	}, {
		Name:       "eth1",
		Type:       state.EthernetDevice,
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
		Type:       state.EthernetDevice,
	}, state.LinkLayerDeviceArgs{
		Name:       "bojack",
		MACAddress: "ab:cd:ef:01:23:46",
		ProviderID: "thing2",
		Type:       state.EthernetDevice,
	})
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.machine.AllProviderInterfaceInfos()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.SameContents, []network.ProviderInterfaceInfo{{
		InterfaceName: "sara-lynn",
		MACAddress:    "ab:cd:ef:01:23:45",
		ProviderId:    "thing1",
	}, {
		InterfaceName: "bojack",
		MACAddress:    "ab:cd:ef:01:23:46",
		ProviderId:    "thing2",
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
	c.Assert(results[0].Name(), gc.Equals, "foo")
	c.Assert(results[1].Name(), gc.Equals, "foo.42")

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
		Type:       state.EthernetDevice,
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
		Type: state.EthernetDevice,
	}
	err := s.machine.SetLinkLayerDevices(args)
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.SetLinkLayerDevices(args)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *linkLayerDevicesStateSuite) createSpaceAndSubnet(c *gc.C, spaceName, CIDR string) {
	_, err := s.State.AddSpace(spaceName, corenetwork.Id(spaceName), nil, true)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSubnet(corenetwork.SubnetInfo{
		CIDR:      CIDR,
		SpaceName: spaceName,
	})
	c.Assert(err, jc.ErrorIsNil)
}

// setupTwoSpaces creates a 'default' and a 'dmz' space, each with a single
// registered subnet. 10.0.0.0/24 for 'default', and '10.10.0.0/24' for 'dmz'
func (s *linkLayerDevicesStateSuite) setupTwoSpaces(c *gc.C) {
	s.createSpaceAndSubnet(c, "default", "10.0.0.0/24")
	s.createSpaceAndSubnet(c, "dmz", "10.10.0.0/24")
}

func (s *linkLayerDevicesStateSuite) setupMachineWithOneNIC(c *gc.C) {
	s.setupTwoSpaces(c)
	// In the default space
	s.createNICWithIP(c, s.machine, "eth0", "10.0.0.20/24")
}

func (s *linkLayerDevicesStateSuite) createNICWithIP(c *gc.C, machine *state.Machine, deviceName, cidrAddress string) {
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       deviceName,
			Type:       state.EthernetDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   deviceName,
			CIDRAddress:  cidrAddress,
			ConfigMethod: state.StaticAddress,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) createLoopbackNIC(c *gc.C, machine *state.Machine) {
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       "lo",
			Type:       state.LoopbackDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   "lo",
			CIDRAddress:  "127.0.0.1/24",
			ConfigMethod: state.StaticAddress,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) createBridgeWithIP(c *gc.C, machine *state.Machine, bridgeName, cidrAddress string) {
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       bridgeName,
			Type:       state.BridgeDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   bridgeName,
			CIDRAddress:  cidrAddress,
			ConfigMethod: state.StaticAddress,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

// createAllDefaultDevices creates the loopback, lxcbr0, lxdbr0, and virbr0 devices
func (s *linkLayerDevicesStateSuite) createAllDefaultDevices(c *gc.C, machine *state.Machine) {
	// loopback
	s.createLoopbackNIC(c, s.machine)
	// container.DefaultLxcBridge
	s.createBridgeWithIP(c, s.machine, "lxcbr0", "10.0.3.1/24")
	// container.DefaultLxdBridge
	s.createBridgeWithIP(c, s.machine, "lxdbr0", "10.0.4.1/24")
	// container.DefaultKvmBridge
	s.createBridgeWithIP(c, s.machine, "virbr0", "192.168.124.1/24")
}

// createNICAndBridgeWithIP creates a network interface and a bridge on the
// machine, and assigns the requested CIDRAddress to the bridge.
func (s *linkLayerDevicesStateSuite) createNICAndBridgeWithIP(c *gc.C, machine *state.Machine, deviceName, bridgeName, cidrAddress string) {
	s.createBridgeWithIP(c, machine, bridgeName, cidrAddress)
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       deviceName,
			Type:       state.EthernetDevice,
			ParentName: bridgeName,
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDevicesForSpaces(c *gc.C) {
	s.setupTwoSpaces(c)
	// Is put into the 'default' space
	s.createNICAndBridgeWithIP(c, s.machine, "eth0", "br-eth0", "10.0.0.20/24")
	res, err := s.machine.LinkLayerDevicesForSpaces([]string{"default"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 1)
	devices, ok := res["default"]
	c.Check(ok, jc.IsTrue)
	// TODO(jam): 2016-12-13 Eventually we should probably notice that 'eth0'
	// *is* part of the right space, possibly just because its parent device
	// is in the space
	c.Check(devices, gc.HasLen, 1)
	c.Check(devices[0].Name(), gc.Equals, "br-eth0")
	c.Check(devices[0].Type(), gc.Equals, state.BridgeDevice)
}

func (s *linkLayerDevicesStateSuite) TestGetNetworkInfoForSpaces(c *gc.C) {
	s.setupTwoSpaces(c)
	s.createSpaceAndSubnet(c, "private", "10.20.0.0/24")
	s.createNICAndBridgeWithIP(c, s.machine, "eth0", "br-eth0", "10.0.0.20/24")
	s.createNICWithIP(c, s.machine, "eth1", "10.10.0.20/24")
	s.createNICWithIP(c, s.machine, "eth2", "10.20.0.20/24")
	s.machine.SetMachineAddresses(network.NewScopedAddress("10.0.0.20", network.ScopePublic),
		network.NewScopedAddress("10.10.0.20", network.ScopePublic),
		network.NewScopedAddress("10.10.0.30", network.ScopePublic),
		network.NewScopedAddress("10.20.0.20", network.ScopeCloudLocal))

	res := s.machine.GetNetworkInfoForSpaces(set.NewStrings("default", "dmz", "doesnotexists", ""))
	c.Check(res, gc.HasLen, 4)

	resDefault, ok := res["default"]
	c.Assert(ok, jc.IsTrue)
	c.Check(resDefault.Error, jc.ErrorIsNil)
	c.Assert(resDefault.NetworkInfos, gc.HasLen, 1)
	c.Check(resDefault.NetworkInfos[0].InterfaceName, gc.Equals, "br-eth0")
	c.Assert(resDefault.NetworkInfos[0].Addresses, gc.HasLen, 1)
	c.Check(resDefault.NetworkInfos[0].Addresses[0].Address, gc.Equals, "10.0.0.20")
	c.Check(resDefault.NetworkInfos[0].Addresses[0].CIDR, gc.Equals, "10.0.0.0/24")

	resDMZ, ok := res["dmz"]
	c.Assert(ok, jc.IsTrue)
	c.Check(resDMZ.Error, jc.ErrorIsNil)
	c.Assert(resDMZ.NetworkInfos, gc.HasLen, 1)
	c.Check(resDMZ.NetworkInfos[0].InterfaceName, gc.Equals, "eth1")
	c.Assert(resDMZ.NetworkInfos[0].Addresses, gc.HasLen, 1)
	c.Check(resDMZ.NetworkInfos[0].Addresses[0].Address, gc.Equals, "10.10.0.20")
	c.Check(resDMZ.NetworkInfos[0].Addresses[0].CIDR, gc.Equals, "10.10.0.0/24")

	resEmpty, ok := res[""]
	c.Assert(ok, jc.IsTrue)
	c.Check(resEmpty.Error, jc.ErrorIsNil)
	c.Assert(resEmpty.NetworkInfos, gc.HasLen, 1)
	c.Check(resEmpty.NetworkInfos[0].InterfaceName, gc.Equals, "eth2")
	c.Assert(resEmpty.NetworkInfos[0].Addresses, gc.HasLen, 1)
	c.Check(resEmpty.NetworkInfos[0].Addresses[0].Address, gc.Equals, "10.20.0.20")
	c.Check(resEmpty.NetworkInfos[0].Addresses[0].CIDR, gc.Equals, "10.20.0.0/24")

	resDoesNotExists, ok := res["doesnotexists"]
	c.Assert(ok, jc.IsTrue)
	c.Check(resDoesNotExists.Error, gc.ErrorMatches, `.*machine "0" has no devices in space "doesnotexists".*`)
	c.Assert(resDoesNotExists.NetworkInfos, gc.HasLen, 0)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDevicesForSpacesNoSuchSpace(c *gc.C) {
	s.setupTwoSpaces(c)
	// Is put into the 'default' space
	s.createNICAndBridgeWithIP(c, s.machine, "eth0", "br-eth0", "10.0.0.20/24")
	res, err := s.machine.LinkLayerDevicesForSpaces([]string{"dmz"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 0)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDevicesForSpacesNoBridge(c *gc.C) {
	s.setupMachineWithOneNIC(c)
	res, err := s.machine.LinkLayerDevicesForSpaces([]string{"default"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 1)
	devices, ok := res["default"]
	c.Check(ok, jc.IsTrue)
	c.Check(devices, gc.HasLen, 1)
	c.Check(devices[0].Name(), gc.Equals, "eth0")
	c.Check(devices[0].Type(), gc.Equals, state.EthernetDevice)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDevicesForSpacesMultipleSpaces(c *gc.C) {
	s.setupTwoSpaces(c)
	// Is put into the 'default' space
	s.createNICAndBridgeWithIP(c, s.machine, "eth0", "br-eth0", "10.0.0.20/24")
	// Now add a NIC in the dmz space, but without a bridge
	s.createNICWithIP(c, s.machine, "eth1", "10.10.0.20/24")
	res, err := s.machine.LinkLayerDevicesForSpaces([]string{"default", "dmz"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 2)
	defaultDevices, ok := res["default"]
	c.Check(ok, jc.IsTrue)
	c.Check(defaultDevices, gc.HasLen, 1)
	c.Check(defaultDevices[0].Name(), gc.Equals, "br-eth0")
	dmzDevices, ok := res["dmz"]
	c.Check(ok, jc.IsTrue)
	c.Check(dmzDevices, gc.HasLen, 1)
	c.Check(dmzDevices[0].Name(), gc.Equals, "eth1")
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDevicesForSpacesWithExtraAddresses(c *gc.C) {
	s.setupTwoSpaces(c)
	// Is put into the 'default' space
	s.createNICAndBridgeWithIP(c, s.machine, "eth0", "br-eth0", "10.0.0.20/24")
	// When we poll the machine, we include any IP addresses that we
	// find. One of them is always the loopback, but we could find any
	// other addresses that someone created on the machine that we
	// don't know what they are.
	s.createNICWithIP(c, s.machine, "lo", "127.0.0.1/24")
	s.createNICWithIP(c, s.machine, "ens5", "172.99.0.24/24")
	res, err := s.machine.LinkLayerDevicesForSpaces([]string{"default"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 1)
	defaultDevices, ok := res["default"]
	c.Check(ok, jc.IsTrue)
	c.Check(defaultDevices, gc.HasLen, 1)
	c.Check(defaultDevices[0].Name(), gc.Equals, "br-eth0")
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDevicesForSpacesSortOrder(c *gc.C) {
	s.setupTwoSpaces(c)
	// Is put into the 'default' space
	s.createNICAndBridgeWithIP(c, s.machine, "eth0", "br-eth0", "10.0.0.20/24")
	// Add more devices to the "default" space, to make sure the result comes
	// back in NaturallySorted order
	devices := []state.LinkLayerDeviceArgs{{
		Name:       "eth1",
		Type:       state.EthernetDevice,
		MACAddress: "11:23:45:67:89:ab:cd:ef",
		ParentName: "br-eth1",
		IsUp:       true,
	}, {
		Name:       "br-eth1",
		Type:       state.BridgeDevice,
		MACAddress: "11:23:45:67:89:ab:cd:ef",
		IsUp:       true,
	}, {
		Name:       "eth1.1",
		Type:       state.EthernetDevice,
		MACAddress: "22:23:45:67:89:ab:cd:ef",
		ParentName: "br-eth1.1",
		IsUp:       true,
	}, {
		Name:       "br-eth1.1",
		Type:       state.BridgeDevice,
		MACAddress: "22:23:45:67:89:ab:cd:ef",
		IsUp:       true,
	}, {
		Name:       "eth1:1",
		Type:       state.EthernetDevice,
		MACAddress: "32:23:45:67:89:ab:cd:ef",
		ParentName: "br-eth1:1",
		IsUp:       true,
	}, {
		Name:       "br-eth1:1",
		Type:       state.BridgeDevice,
		MACAddress: "32:23:45:67:89:ab:cd:ef",
		IsUp:       true,
	}, {
		Name:       "eth10",
		Type:       state.EthernetDevice,
		MACAddress: "42:23:45:67:89:ab:cd:ef",
		ParentName: "br-eth10",
		IsUp:       true,
	}, {
		Name:       "br-eth10",
		Type:       state.BridgeDevice,
		MACAddress: "42:23:45:67:89:ab:cd:ef",
		IsUp:       true,
	}, {
		Name:       "eth10.2",
		Type:       state.EthernetDevice,
		MACAddress: "52:23:45:67:89:ab:cd:ef",
		ParentName: "br-eth10.2",
		IsUp:       true,
	}, {
		Name:       "br-eth10.2",
		Type:       state.BridgeDevice,
		MACAddress: "52:23:45:67:89:ab:cd:ef",
		IsUp:       true,
	}, {
		Name:       "eth2",
		Type:       state.EthernetDevice,
		MACAddress: "61:23:45:67:89:ab:cd:ef",
		IsUp:       true,
	}, {
		Name:       "eth20",
		Type:       state.EthernetDevice,
		MACAddress: "61:23:45:67:89:ab:cd:ef",
		IsUp:       true,
	}, {
		Name:       "eth3",
		Type:       state.EthernetDevice,
		MACAddress: "61:23:45:67:89:ab:cd:ef",
		IsUp:       true,
	}}
	err := s.machine.SetParentLinkLayerDevicesBeforeTheirChildren(devices)
	c.Assert(err, jc.ErrorIsNil)
	addresses := make([]state.LinkLayerDeviceAddress, 0, len(devices))
	for i, device := range devices {
		if device.ParentName != "" {
			// Devices *on* the bridge do not end up with IP addresses, only
			// the Bridge device has an address.
			continue
		}
		addresses = append(addresses, state.LinkLayerDeviceAddress{
			DeviceName:   device.Name,
			CIDRAddress:  fmt.Sprintf("10.0.0.1%02d/24", i),
			ConfigMethod: state.StaticAddress,
		})
	}
	err = s.machine.SetDevicesAddresses(addresses...)
	c.Assert(err, jc.ErrorIsNil)
	res, err := s.machine.LinkLayerDevicesForSpaces([]string{"default"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res, gc.HasLen, 1)
	defaultDevices, ok := res["default"]
	c.Check(ok, jc.IsTrue)
	names := make([]string, 0, len(defaultDevices))
	for _, dev := range defaultDevices {
		names = append(names, dev.Name())
	}
	c.Check(names, gc.DeepEquals, []string{
		"br-eth0", "br-eth1", "br-eth1.1", "br-eth1:1", "br-eth10", "br-eth10.2",
		"eth2", "eth3", "eth20",
	})
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDevicesForSpacesInUnknownSpace(c *gc.C) {
	// We explicitly ask for the devices that are not in a known space.
	s.setupTwoSpaces(c)
	s.createNICWithIP(c, s.machine, "ens4", "172.99.0.24/24")
	s.createNICWithIP(c, s.machine, "ens5", "192.168.10.12/24")
	res, err := s.machine.LinkLayerDevicesForSpaces([]string{""})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)
	devices, ok := res[""]
	c.Assert(ok, jc.IsTrue)
	c.Assert(devices, gc.HasLen, 2)
	c.Check(devices[0].Name(), gc.Equals, "ens4")
	c.Check(devices[0].Type(), gc.Equals, state.EthernetDevice)
	c.Check(devices[1].Name(), gc.Equals, "ens5")
	c.Check(devices[1].Type(), gc.Equals, state.EthernetDevice)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDevicesForSpacesWithUnknown(c *gc.C) {
	// We ask for devices for which we don't know their space, as well as
	// ones that we do.
	s.setupTwoSpaces(c)
	// default space, bridged
	s.createNICAndBridgeWithIP(c, s.machine, "ens4", "br-ens4", "10.0.0.21/24")
	// unknown space
	s.createNICWithIP(c, s.machine, "ens5", "192.168.10.12/24")
	// loopback device
	s.createLoopbackNIC(c, s.machine)
	res, err := s.machine.LinkLayerDevicesForSpaces([]string{"", "default"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 2)
	devices, ok := res[""]
	c.Assert(ok, jc.IsTrue)
	c.Assert(devices, gc.HasLen, 1)
	c.Check(devices[0].Name(), gc.Equals, "ens5")
	c.Check(devices[0].Type(), gc.Equals, state.EthernetDevice)
	devices, ok = res["default"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(devices, gc.HasLen, 1)
	c.Check(devices[0].Name(), gc.Equals, "br-ens4")
	c.Check(devices[0].Type(), gc.Equals, state.BridgeDevice)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDevicesForSpacesWithNoAddress(c *gc.C) {
	// We create a record for the 'lxdbr0' bridge, but it doesn't have an
	// address yet (which is the case when we first show up on a machine.)
	err := s.machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       "lxdbr0",
			Type:       state.BridgeDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	// unknown space
	s.createNICWithIP(c, s.machine, "ens5", "192.168.10.12/24")
	res, err := s.machine.LinkLayerDevicesForSpaces([]string{""})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)
	devices, ok := res[""]
	c.Assert(ok, jc.IsTrue)
	c.Assert(devices, gc.HasLen, 2)
	names := make([]string, len(devices))
	for i, dev := range devices {
		names[i] = dev.Name()
	}
	c.Check(names, gc.DeepEquals, []string{"ens5", "lxdbr0"})
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDevicesForSpacesUnknownIgnoresLoopAndIncludesKnownBridges(c *gc.C) {
	// TODO(jam): 2016-12-28 arguably we should also be aware of Docker
	// devices, possibly the better plan is to look at whether there are
	// routes from the given bridge out into the rest of the world.
	s.setupTwoSpaces(c)
	// Unknown device, bridge, loopback, lxcbr0, lxdbr0 and virbr0
	s.createNICWithIP(c, s.machine, "ens3", "10.99.0.10/24")
	s.createNICAndBridgeWithIP(c, s.machine, "ens4", "br-ens4", "10.100.0.21/24")
	s.createAllDefaultDevices(c, s.machine)
	res, err := s.machine.LinkLayerDevicesForSpaces([]string{""})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 1)
	devices, ok := res[""]
	c.Assert(ok, jc.IsTrue)
	names := make([]string, len(devices))
	for i, dev := range devices {
		names[i] = dev.Name()
	}
	c.Check(names, gc.DeepEquals, []string{"br-ens4", "ens3", "lxcbr0", "lxdbr0", "virbr0"})
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
		Type:        state.BondDevice,
		MTU:         1234,
		MACAddress:  "aa:bb:cc:dd:ee:f0",
		IsAutoStart: true,
		IsUp:        true,
	}, {
		Name:        children[0].Name(),
		Type:        state.VLAN_8021QDevice,
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
		Type:       state.EthernetDevice,
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
	defer state.SetTestHooks(c, s.State, churnHook, churnHook, churnHook).Check()
	s.assertAllLinkLayerDevicesOnMachineMatchCount(c, s.machine, 1) // parent only

	err := s.machine.SetLinkLayerDevices(childArgs)
	c.Assert(errors.Cause(err), gc.Equals, jujutxn.ErrExcessiveContention)
	s.assertAllLinkLayerDevicesOnMachineMatchCount(c, s.machine, 1) // only the parent remains
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesRefusesToAddContainerChildDeviceWithNonBridgeParent(c *gc.C) {
	// Add one device of every type to the host machine, except a BridgeDevice.
	hostDevicesArgs := []state.LinkLayerDeviceArgs{{
		Name: "loopback",
		Type: state.LoopbackDevice,
	}, {
		Name: "ethernet",
		Type: state.EthernetDevice,
	}, {
		Name: "vlan",
		Type: state.VLAN_8021QDevice,
	}, {
		Name: "bond",
		Type: state.BondDevice,
	}}
	hostDevices := s.setMultipleDevicesSucceedsAndCheckAllAdded(c, hostDevicesArgs)
	hostMachineParentDeviceGlobalKeyPrefix := "m#0#d#"
	s.addContainerMachine(c)

	// Now try setting an EthernetDevice on the container specifying each of the
	// hostDevices as parent and expect none of them to succeed, as none of the
	// hostDevices is a BridgeDevice.
	for _, hostDevice := range hostDevices {
		parentDeviceGlobalKey := hostMachineParentDeviceGlobalKeyPrefix + hostDevice.Name()
		containerDeviceArgs := state.LinkLayerDeviceArgs{
			Name:       "eth0",
			Type:       state.EthernetDevice,
			ParentName: parentDeviceGlobalKey,
		}
		err := s.containerMachine.SetLinkLayerDevices(containerDeviceArgs)
		expectedError := `cannot set .* to machine "0/lxd/0": ` +
			`invalid device "eth0": ` +
			`parent device ".*" on host machine "0" must be of type "bridge", not type ".*"`
		c.Check(err, gc.ErrorMatches, expectedError)
		c.Check(err, jc.Satisfies, errors.IsNotValid)
	}
	s.assertNoDevicesOnMachine(c, s.containerMachine)
}

func (s *linkLayerDevicesStateSuite) addContainerMachine(c *gc.C) {
	// Add a container machine with s.machine as its host.
	containerTemplate := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
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
	s.addParentBridgeDeviceWithContainerDevicesAsChildren(c, network.DefaultLXCBridge, "vethZ", 1)
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
		Type: state.BridgeDevice,
	}
	parentDevice = s.assertSetLinkLayerDevicesSucceedsAndResultMatchesArgs(c, parentArgs)
	parentDeviceGlobalKey := "m#" + s.machine.Id() + "#d#" + parentName

	childrenArgsTemplate := state.LinkLayerDeviceArgs{
		Type:       state.EthernetDevice,
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
		Type:       state.EthernetDevice,
		ParentName: children[0].ParentName(),
	}
	err := s.containerMachine.SetLinkLayerDevices(newChildArgs)
	c.Assert(err, gc.ErrorMatches, expectedError)
}

func (s *linkLayerDevicesStateSuite) TestSetLinkLayerDevicesToContainerWhenContainerAndHostRemovedBeforehand(c *gc.C) {
	beforeHook := func() {
		// Remove both container and host machines.
		err := s.containerMachine.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		err = s.containerMachine.Remove()
		c.Assert(err, jc.ErrorIsNil)
		err = s.machine.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		err = s.machine.Remove()
		c.Assert(err, jc.ErrorIsNil)
	}

	s.assertSetLinkLayerDevicesToContainerFailsWithBeforeHook(c, beforeHook,
		`.*host machine "0" of parent device "br-eth1" not found or not alive`,
	)
}

func (s *linkLayerDevicesStateSuite) TestMachineRemoveAlsoRemoveAllLinkLayerDevices(c *gc.C) {
	s.assertNoDevicesOnMachine(c, s.machine)
	s.addNamedParentDeviceWithChildrenAndCheckAllAdded(c, "foo", "bar")

	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertNoDevicesOnMachine(c, s.machine)
}

func (s *linkLayerDevicesStateSuite) TestMachineSetParentLinkLayerDevicesBeforeTheirChildrenUnchangedProviderIDsOK(c *gc.C) {
	s.testMachineSetParentLinkLayerDevicesBeforeTheirChildren(c)
}

func (s *linkLayerDevicesStateSuite) TestMachineSetParentLinkLayerDevicesBeforeTheirChildrenIdempotent(c *gc.C) {
	s.testMachineSetParentLinkLayerDevicesBeforeTheirChildren(c)
	s.testMachineSetParentLinkLayerDevicesBeforeTheirChildren(c)
}

var nestedDevicesArgs = []state.LinkLayerDeviceArgs{{
	Name: "lo",
	Type: state.LoopbackDevice,
}, {
	Name: "br-bond0",
	Type: state.BridgeDevice,
}, {
	Name:       "br-bond0.12",
	Type:       state.BridgeDevice,
	ParentName: "br-bond0",
}, {
	Name:       "br-bond0.34",
	Type:       state.BridgeDevice,
	ParentName: "br-bond0",
}, {
	Name:       "bond0",
	Type:       state.BondDevice,
	ParentName: "br-bond0",
	ProviderID: "100",
}, {
	Name:       "bond0.12",
	Type:       state.VLAN_8021QDevice,
	ParentName: "bond0",
	ProviderID: "101",
}, {
	Name:       "bond0.34",
	Type:       state.VLAN_8021QDevice,
	ParentName: "bond0",
	ProviderID: "102",
}, {
	Name:       "eth0",
	Type:       state.EthernetDevice,
	ParentName: "bond0",
	ProviderID: "103",
}, {
	Name:       "eth1",
	Type:       state.EthernetDevice,
	ParentName: "bond0",
	ProviderID: "104",
}}

func (s *linkLayerDevicesStateSuite) testMachineSetParentLinkLayerDevicesBeforeTheirChildren(c *gc.C) {
	err := s.machine.SetParentLinkLayerDevicesBeforeTheirChildren(nestedDevicesArgs)
	c.Assert(err, jc.ErrorIsNil)
	allDevices, err := s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allDevices, gc.HasLen, len(nestedDevicesArgs))
	for _, device := range allDevices {
		if device.Type() != state.LoopbackDevice && device.Type() != state.BridgeDevice {
			c.Check(device.ProviderID(), gc.Not(gc.Equals), corenetwork.Id(""))
		}
	}
}
