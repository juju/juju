// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sync"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

// linkLayerDevicesStateSuite contains white-box tests for link-layer network
// devices, which include access to mongo.
type linkLayerDevicesStateSuite struct {
	ConnSuite

	machine *state.Machine

	otherState        *state.State
	otherStateMachine *state.Machine
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
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesNoArgs(c *gc.C) {
	err := s.machine.AddLinkLayerDevices() // takes varargs, which includes none.
	expectedError := fmt.Sprintf("cannot add link-layer devices to machine %q: no devices to add", s.machine.Id())
	c.Assert(err, gc.ErrorMatches, expectedError)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesEmptyArgs(c *gc.C) {
	args := state.LinkLayerDeviceArgs{}
	s.assertAddLinkLayerDevicesReturnsNotValidError(c, args, "empty Name not valid")
}

func (s *linkLayerDevicesStateSuite) assertAddLinkLayerDevicesReturnsNotValidError(c *gc.C, args state.LinkLayerDeviceArgs, errorCauseMatches string) {
	err := s.assertAddLinkLayerDevicesFailsValidationForArgs(c, args, errorCauseMatches)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *linkLayerDevicesStateSuite) assertAddLinkLayerDevicesFailsValidationForArgs(c *gc.C, args state.LinkLayerDeviceArgs, errorCauseMatches string) error {
	expectedError := fmt.Sprintf("invalid device %q: %s", args.Name, errorCauseMatches)
	return s.assertAddLinkLayerDevicesFailsForArgs(c, args, expectedError)
}

func (s *linkLayerDevicesStateSuite) assertAddLinkLayerDevicesFailsForArgs(c *gc.C, args state.LinkLayerDeviceArgs, errorCauseMatches string) error {
	err := s.machine.AddLinkLayerDevices(args)
	expectedError := fmt.Sprintf("cannot add link-layer devices to machine %q: %s", s.machine.Id(), errorCauseMatches)
	c.Assert(err, gc.ErrorMatches, expectedError)
	return err
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesInvalidName(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name: "bad#name",
	}
	s.assertAddLinkLayerDevicesReturnsNotValidError(c, args, `Name "bad#name" not valid`)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesSameNameAndParentName(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "foo",
		ParentName: "foo",
	}
	s.assertAddLinkLayerDevicesReturnsNotValidError(c, args, `Name and ParentName must be different`)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesInvalidType(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name: "bar",
		Type: "bad type",
	}
	s.assertAddLinkLayerDevicesReturnsNotValidError(c, args, `Type "bad type" not valid`)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesInvalidParentName(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "eth0",
		ParentName: "bad#name",
	}
	s.assertAddLinkLayerDevicesReturnsNotValidError(c, args, `ParentName "bad#name" not valid`)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesInvalidMACAddress(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "eth0",
		Type:       state.EthernetDevice,
		MACAddress: "bad mac",
	}
	s.assertAddLinkLayerDevicesReturnsNotValidError(c, args, `MACAddress "bad mac" not valid`)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesWhenMachineNotAliveOrGone(c *gc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	args := state.LinkLayerDeviceArgs{
		Name: "eth0",
		Type: state.EthernetDevice,
	}
	s.assertAddLinkLayerDevicesFailsForArgs(c, args, "machine not found or not alive")

	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertAddLinkLayerDevicesFailsForArgs(c, args, "machine not found or not alive")
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesWhenModelNotAlive(c *gc.C) {
	otherModel, err := s.otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = otherModel.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	args := state.LinkLayerDeviceArgs{
		Name: "eth0",
		Type: state.EthernetDevice,
	}
	err = s.otherStateMachine.AddLinkLayerDevices(args)
	expectedError := fmt.Sprintf(
		"cannot add link-layer devices to machine %q: model %q is no longer alive",
		s.otherStateMachine.Id(), otherModel.Name(),
	)
	c.Assert(err, gc.ErrorMatches, expectedError)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesWithMissingParent(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "eth0",
		Type:       state.EthernetDevice,
		ParentName: "br-eth0",
	}
	err := s.assertAddLinkLayerDevicesFailsForArgs(c, args, `parent device "br-eth0" of device "eth0" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesNoParentSuccess(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:        "eth0.42",
		MTU:         9000,
		ProviderID:  "eni-42",
		Type:        state.VLAN_8021QDevice,
		MACAddress:  "aa:bb:cc:dd:ee:f0",
		IsAutoStart: true,
		IsUp:        true,
	}
	s.assertAddLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)
}

func (s *linkLayerDevicesStateSuite) assertAddLinkLayerDevicesSucceedsAndResultMatchesArgs(c *gc.C, args state.LinkLayerDeviceArgs) {
	s.assertMachineAddLinkLayerDevicesSucceedsAndResultMatchesArgs(c, s.machine, args, s.State.ModelUUID())
}

func (s *linkLayerDevicesStateSuite) assertMachineAddLinkLayerDevicesSucceedsAndResultMatchesArgs(c *gc.C, machine *state.Machine, args state.LinkLayerDeviceArgs, modelUUID string) {
	err := machine.AddLinkLayerDevices(args)
	c.Assert(err, jc.ErrorIsNil)
	result, err := machine.LinkLayerDevice(args.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)

	s.checkAddedDeviceMatchesArgs(c, result, args)
	s.checkAddedDeviceMatchesMachineIDAndModelUUID(c, result, s.machine.Id(), modelUUID)
}

func (s *linkLayerDevicesStateSuite) checkAddedDeviceMatchesArgs(c *gc.C, addedDevice *state.LinkLayerDevice, args state.LinkLayerDeviceArgs) {
	c.Check(addedDevice.Name(), gc.Equals, args.Name)
	c.Check(addedDevice.MTU(), gc.Equals, args.MTU)
	c.Check(addedDevice.ProviderID(), gc.Equals, args.ProviderID)
	c.Check(addedDevice.Type(), gc.Equals, args.Type)
	c.Check(addedDevice.MACAddress(), gc.Equals, args.MACAddress)
	c.Check(addedDevice.IsAutoStart(), gc.Equals, args.IsAutoStart)
	c.Check(addedDevice.IsUp(), gc.Equals, args.IsUp)
	c.Check(addedDevice.ParentName(), gc.Equals, args.ParentName)
}

func (s *linkLayerDevicesStateSuite) checkAddedDeviceMatchesMachineIDAndModelUUID(c *gc.C, addedDevice *state.LinkLayerDevice, machineID, modelUUID string) {
	globalKey := fmt.Sprintf("m#%s#d#%s", machineID, addedDevice.Name())
	c.Check(addedDevice.DocID(), gc.Equals, modelUUID+":"+globalKey)
	c.Check(addedDevice.MachineID(), gc.Equals, machineID)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesNoProviderIDSuccess(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name: "eno0",
		Type: state.EthernetDevice,
	}
	s.assertAddLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesWithDuplicateProviderIDFailsInSameModel(c *gc.C) {
	args1 := state.LinkLayerDeviceArgs{
		Name:       "eth0.42",
		Type:       state.EthernetDevice,
		ProviderID: "42",
	}
	s.assertAddLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args1)

	args2 := args1
	args2.Name = "br-eth0"
	err := s.assertAddLinkLayerDevicesFailsValidationForArgs(c, args2, `ProviderID\(s\) not unique: 42`)
	c.Assert(err, jc.Satisfies, state.IsProviderIDNotUniqueError)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesWithDuplicateNameAndProviderIDSucceedsInDifferentModels(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "eth0.42",
		Type:       state.EthernetDevice,
		ProviderID: "42",
	}
	s.assertAddLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)

	s.assertMachineAddLinkLayerDevicesSucceedsAndResultMatchesArgs(c, s.otherStateMachine, args, s.otherState.ModelUUID())
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesWithDuplicateNameAndEmptyProviderIDReturnsAlreadyExistsErrorInSameModel(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name: "eth0.42",
		Type: state.EthernetDevice,
	}
	s.assertAddLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)

	err := s.assertAddLinkLayerDevicesFailsForArgs(c, args, `device "eth0.42" already exists`)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesWithDuplicateNameAndProviderIDFailsInSameModel(c *gc.C) {
	args := state.LinkLayerDeviceArgs{
		Name:       "foo",
		Type:       state.EthernetDevice,
		ProviderID: "42",
	}
	s.assertAddLinkLayerDevicesSucceedsAndResultMatchesArgs(c, args)

	err := s.assertAddLinkLayerDevicesFailsValidationForArgs(c, args, `ProviderID\(s\) not unique: 42`)
	c.Assert(err, jc.Satisfies, state.IsProviderIDNotUniqueError)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesMultipleArgsWithSameNameFails(c *gc.C) {
	foo1 := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: state.BridgeDevice,
	}
	foo2 := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: state.EthernetDevice,
	}
	err := s.machine.AddLinkLayerDevices(foo1, foo2)
	c.Assert(err, gc.ErrorMatches, `.*invalid device "foo": Name specified more than once`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesMultipleArgsChildParentOrderDoesNotMatter(c *gc.C) {
	allArgs := []state.LinkLayerDeviceArgs{{
		Name:       "child1",
		Type:       state.EthernetDevice,
		ParentName: "parent1",
	}, {
		Name: "parent1",
		Type: state.BridgeDevice,
	}, {
		Name: "parent2",
		Type: state.BondDevice,
	}, {
		Name:       "child2",
		Type:       state.VLAN_8021QDevice,
		ParentName: "parent2",
	}}

	s.addLinkLayerDevicesMultipleArgsSucceedsAndEnsureAllAdded(c, allArgs)
}

func (s *linkLayerDevicesStateSuite) addLinkLayerDevicesMultipleArgsSucceedsAndEnsureAllAdded(c *gc.C, allArgs []state.LinkLayerDeviceArgs) {
	err := s.machine.AddLinkLayerDevices(allArgs...)
	c.Assert(err, jc.ErrorIsNil)

	machineID, modelUUID := s.machine.Id(), s.State.ModelUUID()
	for _, args := range allArgs {
		device, err := s.machine.LinkLayerDevice(args.Name)
		c.Check(err, jc.ErrorIsNil)
		s.checkAddedDeviceMatchesArgs(c, device, args)
		s.checkAddedDeviceMatchesMachineIDAndModelUUID(c, device, machineID, modelUUID)
	}
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesMultipleChildrenOfExistingParentSucceeds(c *gc.C) {
	parent := s.addSimpleDevice(c)
	childrenArgs := []state.LinkLayerDeviceArgs{{
		Name:       "child1",
		Type:       state.EthernetDevice,
		ParentName: parent.Name(),
	}, {
		Name:       "child2",
		Type:       state.EthernetDevice,
		ParentName: parent.Name(),
	}}

	s.addLinkLayerDevicesMultipleArgsSucceedsAndEnsureAllAdded(c, childrenArgs)
}

func (s *linkLayerDevicesStateSuite) addSimpleDevice(c *gc.C) *state.LinkLayerDevice {
	args := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: state.EthernetDevice,
	}
	err := s.machine.AddLinkLayerDevices(args)
	c.Assert(err, jc.ErrorIsNil)
	device, err := s.machine.LinkLayerDevice(args.Name)
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
	args := []state.LinkLayerDeviceArgs{{
		Name: "br-eth0",
		Type: state.BridgeDevice,
	}, {
		Name:       "eth0",
		Type:       state.EthernetDevice,
		ParentName: "br-eth0",
	}}
	s.addLinkLayerDevicesMultipleArgsSucceedsAndEnsureAllAdded(c, args)

	child, err := s.machine.LinkLayerDevice("eth0")
	c.Assert(err, jc.ErrorIsNil)
	parent, err := child.ParentDevice()
	c.Assert(err, jc.ErrorIsNil)
	s.checkAddedDeviceMatchesArgs(c, parent, args[0])
	s.checkAddedDeviceMatchesMachineIDAndModelUUID(c, parent, s.machine.Id(), s.State.ModelUUID())
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

	args := []state.LinkLayerDeviceArgs{{
		Name: "br-bond0",
		Type: state.BridgeDevice,
	}, {
		Name:       "bond0",
		Type:       state.BondDevice,
		ParentName: "br-bond0",
	}, {
		Name:       "eth0",
		Type:       state.EthernetDevice,
		ParentName: "bond0",
	}, {
		Name:       "eth1",
		Type:       state.EthernetDevice,
		ParentName: "bond0",
	}}
	s.addLinkLayerDevicesMultipleArgsSucceedsAndEnsureAllAdded(c, args)

	results, err := s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 4)
	for _, result := range results {
		c.Check(result, gc.NotNil)
		c.Check(result.MachineID(), gc.Equals, s.machine.Id())
		c.Check(result.Name(), gc.Matches, `(br-bond0|bond0|eth0|eth1)`)
		if result.Name() == "br-bond0" {
			c.Check(result.ParentName(), gc.Equals, "")
			continue
		}
		c.Check(result.ParentName(), gc.Matches, `(br-bond0|bond0)`)
	}
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

	args := []state.LinkLayerDeviceArgs{{
		Name: "foo",
		Type: state.EthernetDevice,
	}, {
		Name:       "foo.42",
		Type:       state.VLAN_8021QDevice,
		ParentName: "foo",
	}}
	s.addLinkLayerDevicesMultipleArgsSucceedsAndEnsureAllAdded(c, args)

	results, err := s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results[0].Name(), gc.Equals, "foo")
	c.Assert(results[1].Name(), gc.Equals, "foo.42")

	s.assertNoDevicesOnMachine(c, s.otherStateMachine)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDeviceRemoveFailsWithExistingChildren(c *gc.C) {
	args := []state.LinkLayerDeviceArgs{{
		Name: "parent",
		Type: state.BridgeDevice,
	}, {
		Name:       "one-child",
		Type:       state.EthernetDevice,
		ParentName: "parent",
	}, {
		Name:       "another-child",
		Type:       state.EthernetDevice,
		ParentName: "parent",
	}}
	s.addLinkLayerDevicesMultipleArgsSucceedsAndEnsureAllAdded(c, args)

	parent, err := s.machine.LinkLayerDevice("parent")
	c.Assert(err, jc.ErrorIsNil)

	err = parent.Remove()
	expectedError := fmt.Sprintf(
		"cannot remove %s: parent device %q has children: another-child, one-child",
		parent, parent.Name(),
	)
	c.Assert(err, gc.ErrorMatches, expectedError)
	c.Assert(err, jc.Satisfies, state.IsParentDeviceHasChildrenError)
}

func (s *linkLayerDevicesStateSuite) TestLinkLayerDeviceRemoveSuccess(c *gc.C) {
	existingDevice := s.addSimpleDevice(c)

	s.removeDeviceAndAssertSuccess(c, existingDevice)
	s.assertNoDevicesOnMachine(c, s.machine)
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

	args := []state.LinkLayerDeviceArgs{{
		Name: "foo",
		Type: state.EthernetDevice,
	}, {
		Name:       "bar",
		Type:       state.VLAN_8021QDevice,
		ParentName: "foo",
	}}
	s.addLinkLayerDevicesMultipleArgsSucceedsAndEnsureAllAdded(c, args)

	err := s.machine.RemoveAllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoDevicesOnMachine(c, s.machine)
}

func (s *linkLayerDevicesStateSuite) TestMachineRemoveAllLinkLayerDevicesNoErrorIfNoDevicesExist(c *gc.C) {
	s.assertNoDevicesOnMachine(c, s.machine)

	err := s.machine.RemoveAllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesRollbackWithDuplicateProviderIDs(c *gc.C) {
	insertingArgs := []state.LinkLayerDeviceArgs{{
		Name:       "child",
		Type:       state.EthernetDevice,
		ProviderID: "child-id",
		ParentName: "parent",
	}, {
		Name:       "parent",
		Type:       state.BridgeDevice,
		ProviderID: "parent-id",
	}}

	assertTwoExistAndRemoveAll := func() {
		s.assertAllLinkLayerDevicesOnMachineMatchCount(c, s.machine, 2)
		err := s.machine.RemoveAllLinkLayerDevices()
		c.Assert(err, jc.ErrorIsNil)
	}

	hooks := []jujutxn.TestHook{{
		Before: func() {
			// Add the same devices to trigger ErrAborted in the first attempt.
			s.assertNoDevicesOnMachine(c, s.machine)
			err := s.machine.AddLinkLayerDevices(insertingArgs...)
			c.Assert(err, jc.ErrorIsNil)
		},
		After: assertTwoExistAndRemoveAll,
	}, {
		Before: func() {
			// Add devices with same ProviderIDs but different names.
			s.assertNoDevicesOnMachine(c, s.machine)
			insertingAlternateArgs := insertingArgs
			insertingAlternateArgs[0].Name = "other-child"
			insertingAlternateArgs[0].ParentName = "other-parent"
			insertingAlternateArgs[1].Name = "other-parent"
			err := s.machine.AddLinkLayerDevices(insertingAlternateArgs...)
			c.Assert(err, jc.ErrorIsNil)
		},
		After: assertTwoExistAndRemoveAll,
	}}
	defer state.SetTestHooks(c, s.State, hooks...).Check()

	err := s.machine.AddLinkLayerDevices(insertingArgs...)
	c.Assert(err, gc.ErrorMatches, `.*ProviderID\(s\) not unique: child-id, parent-id`)
	c.Assert(err, jc.Satisfies, state.IsProviderIDNotUniqueError)
	s.assertNoDevicesOnMachine(c, s.machine) // Rollback worked.
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesWithLightStateChurn(c *gc.C) {
	allArgs, churnHook := s.prepareAddLinkLayerDevicesWithStateChurn(c)
	defer state.SetTestHooks(c, s.State, churnHook).Check()
	s.assertNoDevicesOnMachine(c, s.machine)

	s.addLinkLayerDevicesMultipleArgsSucceedsAndEnsureAllAdded(c, allArgs)
}

func (s *linkLayerDevicesStateSuite) prepareAddLinkLayerDevicesWithStateChurn(c *gc.C) ([]state.LinkLayerDeviceArgs, jujutxn.TestHook) {
	parentArgs := state.LinkLayerDeviceArgs{
		Name: "parent",
		Type: state.BridgeDevice,
	}
	childArgs := state.LinkLayerDeviceArgs{
		Name:       "child",
		Type:       state.EthernetDevice,
		ParentName: "parent",
	}

	churnHook := jujutxn.TestHook{
		Before: func() {
			s.assertNoDevicesOnMachine(c, s.machine)
			err := s.machine.AddLinkLayerDevices(parentArgs)
			c.Assert(err, jc.ErrorIsNil)
		},
		After: func() {
			s.assertAllLinkLayerDevicesOnMachineMatchCount(c, s.machine, 1)
			parent, err := s.machine.LinkLayerDevice("parent")
			c.Assert(err, jc.ErrorIsNil)
			err = parent.Remove()
			c.Assert(err, jc.ErrorIsNil)
		},
	}

	return []state.LinkLayerDeviceArgs{parentArgs, childArgs}, churnHook
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesWithModerateStateChurn(c *gc.C) {
	allArgs, churnHook := s.prepareAddLinkLayerDevicesWithStateChurn(c)
	defer state.SetTestHooks(c, s.State, churnHook, churnHook).Check()
	s.assertNoDevicesOnMachine(c, s.machine)

	s.addLinkLayerDevicesMultipleArgsSucceedsAndEnsureAllAdded(c, allArgs)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesWithTooMuchStateChurn(c *gc.C) {
	allArgs, churnHook := s.prepareAddLinkLayerDevicesWithStateChurn(c)
	defer state.SetTestHooks(c, s.State, churnHook, churnHook, churnHook).Check()
	s.assertNoDevicesOnMachine(c, s.machine)

	err := s.machine.AddLinkLayerDevices(allArgs...)
	c.Assert(errors.Cause(err), gc.Equals, jujutxn.ErrExcessiveContention)

	s.assertNoDevicesOnMachine(c, s.machine)
}

func (s *linkLayerDevicesStateSuite) TestAddLinkLayerDevicesWithHighConcurrency(c *gc.C) {
	// Tested successfully multiple times with:
	// $ cd $GOPATH/src/github.com/juju/juju/state; go test -c
	// $ for i in {1..100}; do ./state.test -check.v -check.f HighCon & done
	// The only observed issue (even with 500 vs 100 runs) is MgoSuite.SetUpTest()
	// panicking due to/ "address already in use" coming from the test mongod
	// instance, not the production code being tested below. And even then it
	// happens in 1 or 2 out of 100 (or even 500) test runs.
	parentArgs := state.LinkLayerDeviceArgs{
		Name: "parent",
		Type: state.BridgeDevice,
	}
	parentArgsWithID := parentArgs
	parentArgsWithID.ProviderID = "parent-id"
	childArgs := state.LinkLayerDeviceArgs{
		Name:       "child",
		Type:       state.EthernetDevice,
		ParentName: "parent",
	}
	childArgsWithID := childArgs
	childArgsWithID.ProviderID = "child-id"
	// Use a map to randomize iteration order.
	argsPermutations := map[string][]state.LinkLayerDeviceArgs{
		"parent-child-no-ids":        []state.LinkLayerDeviceArgs{parentArgs, childArgs},
		"child-parent-no-ids":        []state.LinkLayerDeviceArgs{childArgs, parentArgs},
		"child-parent-with-ids":      []state.LinkLayerDeviceArgs{childArgsWithID, parentArgsWithID},
		"parent-child-with-ids":      []state.LinkLayerDeviceArgs{parentArgsWithID, childArgsWithID},
		"parent-with-id-child-no-id": []state.LinkLayerDeviceArgs{parentArgsWithID, childArgs},
		"child-no-id-parent-with-id": []state.LinkLayerDeviceArgs{childArgs, parentArgsWithID},
		"child-with-id-parent-no-id": []state.LinkLayerDeviceArgs{childArgsWithID, parentArgs},
	}
	isAlreadyExistsOrProviderIDNotUniqueError := func(err error) bool {
		return err != nil && (errors.IsAlreadyExists(err) || state.IsProviderIDNotUniqueError(err))
	}

	var wg sync.WaitGroup
	wg.Add(len(argsPermutations))
	waitAllStarted := make(chan struct{})                       // sync all goroutines to start simultaneously.
	successfulArgs := make(chan []state.LinkLayerDeviceArgs, 1) // only 1 success expected.
	for about, args := range argsPermutations {
		go func(testAbout string, testArgs []state.LinkLayerDeviceArgs) {
			defer wg.Done()
			<-waitAllStarted

			err := s.machine.AddLinkLayerDevices(testArgs...)
			c.Logf("testing %q -> %v", testAbout, err)
			if err != nil {
				c.Assert(err, jc.Satisfies, isAlreadyExistsOrProviderIDNotUniqueError)
			} else {
				select {
				case successfulArgs <- testArgs:
				default:
					// successfulArgs is buffered, so if we can't send there was
					// more than on success.
					c.Fatalf("unexpected: more than one success for args %+v", testArgs)
				}
			}
		}(about, args)
	}
	close(waitAllStarted)
	wg.Wait()

	// Extract the successful parent and child args.
	addedArgs := <-successfulArgs
	c.Check(addedArgs, gc.HasLen, 2)
	var addedChildArgs, addedParentArgs state.LinkLayerDeviceArgs
	for _, args := range addedArgs {
		if args.ParentName == "" {
			addedParentArgs = args
		} else {
			addedChildArgs = args
		}
	}

	addedDevices, err := s.machine.AllLinkLayerDevices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addedDevices, gc.HasLen, 2)
	machineID, modelUUID := s.machine.Id(), s.State.ModelUUID()
	for _, device := range addedDevices {
		c.Check(device.Name(), gc.Matches, `(parent|child)`)
		if device.Name() == "child" {
			s.checkAddedDeviceMatchesArgs(c, device, addedChildArgs)
		} else {
			s.checkAddedDeviceMatchesArgs(c, device, addedParentArgs)
		}
		s.checkAddedDeviceMatchesMachineIDAndModelUUID(c, device, machineID, modelUUID)
	}
}
