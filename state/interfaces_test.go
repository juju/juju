// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
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

func (s *interfacesStateSuite) TestAddInterfaceEmptyArgs(c *gc.C) {
	args := state.AddInterfaceArgs{}
	s.assertAddInterfaceReturnsNotValidError(c, args, "empty Name not valid")
}

func (s *interfacesStateSuite) assertAddInterfaceReturnsNotValidError(c *gc.C, args state.AddInterfaceArgs, errorCauseMatches string) {
	err := s.assertAddInterfaceFailsWithCauseForArgs(c, args, errorCauseMatches)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *interfacesStateSuite) assertAddInterfaceFailsWithCauseForArgs(c *gc.C, args state.AddInterfaceArgs, errorCauseMatches string) error {
	result, err := s.machine.AddInterface(args)
	c.Assert(result, gc.IsNil)
	expectedError := fmt.Sprintf("cannot add interface %q to machine %q: %s", args.Name, s.machine.Id(), errorCauseMatches)
	c.Assert(err, gc.ErrorMatches, expectedError)
	c.Assert(errors.Cause(err), gc.ErrorMatches, errorCauseMatches)
	return err
}

func (s *interfacesStateSuite) TestAddInterfaceInvalidName(c *gc.C) {
	args := state.AddInterfaceArgs{
		Name: "bad#name",
	}
	s.assertAddInterfaceReturnsNotValidError(c, args, `Name "bad#name" not valid`)
}

func (s *interfacesStateSuite) TestAddInterfaceInvalidType(c *gc.C) {
	args := state.AddInterfaceArgs{
		Name: "ok",
		Type: "bad type",
	}
	s.assertAddInterfaceReturnsNotValidError(c, args, `Type "bad type" not valid`)
}

func (s *interfacesStateSuite) TestAddInterfaceInvalidParentName(c *gc.C) {
	args := state.AddInterfaceArgs{
		Name:       "eth0",
		ParentName: "bad#name",
	}
	s.assertAddInterfaceReturnsNotValidError(c, args, `ParentName "bad#name" not valid`)
}

func (s *interfacesStateSuite) TestAddInterfaceInvalidHardwareAddress(c *gc.C) {
	args := state.AddInterfaceArgs{
		Name:            "eth0",
		Type:            state.EthernetInterface,
		HardwareAddress: "bad mac",
	}
	s.assertAddInterfaceReturnsNotValidError(c, args, `HardwareAddress "bad mac" not valid`)
}

func (s *interfacesStateSuite) TestAddInterfaceInvalidGatewayAddress(c *gc.C) {
	args := state.AddInterfaceArgs{
		Name:           "eth0",
		Type:           state.EthernetInterface,
		GatewayAddress: "bad ip",
	}
	s.assertAddInterfaceReturnsNotValidError(c, args, `GatewayAddress "bad ip" not valid`)
}

func (s *interfacesStateSuite) TestAddInterfaceWhenMachineNotAliveOrGone(c *gc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	args := state.AddInterfaceArgs{
		Name: "eth0",
		Type: state.EthernetInterface,
	}
	s.assertAddInterfaceFailsWithCauseForArgs(c, args, "machine not found or not alive")

	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.assertAddInterfaceFailsWithCauseForArgs(c, args, "machine not found or not alive")
}

func (s *interfacesStateSuite) TestAddInterfaceWhenModelNotAlive(c *gc.C) {
	otherModel, err := s.otherState.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = otherModel.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	args := state.AddInterfaceArgs{
		Name: "eth0",
		Type: state.EthernetInterface,
	}
	_, err = s.otherStateMachine.AddInterface(args)
	c.Assert(err, gc.ErrorMatches, `cannot add interface "eth0" to machine "0": model is no longer alive`)
}

