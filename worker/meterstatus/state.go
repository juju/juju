// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/utils"
	"gopkg.in/yaml.v2"
)

// State represents the worker's internal state.
type State struct {
	Code         string        `yaml:"status-code"`
	Info         string        `yaml:"status-info"`
	Disconnected *Disconnected `yaml:"disconnected,omitempty"`
}

// Disconnected stores the information relevant to the inactive meter status worker.
type Disconnected struct {
	Disconnected int64       `yaml:"disconnected-at,omitempty"`
	State        WorkerState `yaml:"disconnected-state,omitempty"`
}

// When returns the time when the unit was disconnected.
func (d Disconnected) When() time.Time {
	return time.Unix(d.Disconnected, 0)
}

//go:generate mockgen -package mocks -destination mocks/interface_mock.go github.com/juju/juju/worker/meterstatus UnitStateAPI,StateReadWriter

// StateReadWriter is implemented by types that can read and write the meter
// worker's internal state.
type StateReadWriter interface {
	Read() (*State, error)
	Write(*State) error
}

// UnitStateAPI describes the API for reading/writing unit state data from/to
// the controller.
type UnitStateAPI interface {
	State() (params.UnitStateResult, error)
	SetState(params.SetUnitStateArg) error
}

var _ StateReadWriter = (*ControllerBackedState)(nil)

// ControllerBackedState is a StateReadWriter that uses the controller as its
// backing store.
type ControllerBackedState struct {
	api UnitStateAPI
}

// NewControllerBackedState returns a new ControllerBackedState that uses the
// provided UnitStateAPI to communicate with the controller.
func NewControllerBackedState(api UnitStateAPI) *ControllerBackedState {
	return &ControllerBackedState{api: api}
}

// Read the current meter status information from the controller.
func (cbs *ControllerBackedState) Read() (*State, error) {
	ust, err := cbs.api.State()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if ust.MeterStatusState == "" {
		return nil, errors.NotFoundf("state")
	}

	var st State
	if err := yaml.Unmarshal([]byte(ust.MeterStatusState), &st); err != nil {
		return nil, errors.Trace(err)
	}

	return &st, nil
}

// Write the supplied status information to the controller.
func (cbs *ControllerBackedState) Write(st *State) error {
	data, err := yaml.Marshal(st)
	if err != nil {
		return errors.Trace(err)
	}

	dataStr := string(data)
	return errors.Trace(
		cbs.api.SetState(params.SetUnitStateArg{
			MeterStatusState: &dataStr,
		}),
	)
}

var _ StateReadWriter = (*DiskBackedState)(nil)

// DiskBackedState stores the meter status on disk.
type DiskBackedState struct {
	path string
}

// NewDiskBackedState creates a DiskBackedState instance that uses path for
// reading/writing the meter status state.
func NewDiskBackedState(path string) *DiskBackedState {
	return &DiskBackedState{path: path}
}

// Read the current meter status information from disk.
func (dbs *DiskBackedState) Read() (*State, error) {
	var st State
	if err := utils.ReadYaml(dbs.path, &st); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFoundf("state file")
		}
		return nil, errors.Trace(err)
	}

	return &st, nil
}

// Write the supplied status information to disk.
func (dbs *DiskBackedState) Write(st *State) error {
	return errors.Trace(utils.WriteYaml(dbs.path, st))
}
