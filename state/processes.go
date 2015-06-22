// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/process"
)

func (st *State) RegisterProcess(info process.Info) error {
	ps := processesState{}
	if err := ps.register(info); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// TODO(ericsnow) Add names.ProcessTag and use it here?

func (st *State) SetProcessStatus(id string, status process.Status) error {
	ps := processesState{}
	if err := ps.setStatus(id, status); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st *State) ListProcesses(ids ...string) ([]process.Info, error) {
	ps := processesState{}
	results, err := ps.list(ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}

func (st *State) UnregisterProcess(id string) error {
	ps := processesState{}
	if err := ps.unregister(id); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type processesState struct {
}

func (ps processesState) register(info process.Info) error {
	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}

func (ps processesState) setStatus(id string, status process.Status) error {
	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}

func (ps processesState) list(ids ...string) ([]process.Info, error) {
	// TODO(ericsnow) finish!
	return nil, errors.Errorf("not finished")
}

func (ps processesState) unregister(id string) error {
	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}