func (s *interfacesStateSuite) TestAddInterfaceWithMissingParent(c *gc.C) {
	args := state.AddInterfaceArgs{
		Name:       "eth0",
		Type:       state.EthernetInterface,
		ParentName: "br-eth0",
	}
	err := s.assertAddInterfaceFailsWithCauseForArgs(c, args, `parent interface "br-eth0" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *interfacesStateSuite) TestAddInterfaceNoParentSuccess(c *gc.C) {
	args := state.AddInterfaceArgs{
		Name:             "eth0",
		Index:            1,
		MTU:              9000,
		ProviderID:       "eni-42",
		Type:             state.EthernetInterface,
		HardwareAddress:  "aa:bb:cc:dd:ee:f0",
		IsAutoStart:      true,
		IsUp:             true,
		DNSServers:       []string{"ns1.example.com", "127.0.1.1"},
		DNSSearchDomains: []string{"example.com"},
		GatewayAddress:   "8.8.8.8",
	}
	s.assertAddInterfaceSucceedsAndResultMatchesArgs(c, args)
}

func (s *interfacesStateSuite) assertAddInterfaceSucceedsAndResultMatchesArgs(c *gc.C, args state.AddInterfaceArgs) {
	s.assertMachineAddInterfaceSucceedsAndResultMatchesArgs(c, s.machine, args, s.State.ModelUUID())
}

func (s *interfacesStateSuite) assertMachineAddInterfaceSucceedsAndResultMatchesArgs(c *gc.C, machine *state.Machine, args state.AddInterfaceArgs, modelUUID string) {
	result, err := machine.AddInterface(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)

	s.checkAddedInterfaceMatchesArgs(c, result, args)
	s.checkAddedInterfaceMatchesMachineIDAndModelUUID(c, result, s.machine.Id(), modelUUID)
}

func (s *interfacesStateSuite) checkAddedInterfaceMatchesArgs(c *gc.C, addedInterface *state.Interface, args state.AddInterfaceArgs) {
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

func (s *interfacesStateSuite) TestAddInterfaceNoProviderIDSuccess(c *gc.C) {
	args := state.AddInterfaceArgs{
		Name: "eno0",
		Type: state.EthernetInterface,
	}
	s.assertAddInterfaceSucceedsAndResultMatchesArgs(c, args)
}

func (s *interfacesStateSuite) TestAddInterfaceWithDuplicateProviderIDFailsInSameModel(c *gc.C) {
	args1 := state.AddInterfaceArgs{
		Name:       "eth0.42",
		Type:       state.EthernetInterface,
		ProviderID: "42",
	}
	s.assertAddInterfaceSucceedsAndResultMatchesArgs(c, args1)

	args2 := args1
	args2.Name = "br-eth0"
	s.assertAddInterfaceFailsWithCauseForArgs(c, args2, `ProviderID "42" not unique`)
}

func (s *interfacesStateSuite) TestAddInterfaceWithDuplicateNameAndProviderIDSucceedsInDifferentModels(c *gc.C) {
	args := state.AddInterfaceArgs{
		Name:       "eth0.42",
		Type:       state.EthernetInterface,
		ProviderID: "42",
	}
	s.assertAddInterfaceSucceedsAndResultMatchesArgs(c, args)

	s.assertMachineAddInterfaceSucceedsAndResultMatchesArgs(c, s.otherStateMachine, args, s.otherState.ModelUUID())
}

func (s *interfacesStateSuite) TestAddInterfaceWithDuplicateNameReturnsAlreadyExistsErrorInSameModel(c *gc.C) {
	args := state.AddInterfaceArgs{
		Name:       "eth0.42",
		Type:       state.EthernetInterface,
		ProviderID: "42",
	}
	s.assertAddInterfaceSucceedsAndResultMatchesArgs(c, args)

	err := s.assertAddInterfaceFailsWithCauseForArgs(c, args, `interface already exists`)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
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

func (s *interfacesStateSuite) addSimpleInterface(c *gc.C) *state.Interface {
	args := state.AddInterfaceArgs{
		Name: "foo",
		Type: state.UnknownInterface,
	}
	nic, err := s.machine.AddInterface(args)
	c.Assert(err, jc.ErrorIsNil)
	return nic
}

func (s *interfacesStateSuite) TestMachineMethodReturnsMachine(c *gc.C) {
	nic := s.addSimpleInterface(c)

	result, err := nic.Machine()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, s.machine)
}

func (s *interfacesStateSuite) TestParentInterfaceReturnsInterface(c *gc.C) {
	parentArgs := state.AddInterfaceArgs{
		Name: "br-eth0",
		Type: state.BridgeInterface,
	}
	s.assertAddInterfaceSucceedsAndResultMatchesArgs(c, parentArgs)

	childArgs := state.AddInterfaceArgs{
		Name:       "eth0",
		Type:       state.EthernetInterface,
		ParentName: parentArgs.Name,
	}
	s.assertAddInterfaceSucceedsAndResultMatchesArgs(c, childArgs)

	child, err := s.machine.Interface(childArgs.Name)
	c.Assert(err, jc.ErrorIsNil)

	parent, err := child.ParentInterface()
	c.Assert(err, jc.ErrorIsNil)
	s.checkAddedInterfaceMatchesArgs(c, parent, parentArgs)
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
