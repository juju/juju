// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	coresettings "github.com/juju/juju/core/settings"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v3"
)

var logger = loggo.GetLogger("juju.apiserver.spaces.shims")

// NewStateShim returns a new state shim.
func NewStateShim(st *state.State) (*stateShim, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &stateShim{EnvironConfigGetter: stateenvirons.EnvironConfigGetter{State: st, Model: m},
		State: st, modelTag: m.ModelTag()}, nil
}

// stateShim forwards and adapts state.State methods to Backing
// method.
type stateShim struct {
	stateenvirons.EnvironConfigGetter
	*state.State
	modelTag names.ModelTag
}

func (s *stateShim) ModelTag() names.ModelTag {
	return s.modelTag
}

func (s *stateShim) AddSpace(name string, providerId network.Id, subnetIds []string, public bool) error {
	_, err := s.State.AddSpace(name, providerId, subnetIds, public)
	return err
}

func (s *stateShim) SpaceByName(name string) (networkingcommon.BackingSpace, error) {
	result, err := s.State.SpaceByName(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	space := networkingcommon.NewSpaceShim(result)
	return space, nil
}

// AllEndpointBindings returns all endpoint bindings and maps it to a corresponding common type
func (s *stateShim) AllEndpointBindings() ([]ApplicationEndpointBindingsShim, error) {
	model, err := s.State.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	endpointBindings, err := model.AllEndpointBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	all := make([]ApplicationEndpointBindingsShim, len(endpointBindings))
	for i, value := range endpointBindings {
		all[i].AppName = value.AppName
		all[i].Bindings = value.Bindings.Map()
	}
	return all, nil
}

// AllMachines returns all machines and maps it to a corresponding common type.
func (s *stateShim) AllMachines() ([]Machine, error) {
	allStateMachines, err := s.State.AllMachines()
	if err != nil {
		return nil, err
	}
	all := make([]Machine, len(allStateMachines))
	for i, m := range allStateMachines {
		all[i] = m
	}
	return all, nil
}

func (s *stateShim) AllSpaces() ([]networkingcommon.BackingSpace, error) {
	results, err := s.State.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}
	spaces := make([]networkingcommon.BackingSpace, len(results))
	for i, result := range results {
		spaces[i] = networkingcommon.NewSpaceShim(result)
	}
	return spaces, nil
}

func (s *stateShim) SubnetByCIDR(cidr string) (networkingcommon.BackingSubnet, error) {
	result, err := s.State.SubnetByCIDR(cidr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return networkingcommon.NewSubnetShim(result), nil
}

func (s *stateShim) SpaceByName(name string) (networkingcommon.BackingSpace, error) {
	result, err := s.State.SpaceByName(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	space := networkingcommon.NewSpaceShim(result)
	return space, nil
}

// TODO: nammn check whether we want to have ops and do transaction here? VS single transactions
// TODO: spaces collection, constraints collection and controllerSettings
func (s *stateShim) RenameSpace(fromSpaceName, toName string) error {

	constraints, err := getChangedConstraints(s.State, fromSpaceName, toName)
	if err != nil {
		logger.Errorf("constraints failed: %q", err)
		return errors.Trace(err)
	}

	settingsChanges, err := getSettingsChanges(s.State, fromSpaceName, toName)
	if err != nil {
		logger.Errorf("settings failed: %q", err)
		return errors.Trace(err)
	}

	err = s.State.RenameSpace(fromSpaceName, toName, settingsChanges, constraints)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func getSettingsChanges(st *state.State, fromSpaceName, toName string) (coresettings.ItemChanges, error) {
	config, err := st.ControllerConfig()
	if err != nil {
		logger.Errorf("failed getting conf: %q", err)
		return nil, errors.Trace(err)
	}
	var deltas coresettings.ItemChanges

	if mgmtSpace := config.JujuManagementSpace(); mgmtSpace == fromSpaceName {
		change := coresettings.MakeModification(jujucontroller.JujuManagementSpace, fromSpaceName, toName)
		deltas = append(deltas, change)
	}
	if haSpace := config.JujuHASpace(); haSpace == fromSpaceName {
		change := coresettings.MakeModification(jujucontroller.JujuHASpace, fromSpaceName, toName)
		deltas = append(deltas, change)
	}
	return deltas, nil
}

// getChangedConstraints will do nothing if there are no spaces constraints to update
func getChangedConstraints(st *state.State, fromSpaceName, toName string) (constraints.Value, error) {
	modelConstraints, err := st.ModelConstraints()
	if err != nil {
		return constraints.Value{}, errors.Trace(err)
	}
	if modelConstraints.HasSpaces() {
		deref := *modelConstraints.Spaces
		for i, space := range *modelConstraints.Spaces {
			if space == fromSpaceName {
				deref[i] = toName
				modelConstraints.Spaces = &deref
				break
			}
		}

	}
	return modelConstraints, nil
}
