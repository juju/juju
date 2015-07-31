// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

// TODO(ericsnow) Turn StateError into a field (like Failed)?

// The Juju-recognized states in which a workload process might be.
const (
	StateUndefined = ""
	StateDefined   = "defined"
	StateStarting  = "starting"
	StateRunning   = "running"
	StateError     = "error"
	StateStopping  = "stopping"
	StateStopped   = "stopped"
)

var (
	okayStates = set.NewStrings(
		StateUndefined, // TODO(ericsnow) Drop from the set.
		StateDefined,
		StateStarting,
		StateRunning,
		StateError,
		StateStopping,
		StateStopped,
	)
)

// TODO(ericsnow) Use a separate StatusInfo and keep Status (quasi-)immutable?

// Status is the Juju-level status of a workload process.
type Status struct {
	// State is which state the process is in relative to Juju.
	State string
	// Failed identifies whether or not Juju got a failure while trying
	// to interact with the process (via its plugin).
	Failed bool
	// Message is a human-readable message describing the current status
	// of the process, why it is in the current state, or what Juju is
	// doing right now relative to the process. There may be no message.
	Message string
}

// TODO(ericsnow) Add a String method.

// IsBlocked indicates whether or not the workload process may proceed
// to the next state.
func (s *Status) IsBlocked() bool {
	return s.Failed || s.State == StateError
}

// Advance updates the state of the Status to the next appropriate one.
// If a message is provided, it is set. Otherwise the current message
// will be cleared.
func (s *Status) Advance(message string) error {
	if s.Failed {
		return errors.Errorf("cannot advance from a failed state")
	}
	switch s.State {
	case StateUndefined:
		s.State = StateDefined
	case StateDefined:
		s.State = StateStarting
	case StateStarting:
		s.State = StateRunning
	case StateRunning:
		s.State = StateStopping
	case StateError:
		return errors.Errorf("cannot advance from an error state")
	case StateStopping:
		s.State = StateStopped
	case StateStopped:
		return errors.Errorf("cannot advance from a final state")
	default:
		return errors.NotValidf("unrecognized state %q", s.State)
	}
	s.Message = message
	return nil
}

// SetFailed records that Juju encountered a problem when trying to
// interact with the process. If Status.Failed is already true then the
// message is updated. If the process is in an initial or final state
// then an error is returned.
func (s *Status) SetFailed(message string) error {
	switch s.State {
	case StateUndefined:
		return errors.Errorf("cannot fail while in an initial state")
	case StateDefined:
		return errors.Errorf("cannot fail while in an initial state")
	case StateStopped:
		return errors.Errorf("cannot fail while in a final state")
	}

	if message == "" {
		message = "problem while interacting with workload process"
	}
	s.Failed = true
	s.Message = message
	return nil
}

// SetError records that the workload process isn't working correctly,
// as reported by the plugin. If already in an error state then the
// message is updated. The process must be in a running state for there
// to be an error. problems during starting and stopping are recorded
// as failures rather than errors.
func (s *Status) SetError(message string) error {
	switch s.State {
	case StateRunning:
	case StateError:
	default:
		return errors.Errorf("can error only while running")
	}

	if message == "" {
		message = "the workload process has an error"
	}
	s.State = StateError
	s.Message = message
	return nil
}

// Resolve clears any existing error or failure status for the process.
// If a message is provided then it is set. Otherwise a message
// describing what was resolved will be set. If the process is both
// failed and in an error state then both will be resolved at once.
// If the process isn't currently blocked then an error is returned.
func (s *Status) Resolve(message string) error {
	if !s.IsBlocked() {
		// TODO(ericsnow) Do nothing?
		return errors.Errorf("not in an error or failed state")
	}

	if s.State == StateError {
		s.State = StateRunning
		if message == "" {
			// TODO(ericsnow) Add in the current message.
			message = "error resolved"
		}
	} else if s.Failed {
		if message == "" {
			// TODO(ericsnow) Add in the current message.
			message = "failure resolved"
		}
	}

	s.Failed = false
	s.Message = message
	return nil
}

// Validate checkes the status to ensure it contains valid data.
func (s Status) Validate() error {
	if !okayStates.Contains(s.State) {
		return errors.NotValidf("Status.State (%q)", s.State)
	}
	if s.Failed {
		switch s.State {
		case StateUndefined:
			return errors.NotValidf("failure in an initial state")
		case StateDefined:
			return errors.NotValidf("failure in an initial state")
		case StateStopped:
			return errors.NotValidf("failure in a final state")
		}
	}

	return nil
}

// PluginStatus represents the data returned from the Plugin.Status call.
type PluginStatus struct {
	// Label represents the human-readable label returned by the plugin
	// that represents the status of the workload process.
	Label string `json:"label"`
}

// Validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (s PluginStatus) Validate() error {
	if s.Label == "" {
		return errors.NotValidf("Label cannot be empty")
	}
	return nil
}
