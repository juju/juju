// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	coresettings "github.com/juju/juju/core/settings"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

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
	var totalOps []txn.Op

	constraintsOps, err := updateConstraints(s.State, fromSpaceName, toName)
	if err != nil {
		return errors.Trace(err)
	}
	totalOps = append(totalOps, constraintsOps...)

	settingsOps, err := getSettingsOps(s.State, fromSpaceName, toName)
	if err != nil {
		return errors.Trace(err)
	}
	totalOps = append(totalOps, settingsOps...)

	err = s.State.RenameSpace(fromSpaceName, toName, totalOps)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func getSettingsOps(st *state.State, fromSpaceName, toName string) ([]txn.Op, error) {
	config, err := st.ControllerConfig()
	toRename := false
	var deltas coresettings.ItemChanges
	var ops []txn.Op
	if err != nil {
		return nil, err
	}
	if mgmtSpace := config.JujuManagementSpace(); mgmtSpace == fromSpaceName {
		change := coresettings.MakeModification(jujucontroller.JujuManagementSpace, fromSpaceName, toName)
		deltas = append(deltas, change)
		toRename = true
	}
	if haSpace := config.JujuHASpace(); haSpace == fromSpaceName {
		change := coresettings.MakeModification(jujucontroller.JujuHASpace, fromSpaceName, toName)
		deltas = append(deltas, change)
		toRename = true
	}
	if toRename {
		settings := st.NewSettings()
		ops, err = settings.DeltaOps("tocxhange", deltas)
		if err != nil {
			return nil, err
		}
	}

	return ops, nil
}

// updateConstraints will do nothing if there are no spaces constraints to update
func updateConstraints(st *state.State, fromSpaceName, toName string) ([]txn.Op, error) {
	constraints, err := st.ModelConstraints()
	var ops []txn.Op
	if err != nil {
		return nil, errors.Trace(err)
	}
	toUpdateConstraint := false
	if constraints.HasSpaces() {
		deref := *constraints.Spaces
		for i, space := range *constraints.Spaces {
			if space == fromSpaceName {
				toUpdateConstraint = true
				deref[i] = toName
				break
			}
		}
		if toUpdateConstraint {
			constraints.Spaces = &deref
			ops, err = st.GetModelConstraintsOps(constraints)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
	}
	return ops, nil
}
