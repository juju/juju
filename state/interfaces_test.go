// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

// interfacesStateSuite contains white-box tests for network interfaces, which
// include access to mongo.
type interfacesStateSuite struct {
	ConnSuite

	machine *state.Machine

	otherState        *state.State
	otherStateMachine *state.Machine
}

var _ = gc.Suite(&interfacesStateSuite{})

func (s *interfacesStateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.otherState = s.NewStateForModelNamed(c, "other-model")
	s.otherStateMachine, err = s.otherState.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *interfacesStateSuite) TestAddInterfacesNoArgs(c *gc.C) {
	err := s.machine.AddInterfaces()
	expectedError := fmt.Sprintf("cannot add interfaces to machine %q: no interfaces to add", s.machine.Id())
	c.Assert(err, gc.ErrorMatches, expectedError)
}

func (s *interfacesStateSuite) TestAddInterfacesEmptyArgs(c *gc.C) {
	args := state.InterfaceArgs{}
	s.assertAddInterfacesReturnsNotValidError(c, args, "empty Name not valid")
}

func (s *interfacesStateSuite) assertAddInterfacesReturnsNotValidError(c *gc.C, args state.InterfaceArgs, errorCauseMatches string) {
	err := s.assertAddInterfacesFailsValidationForArgs(c, args, errorCauseMatches)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *interfacesStateSuite) assertAddInterfacesFailsValidationForArgs(c *gc.C, args state.InterfaceArgs, errorCauseMatches string) error {
	expectedError := fmt.Sprintf("invalid interface %q: %s", args.Name, errorCauseMatches)
	return s.assertAddInterfacesFailsForArgs(c, args, expectedError)
}

func (s *interfacesStateSuite) assertAddInterfacesFailsForArgs(c *gc.C, args state.InterfaceArgs, errorCauseMatches string) error {
	err := s.machine.AddInterfaces(args)
	expectedError := fmt.Sprintf("cannot add interfaces to machine %q: %s", s.machine.Id(), errorCauseMatches)
	c.Assert(err, gc.ErrorMatches, expectedError)
	return err
}

func (s *interfacesStateSuite) TestAddInterfacesInvalidName(c *gc.C) {
	args := state.InterfaceArgs{
		Name: "bad#name",
	}
	s.assertAddInterfacesReturnsNotValidError(c, args, `Name "bad#name" not valid`)
}

func (s *interfacesStateSuite) TestAddInterfacesSameNameAndParentName(c *gc.C) {
	args := state.InterfaceArgs{
		Name:       "foo",
		ParentName: "foo",
	}
	s.assertAddInterfacesReturnsNotValidError(c, args, `Name and ParentName must be different`)
}

func (s *interfacesStateSuite) TestAddInterfacesInvalidType(c *gc.C) {
	args := state.InterfaceArgs{
		Name: "bar",
		Type: "bad type",
	}
	s.assertAddInterfacesReturnsNotValidError(c, args, `Type "bad type" not valid`)
}

func (s *interfacesStateSuite) TestAddInterfacesInvalidParentName(c *gc.C) {
	args := state.InterfaceArgs{
		Name:       "eth0",
		ParentName: "bad#name",
	}
	s.assertAddInterfacesReturnsNotValidError(c, args, `ParentName "bad#name" not valid`)
}

func (s *interfacesStateSuite) TestAddInterfacesInvalidHardwareAddress(c *gc.C) {
	args := state.InterfaceArgs{
		Name:            "eth0",
		Type:            state.EthernetInterface,
		HardwareAddress: "bad mac",
	}
	s.assertAddInterfacesReturnsNotValidError(c, args, `HardwareAddress "bad mac" not valid`)
}

func (s *interfacesStateSuite) TestAddInterfacesInvalidGatewayAddress(c *gc.C) {
	args := state.InterfaceArgs{
		Name:           "eth0",
		Type:           state.EthernetInterface,
		GatewayAddress: "bad ip",
	}
	s.assertAddInterfacesReturnsNotValidError(c, args, `GatewayAddress "bad ip" not valid`)
}

func (s *interfacesStateSuite) TestAddInterfacesWhenMachineNotAliveOrGone(c *gc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	args := state.InterfaceArgs{
		Name: "eth0",
		Type: state.EthernetInterface,
	}
	s.assertAddInterfacesFailsForArgs(c, args, "machine not found or not alive")

	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertAddInterfacesFailsForArgs(c, args, "machine not found or not alive")
}

