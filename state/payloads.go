// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/workload"
)

// TODO(ericsnow) Track juju-level status in the status collection.

type EnvPayloads interface {
	// ListAll builds the list of registered payloads in the env and returns it.
	ListAll() ([]workload.Payload, error)
}

// TODO(ericsnow) Use a more generic component registration mechanism?

type newEnvPayloadsFunc func(Persistence, func() ([]string, error), func(string) ([]names.UnitTag, error)) (EnvPayloads, error)

var (
	newEnvPayloads newEnvPayloadsFunc
)

// SetPayloadComponent registers the functions that provide the state
// functionality related to payloads.
func SetPayloadsComponent(upFunc newEnvPayloadsFunc) {
	newEnvPayloads = upFunc
}

// EnvPayloads exposes interaction with payloads in state.
func (st *State) EnvPayloads() (EnvPayloads, error) {
	if newEnvPayloads == nil {
		return nil, errors.Errorf("payloads not supported")
	}

	persist := st.newPersistence()
	envPayloads, err := newEnvPayloads(persist, st.allMachineNames, st.unitTagsFor)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return envPayloads, nil
}

func (st *State) allMachineNames() ([]string, error) {
	ms, err := st.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var names []string
	for _, m := range ms {
		names = append(names, m.Id())
	}
	return names, nil
}

func (st *State) unitTagsFor(machine string) ([]names.UnitTag, error) {
	us, err := st.UnitsFor(machine)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var tags []names.UnitTag
	for _, u := range us {
		tags = append(tags, u.UnitTag())
	}
	return tags, nil
}
