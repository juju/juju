// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
)

// Kind enumerates the operations the uniter can perform.
type Kind string

const (
	// Install indicates that the uniter is installing the charm.
	Install Kind = "install"

	// RunHook indicates that the uniter is running a hook.
	RunHook Kind = "run-hook"

	// RunAction indicates that the uniter is running an action.
	RunAction Kind = "run-action"

	// Upgrade indicates that the uniter is upgrading the charm.
	Upgrade Kind = "upgrade"

	// Continue indicates that the uniter should run ModeContinue
	// to determine the next operation.
	Continue Kind = "continue"

	// RemoteInit indicates the CAAS uniter is installing/upgrading the
	// charm on the remote instance.
	RemoteInit Kind = "remote-init"
)

// Step describes the recorded progression of an operation.
type Step string

const (
	// Queued indicates that the uniter should undertake the operation
	// as soon as possible.
	Queued Step = "queued"

	// Pending indicates that the uniter has started, but not completed,
	// the operation.
	Pending Step = "pending"

	// Done indicates that the uniter has completed the operation,
	// but may not yet have synchronized all necessary secondary state.
	Done Step = "done"
)

// State defines the local persistent state of the uniter, excluding relation
// state.
type State struct {

	// Leader indicates whether a leader-elected hook has been queued to run, and
	// no more recent leader-deposed hook has completed.
	Leader bool `yaml:"leader"`

	// Started indicates whether the start hook has run.
	Started bool `yaml:"started"`

	// Stopped indicates whether the stop hook has run.
	Stopped bool `yaml:"stopped"`

	// Installed indicates whether the install hook has run.
	Installed bool `yaml:"installed"`

	// Removed indicates whether the remove hook has run.
	Removed bool `yaml:"removed"`

	// StatusSet indicates whether the charm being deployed has ever invoked
	// the status-set hook tool.
	StatusSet bool `yaml:"status-set"`

	// Kind indicates the current operation.
	Kind Kind `yaml:"op"`

	// Step indicates the current operation's progression.
	Step Step `yaml:"opstep"`

	// Hook holds hook information relevant to the current operation. If Kind
	// is Continue, it holds the last hook that was executed; if Kind is RunHook,
	// it holds the running hook; if Kind is Upgrade, a non-nil hook indicates
	// that the uniter should return to that hook's Pending state after the
	// upgrade is complete (instead of running an upgrade-charm hook).
	Hook *hook.Info `yaml:"hook,omitempty"`

	// ActionId holds action information relevant to the current operation. If
	// Kind is Continue, it holds the last action that was executed; if Kind is
	// RunAction, it holds the running action.
	ActionId *string `yaml:"action-id,omitempty"`

	// Charm describes the charm being deployed by an Install or Upgrade
	// operation, and is otherwise blank.
	CharmURL *charm.URL `yaml:"charm,omitempty"`

	// ConfigHash stores a hash of the latest known charm
	// configuration settings - it's used to determine whether we need
	// to run config-changed.
	ConfigHash string `yaml:"config-hash,omitempty"`

	// TrustHash stores a hash of the latest known charm trust
	// configuration settings - it's used to determine whether we need
	// to run config-changed.
	TrustHash string `yaml:"trust-hash,omitempty"`

	// AddressesHash stores a hash of the latest known
	// machine/container addresses - it's used to determine whether we
	// need to run config-changed.
	AddressesHash string `yaml:"addresses-hash,omitempty"`
}

// Validate returns an error if the state violates expectations.
func (st State) Validate() (err error) {
	defer errors.DeferredAnnotatef(&err, "invalid operation state")
	hasHook := st.Hook != nil
	hasActionId := st.ActionId != nil
	hasCharm := st.CharmURL != nil
	switch st.Kind {
	case Install:
		if st.Installed {
			return errors.New("unexpected hook info with Kind Install")
		}
		fallthrough
	case Upgrade:
		switch {
		case !hasCharm:
			return errors.New("missing charm URL")
		case hasActionId:
			return errors.New("unexpected action id")
		}
	case RunAction:
		switch {
		case !hasActionId:
			return errors.New("missing action id")
		case hasCharm:
			return errors.New("unexpected charm URL")
		}
	case RunHook:
		switch {
		case !hasHook:
			return errors.New("missing hook info with Kind RunHook")
		case hasCharm:
			return errors.New("unexpected charm URL")
		case hasActionId:
			return errors.New("unexpected action id")
		}
	case Continue:
		// TODO(jw4) LP-1438489
		// ModeContinue should no longer have a Hook, but until the upgrade is
		// fixed we can't fail the validation if it does.
		if hasHook {
			logger.Errorf("unexpected hook info with Kind Continue")
		}
		switch {
		case hasCharm:
			return errors.New("unexpected charm URL")
		case hasActionId:
			return errors.New("unexpected action id")
		}
	case RemoteInit:
		// Nothing to check for.
	default:
		return errors.Errorf("unknown operation %q", st.Kind)
	}
	switch st.Step {
	case Queued, Pending, Done:
	default:
		return errors.Errorf("unknown operation step %q", st.Step)
	}
	if hasHook {
		return st.Hook.Validate()
	}
	return nil
}

func (st State) match(otherState State) bool {
	stateYaml, _ := yaml.Marshal(st)
	otherStateYaml, _ := yaml.Marshal(otherState)
	return string(stateYaml) == string(otherStateYaml)
}

// stateChange is useful for a variety of Operation implementations.
type stateChange struct {
	Kind            Kind
	Step            Step
	Hook            *hook.Info
	ActionId        *string
	CharmURL        *charm.URL
	HasRunStatusSet bool
}

func (change stateChange) apply(state State) *State {
	state.Kind = change.Kind
	state.Step = change.Step
	state.Hook = change.Hook
	state.ActionId = change.ActionId
	state.CharmURL = change.CharmURL
	state.StatusSet = state.StatusSet || change.HasRunStatusSet
	return &state
}

// StateOps reads and writes uniter state from/to the controller.
type StateOps struct {
	unitStateRW UnitStateReadWriter
}

// NewStateOps returns a new StateOps.
func NewStateOps(readwriter UnitStateReadWriter) *StateOps {
	return &StateOps{unitStateRW: readwriter}
}

// UnitStateReadWriter encapsulates the methods from a state.Unit
// required to set and get unit state.
//go:generate mockgen -package mocks -destination mocks/uniterstaterw_mock.go github.com/juju/juju/worker/uniter/operation UnitStateReadWriter
type UnitStateReadWriter interface {
	State() (params.UnitStateResult, error)
	SetState(unitState params.SetUnitStateArg) error
}

// Read a State from the controller. If the saved state does not exist
// it returns ErrNoSavedState.
func (f *StateOps) Read() (*State, error) {
	var st State
	unitState, err := f.unitStateRW.State()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if unitState.UniterState == "" {
		return nil, ErrNoSavedState
	}
	if yaml.Unmarshal([]byte(unitState.UniterState), &st) != nil {
		return nil, errors.Trace(err)
	}
	if err := st.Validate(); err != nil {
		return nil, errors.Errorf("validation of uniter state: %v", err)
	}
	return &st, nil
}

// Write stores the supplied state on the controller.
func (f *StateOps) Write(st *State) error {
	if err := st.Validate(); err != nil {
		return errors.Trace(err)
	}
	data, err := yaml.Marshal(st)
	if err != nil {
		return errors.Trace(err)
	}
	s := string(data)
	return f.unitStateRW.SetState(params.SetUnitStateArg{UniterState: &s})
}
