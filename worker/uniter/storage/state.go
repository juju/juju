// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/charm/v7/hooks"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
)

// State describes the State of storage attachments.
type State struct {
	// storage is map of storage attachments.  The
	// key is the storage tag id, the value is attached
	// or not.
	storage map[string]bool
}

func (s *State) Detach(storageID string) error {
	if _, ok := s.storage[storageID]; !ok {
		return errors.NotFoundf("storage %q", storageID)
	}
	s.storage[storageID] = false
	return nil
}

func (s *State) Attach(storageID string) {
	s.storage[storageID] = true
}

func (s *State) Attached(storageID string) (bool, bool) {
	attached, ok := s.storage[storageID]
	return attached, ok
}

func (s *State) Empty() bool {
	return len(s.storage) == 0
}

func NewState() *State {
	return &State{storage: make(map[string]bool)}
}

// ValidateHook returns an error if the supplied hook.Info does not represent
// a valid change to the storage State. Hooks must always be validated
// against the current State before they are run, to ensure that the system
// meets its guarantees about hook execution order.
func (s *State) ValidateHook(hi hook.Info) (err error) {
	defer errors.DeferredAnnotatef(&err, "inappropriate %q hook for storage %q", hi.Kind, hi.StorageId)

	attached, _ := s.Attached(hi.StorageId)
	switch hi.Kind {
	case hooks.StorageAttached:
		if attached {
			return errors.New("storage already attached")
		}
	case hooks.StorageDetaching:
		if !attached {
			return errors.New("storage not attached")
		}
	}
	return nil
}

// stateOps reads and writes storage state from/to the controller.
type stateOps struct {
	unitStateRW UnitStateReadWriter
}

// UnitStateReadWriter encapsulates the methods from a state.Unit
// required to set and get unit state.
type UnitStateReadWriter interface {
	State() (params.UnitStateResult, error)
	SetState(unitState params.SetUnitStateArg) error
}

// NewStateOps returns a new StateOps.
func NewStateOps(rw UnitStateReadWriter) *stateOps {
	return &stateOps{unitStateRW: rw}
}

// Read reads a storage State from the controller. If the saved State
// does not exist it returns NotFound and a new state.
func (f *stateOps) Read() (*State, error) {
	var stor map[string]bool
	unitState, err := f.unitStateRW.State()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if unitState.StorageState == "" {
		return NewState(), errors.NotFoundf("storage State")
	}
	if err = yaml.Unmarshal([]byte(unitState.StorageState), &stor); err != nil {
		return nil, errors.Trace(err)
	}
	return &State{storage: stor}, nil
}

// Write stores the supplied State storage map on the controller.  If
// the storage map is empty, all data will be removed.
func (f *stateOps) Write(st *State) error {
	if st == nil {
		return errors.Trace(errors.BadRequestf("arg is nil"))
	}
	var str string
	if len(st.storage) > 0 {
		data, err := yaml.Marshal(st.storage)
		if err != nil {
			return errors.Trace(err)
		}
		str = string(data)
	}
	return f.unitStateRW.SetState(params.SetUnitStateArg{StorageState: &str})
}