func (s *interfacesStateSuite) TestAddInterfacesWhenModelNotAlive(c *gc.C) {
	otherModel, err := s.otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = otherModel.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	args := state.InterfaceArgs{
		Name: "eth0",
		Type: state.EthernetInterface,
	}
	err = s.otherStateMachine.AddInterfaces(args)
	expectedError := fmt.Sprintf(
		"cannot add interfaces to machine %q: model is no longer alive",
		s.otherStateMachine.Id(),
	)
	c.Assert(err, gc.ErrorMatches, expectedError)
}

func (s *interfacesStateSuite) TestAddInterfacesWithMissingParent(c *gc.C) {
	args := state.InterfaceArgs{
		Name:       "eth0",
		Type:       state.EthernetInterface,
		ParentName: "br-eth0",
	}
	err := s.assertAddInterfacesFailsForArgs(c, args, `parent interface "br-eth0" of interface "eth0" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *interfacesStateSuite) TestAddInterfacesNoParentSuccess(c *gc.C) {
	args := state.InterfaceArgs{
		Name:             "eth0.42",
		Index:            1,
		MTU:              9000,
		ProviderID:       "eni-42",
		Type:             state.VLAN_8021QInterface,
		HardwareAddress:  "aa:bb:cc:dd:ee:f0",
		IsAutoStart:      true,
		IsUp:             true,
		DNSServers:       []string{"ns1.example.com", "127.0.1.1"},
		DNSSearchDomains: []string{"example.com"},
		GatewayAddress:   "8.8.8.8",
	}
	s.assertAddInterfacesSucceedsAndResultMatchesArgs(c, args)
}

func (s *interfacesStateSuite) assertAddInterfacesSucceedsAndResultMatchesArgs(c *gc.C, args state.InterfaceArgs) {
	s.assertMachineAddInterfacesSucceedsAndResultMatchesArgs(c, s.machine, args, s.State.ModelUUID())
}

func (s *interfacesStateSuite) assertMachineAddInterfacesSucceedsAndResultMatchesArgs(c *gc.C, machine *state.Machine, args state.InterfaceArgs, modelUUID string) {
	err := machine.AddInterfaces(args)
	c.Assert(err, jc.ErrorIsNil)
	result, err := machine.Interface(args.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)

	s.checkAddedInterfaceMatchesArgs(c, result, args)
	s.checkAddedInterfaceMatchesMachineIDAndModelUUID(c, result, s.machine.Id(), modelUUID)
}

func (s *interfacesStateSuite) checkAddedInterfaceMatchesArgs(c *gc.C, addedInterface *state.Interface, args state.InterfaceArgs) {
	c.Check(addedInterface.Name(), gc.Equals, args.Name)
	c.Check(addedInterface.Index(), gc.Equals, args.Index)
	c.Check(addedInterface.MTU(), gc.Equals, args.MTU)
	c.Check(addedInterface.ProviderID(), gc.Equals, args.ProviderID)
	c.Check(addedInterface.Type(), gc.Equals, args.Type)
	c.Check(addedInterface.HardwareAddress(), gc.Equals, args.HardwareAddress)
	c.Check(addedInterface.IsAutoStart(), gc.Equals, args.IsAutoStart)
	c.Check(addedInterface.IsUp(), gc.Equals, args.IsUp)
	c.Check(addedInterface.ParentName(), gc.Equals, args.ParentName)
	c.Check(addedInterface.DNSServers(), jc.DeepEquals, args.DNSServers)
	c.Check(addedInterface.DNSSearchDomains(), jc.DeepEquals, args.DNSSearchDomains)
	c.Check(addedInterface.GatewayAddress(), gc.Equals, args.GatewayAddress)
}

func (s *interfacesStateSuite) checkAddedInterfaceMatchesMachineIDAndModelUUID(c *gc.C, addedInterface *state.Interface, machineID, modelUUID string) {
	globalKey := fmt.Sprintf("m#%si#%s", machineID, addedInterface.Name())
	c.Check(addedInterface.DocID(), gc.Equals, modelUUID+":"+globalKey)
	c.Check(addedInterface.MachineID(), gc.Equals, machineID)
}

func (s *interfacesStateSuite) TestAddInterfacesNoProviderIDSuccess(c *gc.C) {
	args := state.InterfaceArgs{
		Name: "eno0",
		Type: state.EthernetInterface,
	}
	s.assertAddInterfacesSucceedsAndResultMatchesArgs(c, args)
}

func (s *interfacesStateSuite) TestAddInterfacesWithDuplicateProviderIDFailsInSameModel(c *gc.C) {
	args1 := state.InterfaceArgs{
		Name:       "eth0.42",
		Type:       state.EthernetInterface,
		ProviderID: "42",
	}
	s.assertAddInterfacesSucceedsAndResultMatchesArgs(c, args1)

	args2 := args1
	args2.Name = "br-eth0"
	err := s.assertAddInterfacesFailsValidationForArgs(c, args2, `ProviderID\(s\) not unique: 42`)
	c.Assert(err, jc.Satisfies, state.IsProviderIDNotUniqueError)
}

func (s *interfacesStateSuite) TestAddInterfacesWithDuplicateNameAndProviderIDSucceedsInDifferentModels(c *gc.C) {
	args := state.InterfaceArgs{
		Name:       "eth0.42",
		Type:       state.EthernetInterface,
		ProviderID: "42",
	}
	s.assertAddInterfacesSucceedsAndResultMatchesArgs(c, args)

	s.assertMachineAddInterfacesSucceedsAndResultMatchesArgs(c, s.otherStateMachine, args, s.otherState.ModelUUID())
}

func (s *interfacesStateSuite) TestAddInterfacesWithDuplicateNameAndEmptyProviderIDReturnsAlreadyExistsErrorInSameModel(c *gc.C) {
	args := state.InterfaceArgs{
		Name: "eth0.42",
		Type: state.EthernetInterface,
	}
	s.assertAddInterfacesSucceedsAndResultMatchesArgs(c, args)

	err := s.assertAddInterfacesFailsForArgs(c, args, `interface "eth0.42" already exists`)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *interfacesStateSuite) TestAddInterfacesWithDuplicateNameAndProviderIDFailsInSameModel(c *gc.C) {
	args := state.InterfaceArgs{
		Name:       "foo",
		Type:       state.EthernetInterface,
		ProviderID: "42",
	}
	s.assertAddInterfacesSucceedsAndResultMatchesArgs(c, args)

	err := s.assertAddInterfacesFailsValidationForArgs(c, args, `ProviderID\(s\) not unique: 42`)
	c.Assert(err, jc.Satisfies, state.IsProviderIDNotUniqueError)
}

func (s *interfacesStateSuite) TestAddInterfacesMultipleArgsWithSameNameFails(c *gc.C) {
	foo1 := state.InterfaceArgs{
		Name: "foo",
		Type: state.BridgeInterface,
	}
	foo2 := state.InterfaceArgs{
		Name: "foo",
		Type: state.EthernetInterface,
	}
	err := s.machine.AddInterfaces(foo1, foo2)
	c.Assert(err, gc.ErrorMatches, `.*invalid interface "foo": Name specified more than once`)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *interfacesStateSuite) TestAddInterfacesMultipleArgsChildParentOrderDoesNotMatter(c *gc.C) {
	allArgs := []state.InterfaceArgs{{
		Name:       "child1",
		Type:       state.EthernetInterface,
		ParentName: "parent1",
	}, {
		Name: "parent1",
		Type: state.BridgeInterface,
	}, {
		Name: "parent2",
		Type: state.BondInterface,
	}, {
		Name:       "child2",
		Type:       state.VLAN_8021QInterface,
		ParentName: "parent2",
	}}

	s.addInterfacesMultipleArgsSucceedsAndEnsureAllAdded(c, allArgs)
}

func (s *interfacesStateSuite) addInterfacesMultipleArgsSucceedsAndEnsureAllAdded(c *gc.C, allArgs []state.InterfaceArgs) {
	err := s.machine.AddInterfaces(allArgs...)
	c.Assert(err, jc.ErrorIsNil)

	machineID, modelUUID := s.machine.Id(), s.State.ModelUUID()
	for _, args := range allArgs {
		nic, err := s.machine.Interface(args.Name)
		c.Check(err, jc.ErrorIsNil)
		s.checkAddedInterfaceMatchesArgs(c, nic, args)
		s.checkAddedInterfaceMatchesMachineIDAndModelUUID(c, nic, machineID, modelUUID)
	}
}

func (s *interfacesStateSuite) TestAddInterfacesMultipleChildrenOfExistingParentSucceeds(c *gc.C) {
	parent := s.addSimpleInterface(c)
	childrenArgs := []state.InterfaceArgs{{
		Name:       "child1",
		Type:       state.EthernetInterface,
		ParentName: parent.Name(),
	}, {
		Name:       "child2",
		Type:       state.UnknownInterface,
		ParentName: parent.Name(),
	}}

	s.addInterfacesMultipleArgsSucceedsAndEnsureAllAdded(c, childrenArgs)
}

func (s *interfacesStateSuite) addSimpleInterface(c *gc.C) *state.Interface {
	args := state.InterfaceArgs{
		Name: "foo",
		Type: state.UnknownInterface,
	}
	err := s.machine.AddInterfaces(args)
	c.Assert(err, jc.ErrorIsNil)
	nic, err := s.machine.Interface(args.Name)
	c.Assert(err, jc.ErrorIsNil)
	return nic
}

func (s *interfacesStateSuite) TestMachineMethodReturnsNotFoundErrorWhenMissing(c *gc.C) {
	nic := s.addSimpleInterface(c)

	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	result, err := nic.Machine()
	c.Assert(err, gc.ErrorMatches, "machine 0 not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(result, gc.IsNil)
}

func (s *interfacesStateSuite) TestMachineMethodReturnsMachine(c *gc.C) {
	nic := s.addSimpleInterface(c)

	result, err := nic.Machine()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, s.machine)
}

func (s *interfacesStateSuite) TestParentInterfaceReturnsInterface(c *gc.C) {
	args := []state.InterfaceArgs{{
		Name: "br-eth0",
		Type: state.BridgeInterface,
	}, {
		Name:       "eth0",
		Type:       state.EthernetInterface,
		ParentName: "br-eth0",
	}}
	s.addInterfacesMultipleArgsSucceedsAndEnsureAllAdded(c, args)

	child, err := s.machine.Interface("eth0")
	c.Assert(err, jc.ErrorIsNil)
	parent, err := child.ParentInterface()
	c.Assert(err, jc.ErrorIsNil)
	s.checkAddedInterfaceMatchesArgs(c, parent, args[0])
	s.checkAddedInterfaceMatchesMachineIDAndModelUUID(c, parent, s.machine.Id(), s.State.ModelUUID())
}

func (s *interfacesStateSuite) TestMachineInterfaceReturnsNotFoundErrorWhenMissing(c *gc.C) {
	result, err := s.machine.Interface("missing")
	c.Assert(result, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `interface "missing" on machine "0" not found`)
}

func (s *interfacesStateSuite) TestMachineInterfaceReturnsInterface(c *gc.C) {
	existingInterface := s.addSimpleInterface(c)

	result, err := s.machine.Interface(existingInterface.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, existingInterface)
}

func (s *interfacesStateSuite) TestMachineAllInterfaces(c *gc.C) {
	s.assertNoInterfacesOnMachine(c, s.machine)

	args := []state.InterfaceArgs{{
		Name: "br-bond0",
		Type: state.BridgeInterface,
	}, {
		Name:       "bond0",
		Type:       state.BondInterface,
		ParentName: "br-bond0",
	}, {
		Name:       "eth0",
		Type:       state.EthernetInterface,
		ParentName: "bond0",
	}, {
		Name:       "eth1",
		Type:       state.EthernetInterface,
		ParentName: "bond0",
	}}
	s.addInterfacesMultipleArgsSucceedsAndEnsureAllAdded(c, args)

	results, err := s.machine.AllInterfaces()
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

func (s *interfacesStateSuite) assertNoInterfacesOnMachine(c *gc.C, machine *state.Machine) {
	s.assertAllInterfacesOnMachineMatchCount(c, machine, 0)
}

func (s *interfacesStateSuite) assertAllInterfacesOnMachineMatchCount(c *gc.C, machine *state.Machine, expectedCount int) {
	results, err := machine.AllInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, expectedCount)
}

func (s *interfacesStateSuite) TestMachineAllInterfacesOnlyReturnsSameModelInterfaces(c *gc.C) {
	s.assertNoInterfacesOnMachine(c, s.machine)
	s.assertNoInterfacesOnMachine(c, s.otherStateMachine)

	args := []state.InterfaceArgs{{
		Name: "foo",
		Type: state.EthernetInterface,
	}, {
		Name:       "foo.42",
		Type:       state.VLAN_8021QInterface,
		ParentName: "foo",
	}}
	s.addInterfacesMultipleArgsSucceedsAndEnsureAllAdded(c, args)

	results, err := s.machine.AllInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	c.Assert(results[0].Name(), gc.Matches, "(foo|foo.42)")
	c.Assert(results[1].Name(), gc.Matches, "(foo|foo.42)")

	s.assertNoInterfacesOnMachine(c, s.otherStateMachine)
}

func (s *interfacesStateSuite) TestInterfaceRemoveFailsWithExistingChildren(c *gc.C) {
	args := []state.InterfaceArgs{{
		Name: "parent",
		Type: state.BridgeInterface,
	}, {
		Name:       "one-child",
		Type:       state.EthernetInterface,
		ParentName: "parent",
	}, {
		Name:       "another-child",
		Type:       state.EthernetInterface,
		ParentName: "parent",
	}}
	s.addInterfacesMultipleArgsSucceedsAndEnsureAllAdded(c, args)

	parent, err := s.machine.Interface("parent")
	c.Assert(err, jc.ErrorIsNil)

	err = parent.Remove()
	expectedError := fmt.Sprintf("cannot remove %s: parent interface to: another-child, one-child", parent)
	c.Assert(err, gc.ErrorMatches, expectedError)
}

func (s *interfacesStateSuite) TestInterfaceRemoveSuccess(c *gc.C) {
	existingInterface := s.addSimpleInterface(c)

	s.removeInterfaceAndAssertSuccess(c, existingInterface)
	s.assertNoInterfacesOnMachine(c, s.machine)
}

func (s *interfacesStateSuite) removeInterfaceAndAssertSuccess(c *gc.C, givenInterface *state.Interface) {
	err := givenInterface.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *interfacesStateSuite) TestInterfaceRemoveTwiceStillSucceeds(c *gc.C) {
	existingInterface := s.addSimpleInterface(c)

	s.removeInterfaceAndAssertSuccess(c, existingInterface)
	s.removeInterfaceAndAssertSuccess(c, existingInterface)
	s.assertNoInterfacesOnMachine(c, s.machine)
}

func (s *interfacesStateSuite) TestMachineRemoveAllInterfacesDeletesAllInterfaces(c *gc.C) {
	s.assertNoInterfacesOnMachine(c, s.machine)

	args := []state.InterfaceArgs{{
		Name: "foo",
		Type: state.EthernetInterface,
	}, {
		Name:       "bar",
		Type:       state.VLAN_8021QInterface,
		ParentName: "foo",
	}}
	s.addInterfacesMultipleArgsSucceedsAndEnsureAllAdded(c, args)

	err := s.machine.RemoveAllInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	s.assertNoInterfacesOnMachine(c, s.machine)
}

func (s *interfacesStateSuite) TestMachineRemoveAllInterfacesNoErrorIfNoInterfacesExist(c *gc.C) {
	s.assertNoInterfacesOnMachine(c, s.machine)

	err := s.machine.RemoveAllInterfaces()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *interfacesStateSuite) TestInterfaceRefreshUpdatesStaleDocData(c *gc.C) {
	fooInterface := s.addSimpleInterface(c)
	c.Assert(fooInterface.HardwareAddress(), gc.Equals, "")
	s.removeInterfaceAndAssertSuccess(c, fooInterface)
	args := state.InterfaceArgs{
		Name:            "foo",
		Type:            state.BondInterface,
		HardwareAddress: "aa:bb:cc:dd:ee:f0",
	}
	err := s.machine.AddInterfaces(args)
	c.Assert(err, jc.ErrorIsNil)

	err = fooInterface.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fooInterface.HardwareAddress(), gc.Equals, "aa:bb:cc:dd:ee:f0")
}

func (s *interfacesStateSuite) TestInterfaceRefreshPassesThroughNotFoundError(c *gc.C) {
	existingInterface := s.addSimpleInterface(c)
	s.removeInterfaceAndAssertSuccess(c, existingInterface)

	err := existingInterface.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `interface "foo" on machine "0" not found`)
}

func (s *interfacesStateSuite) TestAddInterfacesRollbackWithDuplicateProviderIDs(c *gc.C) {
	insertingArgs := []state.InterfaceArgs{{
		Name:       "child",
		Type:       state.EthernetInterface,
		ProviderID: "child-id",
		ParentName: "parent",
	}, {
		Name:       "parent",
		Type:       state.BridgeInterface,
		ProviderID: "parent-id",
	}}

	assertTwoExistAndRemoveAll := func() {
		s.assertAllInterfacesOnMachineMatchCount(c, s.machine, 2)
		err := s.machine.RemoveAllInterfaces()
		c.Assert(err, jc.ErrorIsNil)
	}

	hooks := []jujutxn.TestHook{{
		Before: func() {
			// Add the same interfaces to trigger ErrAborted in the first attempt.
			s.assertNoInterfacesOnMachine(c, s.machine)
			err := s.machine.AddInterfaces(insertingArgs...)
			c.Assert(err, jc.ErrorIsNil)
		},
		After: assertTwoExistAndRemoveAll,
	}, {
		Before: func() {
			// Add interfaces with same ProviderIDs but different names.
			s.assertNoInterfacesOnMachine(c, s.machine)
			insertingAlternateArgs := insertingArgs
			insertingAlternateArgs[0].Name = "other-child"
			insertingAlternateArgs[0].ParentName = "other-parent"
			insertingAlternateArgs[1].Name = "other-parent"
			err := s.machine.AddInterfaces(insertingAlternateArgs...)
			c.Assert(err, jc.ErrorIsNil)
		},
		After: assertTwoExistAndRemoveAll,
	}}
	defer state.SetTestHooks(c, s.State, hooks...).Check()

	err := s.machine.AddInterfaces(insertingArgs...)
	c.Assert(err, gc.ErrorMatches, `.*ProviderID\(s\) not unique: child-id, parent-id`)
	c.Assert(err, jc.Satisfies, state.IsProviderIDNotUniqueError)
	s.assertNoInterfacesOnMachine(c, s.machine) // Rollback worked.
}

func (s *interfacesStateSuite) TestAddInterfacesWithLightStateChurn(c *gc.C) {
	allArgs, churnHook := s.prepareAddInterfacesWithStateChurn(c)
	defer state.SetTestHooks(c, s.State, churnHook).Check()
	s.assertNoInterfacesOnMachine(c, s.machine)

	s.addInterfacesMultipleArgsSucceedsAndEnsureAllAdded(c, allArgs)
}

func (s *interfacesStateSuite) prepareAddInterfacesWithStateChurn(c *gc.C) ([]state.InterfaceArgs, jujutxn.TestHook) {
	parentArgs := state.InterfaceArgs{
		Name: "parent",
		Type: state.BridgeInterface,
	}
	childArgs := state.InterfaceArgs{
		Name:       "child",
		Type:       state.EthernetInterface,
		ParentName: "parent",
	}

	churnHook := jujutxn.TestHook{
		Before: func() {
			s.assertNoInterfacesOnMachine(c, s.machine)
			err := s.machine.AddInterfaces(parentArgs)
			c.Assert(err, jc.ErrorIsNil)
		},
		After: func() {
			s.assertAllInterfacesOnMachineMatchCount(c, s.machine, 1)
			parent, err := s.machine.Interface("parent")
			c.Assert(err, jc.ErrorIsNil)
			err = parent.Remove()
			c.Assert(err, jc.ErrorIsNil)
		},
	}

	return []state.InterfaceArgs{parentArgs, childArgs}, churnHook
}

func (s *interfacesStateSuite) TestAddInterfacesWithModerateStateChurn(c *gc.C) {
	allArgs, churnHook := s.prepareAddInterfacesWithStateChurn(c)
	defer state.SetTestHooks(c, s.State, churnHook, churnHook).Check()
	s.assertNoInterfacesOnMachine(c, s.machine)

	s.addInterfacesMultipleArgsSucceedsAndEnsureAllAdded(c, allArgs)
}

func (s *interfacesStateSuite) TestAddInterfacesWithTooMuchStateChurn(c *gc.C) {
	allArgs, churnHook := s.prepareAddInterfacesWithStateChurn(c)
	defer state.SetTestHooks(c, s.State, churnHook, churnHook, churnHook).Check()
	s.assertNoInterfacesOnMachine(c, s.machine)

	err := s.machine.AddInterfaces(allArgs...)
	c.Assert(errors.Cause(err), gc.Equals, jujutxn.ErrExcessiveContention)

	s.assertNoInterfacesOnMachine(c, s.machine)
}
