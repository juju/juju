// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE store for details.

package operation

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/caasoperator/hook"
)

// Kind enumerates the operations the operator can perform.
type Kind string

const (
	// RunHook indicates that the operator is running a hook.
	RunHook Kind = "run-hook"

	// Continue indicates that the operator should run ModeContinue
	// to determine the next operation.
	Continue Kind = "continue"
)

// Step describes the recorded progression of an operation.
type Step string

const (
	// Queued indicates that the operator should undertake the operation
	// as soon as possible.
	Queued Step = "queued"

	// Pending indicates that the operator has started, but not completed,
	// the operation.
	Pending Step = "pending"

	// Done indicates that the operator has completed the operation,
	// but may not yet have synchronized all necessary secondary state.
	Done Step = "done"
)

// State defines the local state of the operator, excluding relation state.
type State struct {

	// Kind indicates the current operation.
	Kind Kind

	// Step indicates the current operation's progression.
	Step Step

	// Hook holds hook information relevant to the current operation. If Kind
	// is Continue, it holds the last hook that was executed; if Kind is RunHook,
	// it holds the running hook.
	Hook *hook.Info
}

// validate returns an error if the state violates expectations.
func (st State) validate() (err error) {
	defer errors.DeferredAnnotatef(&err, "invalid operation state")
	hasHook := st.Hook != nil
	switch st.Kind {
	case RunHook:
		switch {
		case !hasHook:
			return errors.New("missing hook info with Kind RunHook")
		}
	case Continue:
		if hasHook {
			logger.Errorf("unexpected hook info with Kind Continue")
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

// stateChange is useful for a variety of Operation implementations.
type stateChange struct {
	Kind Kind
	Step Step
	Hook *hook.Info
}

func (change stateChange) apply(state State) *State {
	state.Kind = change.Kind
	state.Step = change.Step
	state.Hook = change.Hook
	return &state
}
