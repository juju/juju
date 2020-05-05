// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/facades/client/backups"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/state"
)

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model

	isController     *bool
	controllerNodesF func() ([]state.ControllerNode, error)
	machineF         func(id string) (backups.Machine, error)
}

func (s *stateShim) IsController() bool {
	if s.isController == nil {
		return s.State.IsController()
	}
	return *s.isController
}

func (s *stateShim) MachineSeries(id string) (string, error) {
	return "xenial", nil
}

func (s *stateShim) ControllerTag() names.ControllerTag {
	return s.State.ControllerTag()
}

func (s *stateShim) ModelType() state.ModelType {
	return s.Model.Type()
}

func (s stateShim) ControllerNodes() ([]state.ControllerNode, error) {
	return s.controllerNodesF()
}

func (s stateShim) Machine(id string) (backups.Machine, error) {
	return s.machineF(id)
}

type testMachine struct {
	*state.Machine
}

func (m *testMachine) InstanceId() (instance.Id, error) {
	return instance.Id("inst-0"), nil
}
