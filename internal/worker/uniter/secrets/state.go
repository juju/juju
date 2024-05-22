// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/rpc/params"
)

// State describes the state relating to secrets.
type State struct {
	// ConsumedSecretInfo stores the last seen revision for each secret - it's
	// used to determine if we need to run secret-changed.
	ConsumedSecretInfo map[string]int `yaml:"secret-revisions,omitempty"`

	// SecretObsoleteRevisions stores the revisions for which the secret-remove
	// hook has already been run for a given secret.
	SecretObsoleteRevisions map[string][]int `yaml:"secret-obsolete-revisions,omitempty"`
}

// NewState returns an initial State.
func NewState() *State {
	return &State{
		ConsumedSecretInfo:      map[string]int{},
		SecretObsoleteRevisions: map[string][]int{},
	}
}

// UpdateStateForHook updates the current secrets state with changes in hi.
// It must be called after the respective hook was executed successfully.
// UpdateStateForHook doesn't validate hi but guarantees that successive
// changes of the same hi are idempotent.
func (s *State) UpdateStateForHook(info hook.Info) {
	switch info.Kind {
	case hooks.SecretChanged:
		s.ConsumedSecretInfo[info.SecretURI] = info.SecretRevision
	case hooks.SecretRemove:
		obsolete := set.NewInts(s.SecretObsoleteRevisions[info.SecretURI]...)
		obsolete.Add(info.SecretRevision)
		s.SecretObsoleteRevisions[info.SecretURI] = obsolete.SortedValues()
	}
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *State) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type StateCopy State
	var sc StateCopy
	err := unmarshal(&sc)
	if err != nil {
		return err
	}
	*s = State(sc)
	if s.ConsumedSecretInfo == nil {
		s.ConsumedSecretInfo = map[string]int{}
	}
	if s.SecretObsoleteRevisions == nil {
		s.SecretObsoleteRevisions = map[string][]int{}
	}
	return nil
}

// stateOps reads and writes secrets state from/to the controller.
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

// Read reads secrets state from the controller. If the saved State
// does not exist it returns NotFound and a new state.
func (f *stateOps) Read() (*State, error) {
	var st State
	unitState, err := f.unitStateRW.State()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if unitState.SecretState == "" {
		return NewState(), errors.NotFoundf("secret State")
	}
	if err = yaml.Unmarshal([]byte(unitState.SecretState), &st); err != nil {
		return nil, errors.Trace(err)
	}
	return &st, nil
}

// Write stores the supplied secrets state to the controller.
func (f *stateOps) Write(st *State) error {
	if st == nil {
		return errors.Trace(errors.BadRequestf("arg is nil"))
	}
	var str string
	data, err := yaml.Marshal(st)
	if err != nil {
		return errors.Trace(err)
	}
	str = string(data)
	return f.unitStateRW.SetState(params.SetUnitStateArg{SecretState: &str})
}
