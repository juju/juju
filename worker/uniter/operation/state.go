// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v5"

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

	// CollectMetricsTime records the time the collect metrics hook was last run.
	// It's set to nil if the hook was not run at all. Recording time as int64
	// because the yaml encoder cannot encode the time.Time struct.
	CollectMetricsTime int64 `yaml:"collectmetricstime,omitempty"`

	// SendMetricsTime records the time when metrics were last sent to the
	// state server (see also CollectMetricsTime).
	SendMetricsTime int64 `yaml:"sendmetricstime,omitempty"`

	// UpdateStatusTime records the time the update status hook was last run.
	// It's set to nil if the hook was not run at all.
	UpdateStatusTime int64 `yaml:"updatestatustime,omitempty"`
}

// validate returns an error if the state violates expectations.
func (st State) validate() (err error) {
	defer errors.DeferredAnnotatef(&err, "invalid operation state")
	hasHook := st.Hook != nil
	hasActionId := st.ActionId != nil
	hasCharm := st.CharmURL != nil
	switch st.Kind {
	case Install:
		if hasHook {
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

func (st State) CollectedMetricsAt() time.Time {
	return time.Unix(st.CollectMetricsTime, 0)
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

// StateFile holds the disk state for a uniter.
type StateFile struct {
	path string
}

// NewStateFile returns a new StateFile using path.
func NewStateFile(path string) *StateFile {
	return &StateFile{path}
}

// Read reads a State from the file. If the file does not exist it returns
// ErrNoStateFile.
func (f *StateFile) Read() (*State, error) {
	var st State
	if err := utils.ReadYaml(f.path, &st); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoStateFile
		}
	}
	if err := st.validate(); err != nil {
		return nil, errors.Errorf("cannot read %q: %v", f.path, err)
	}
	return &st, nil
}

// Write stores the supplied state to the file.
func (f *StateFile) Write(st *State) error {
	if err := st.validate(); err != nil {
		panic(err)
	}
	return utils.WriteYaml(f.path, st)
}
