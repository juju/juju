// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
)

// The Juju-recognized states in which a workload might be.
const (
	StateUndefined = ""
	StateDefined   = "defined"
	StateStarting  = "starting"
	StateRunning   = "running"
	StateStopping  = "stopping"
	StateStopped   = "stopped"
)

var okayStates = set.NewStrings(
	StateStarting,
	StateRunning,
	StateStopping,
	StateStopped,
)

// The different kinds of status blockers.
const (
	NotBlocked = ""
	// BlockerError indicates that the plugin reported a problem with
	// the workload.
	BlockerError = "error"
	// BlockerFailed indicates that Juju got a failure while trying
	// to interact with the workload (via its plugin).
	BlockerFailed = "failed"
)

// TODO(ericsnow) Use a separate StatusInfo and keep Status (quasi-)immutable?

// Status is the Juju-level status of a workload.
type Status struct {
	// State is which state the workload is in relative to Juju.
	State string
	// Blocker identifies the kind of blocker preventing interaction
	// with the workload.
	Blocker string
	// Message is a human-readable message describing the current status
	// of the workload, why it is in the current state, or what Juju is
	// doing right now relative to the workload. There may be no message.
	Message string
}

// String returns a string representing the status of the workload.
func (s Status) String() string {
	message := s.Message
	if s.IsBlocked() {
		if message == "" {
			message = "<no message>"
		}
		return fmt.Sprintf("(%s) %s", s.Blocker, message)
	}
	return message
}

// IsBlocked indicates whether or not the workload may workload
// to the next state.
func (s *Status) IsBlocked() bool {
	return s.Blocker != NotBlocked
}

// Advance updates the state of the Status to the next appropriate one.
// If a message is provided, it is set. Otherwise the current message
// will be cleared.
func (s *Status) Advance(message string) error {
	if s.IsBlocked() {
		reason := s.Blocker
		return errors.Errorf("cannot advance state (" + reason + ")")
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
// interact with the workload. If Status.Blocker is already failed then
// the message is updated. If the workload is in an initial or final
// state then an error is returned.
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
		message = "problem while interacting with workload"
	}
	s.Blocker = BlockerFailed
	s.Message = message
	return nil
}

// SetError records that the workload isn't working correctly,
// as reported by the plugin. If already in an error state then the
// message is updated. The workload must be in a running state for there
// to be an error. problems during starting and stopping are recorded
// as failures rather than errors.
func (s *Status) SetError(message string) error {
	// TODO(ericsnow) Allow errors in other states?
	switch s.State {
	case StateRunning:
	default:
		return errors.Errorf("can error only while running")
	}

	if message == "" {
		message = "the workload has an error"
	}
	s.Blocker = BlockerError
	s.Message = message
	return nil
}

// Resolve clears any existing error or failure status for the workload.
// If a message is provided then it is set. Otherwise a message
// describing what was resolved will be set. If the workload is both
// failed and in an error state then both will be resolved at once.
// If the workload isn't currently blocked then an error is returned.
func (s *Status) Resolve(message string) error {
	if !s.IsBlocked() {
		// TODO(ericsnow) Do nothing?
		return errors.Errorf("not in an error or failed state")
	}

	defaultMessage := fmt.Sprintf("resolved blocker %q", s.Blocker)
	if message == "" {
		// TODO(ericsnow) Add in the current message?
		message = defaultMessage
	}

	s.Blocker = NotBlocked
	s.Message = message
	return nil
}

// Validate checkes the status to ensure it contains valid data.
func (s Status) Validate() error {
	if !okayStates.Contains(s.State) {
		return errors.NotValidf("Status.State (%q)", s.State)
	}

	switch s.Blocker {
	case BlockerFailed:
		switch s.State {
		case StateUndefined:
			return errors.NotValidf("failure in an initial state")
		case StateDefined:
			return errors.NotValidf("failure in an initial state")
		case StateStopped:
			return errors.NotValidf("failure in a final state")
		}
	case BlockerError:
		if s.State != StateRunning {
			return errors.NotValidf("error outside running state")
		}
	}

	return nil
}

// ValidateState verifies the state passed in is a valid okayState.
func ValidateState(state string) error {
	if !okayStates.Contains(state) {
		return errors.NotValidf("state (%q)", state)
	}
	return nil
}

// PluginStatus represents the data returned from the Plugin.Status call.
type PluginStatus struct {
	// State is the human-readable label returned by the plugin
	// that represents the status of the workload.
	State string `json:"state"`
}

// Validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (s PluginStatus) Validate() error {
	if s.State == "" {
		return errors.NotValidf("State cannot be empty")
	}
	return nil
}

// CombinedStatus holds the status information for a workload,
// from all sources.
type CombinedStatus struct {
	// Status is the Juju-level status information.
	Status Status
	// PluginStatus is the plugin-defined status information.
	PluginStatus PluginStatus
}

// Validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (s CombinedStatus) Validate() error {
	if err := s.Status.Validate(); err != nil {
		return errors.Trace(err)
	}
	if err := s.PluginStatus.Validate(); err != nil {
		return errors.Trace(err)
	}
	return nil
}
