// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/names/v4"

	"github.com/juju/juju/v2/core/instance"
	corenetwork "github.com/juju/juju/v2/core/network"
	"github.com/juju/juju/v2/state"
	"github.com/juju/juju/v2/state/backups"
)

// This file contains untested shims to let us wrap state in a sensible
// interface and avoid writing tests that depend on mongodb. If you were
// to change any part of it so that it were no longer *obviously* and
// *trivially* correct, you would be Doing It Wrong.

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model
}

// MachineSeries implements backups.Backend
func (s *stateShim) MachineSeries(id string) (string, error) {
	m, err := s.State.Machine(id)
	if err != nil {
		return "", errors.Trace(err)
	}
	return m.Series(), nil
}

// ControllerTag disambiguates the ControllerTag method pending further
// refactoring to separate model functionality from state functionality.
func (s *stateShim) ControllerTag() names.ControllerTag {
	return s.State.ControllerTag()
}

// ModelTag disambiguates the ControllerTag method pending further refactoring
// to separate model functionality from state functionality.
func (s *stateShim) ModelTag() names.ModelTag {
	return s.Model.ModelTag()
}

// ModelType returns type of the model from the shim.
func (s *stateShim) ModelType() state.ModelType {
	return s.Model.Type()
}

// ControllerNodes returns controller nodes in HA.
func (s stateShim) ControllerNodes() ([]state.ControllerNode, error) {
	nodes, err := s.State.ControllerNodes()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]state.ControllerNode, len(nodes))
	for i, n := range nodes {
		result[i] = n
	}
	return result, nil
}

// Machine returns desired Machine.
func (s stateShim) Machine(id string) (Machine, error) {
	m, err := s.State.Machine(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
}

// Machine represent machine used in backups.
type Machine interface {

	// InstanceId has machine's cloud instance id.
	InstanceId() (instance.Id, error)

	// PrivateAddress has machine's private address.
	PrivateAddress() (corenetwork.SpaceAddress, error)

	// PublicAddress has machine's public address.
	PublicAddress() (corenetwork.SpaceAddress, error)

	// Tag has machine's tag.
	Tag() names.Tag

	// Series has machine's series.
	Series() string
}

type sessionShim struct {
	*mgo.Session
}

func (s sessionShim) DB(name string) backups.Database {
	return s.Session.DB(name)
}
