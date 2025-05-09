// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import "github.com/juju/juju/internal/errors"

const (
	// ErrTerminalPhase is the error returned when the object store is already
	// in a terminal phase.
	ErrTerminalPhase = errors.ConstError("object store is already in a terminal phase")
)

// Phase is the type to identify the phase of the object store.
type Phase string

const (
	// PhaseUnknown is the initial phase of a draining action.
	PhaseUnknown Phase = "unknown"

	// PhaseDraining is the phase when the object store is being drained.
	PhaseDraining Phase = "draining"

	// PhaseError is the phase when the object store is in error.
	PhaseError Phase = "error"

	// PhaseCompleted is the phase when the object store is completed.
	PhaseCompleted Phase = "completed"
)

// IsTerminal returns true when the phase means a migration has
// finished (successfully or otherwise).
func (p Phase) IsTerminal() bool {
	switch p {
	case PhaseError, PhaseCompleted:
		return true
	default:
		return false
	}
}

// IsNotStarted returns true when the phase is not started.
func (p Phase) IsNotStarted() bool {
	switch p {
	case PhaseUnknown:
		return true
	default:
		return false
	}
}

// TransitionTo the new phase if it can transition from the current phase
// to the new phase.
func (p Phase) TransitionTo(newPhase Phase) (Phase, error) {
	if !p.IsValid() {
		return "", errors.Errorf("invalid phase %q", p)
	}

	switch p {
	case PhaseUnknown:
		if newPhase == PhaseDraining {
			return newPhase, nil
		}
	case PhaseDraining:
		if newPhase == PhaseError || newPhase == PhaseCompleted {
			return newPhase, nil
		}
	case PhaseError:
		if newPhase.IsTerminal() {
			return p, ErrTerminalPhase
		}
	case PhaseCompleted:
		if newPhase.IsTerminal() {
			return p, ErrTerminalPhase
		}
	}
	return "", errors.Errorf("invalid transition from %q to %q", p, newPhase)
}

// IsValid returns true if the phase is a valid phase.
func (p Phase) IsValid() bool {
	switch p {
	case PhaseUnknown, PhaseDraining, PhaseError, PhaseCompleted:
		return true
	default:
		return false
	}
}

// String returns the string representation of the phase.
func (p Phase) String() string {
	return string(p)
}

// ParsePhase parses the string value into a Phase type.
func ParsePhase(value string) (Phase, error) {
	switch value {
	case string(PhaseUnknown):
		return PhaseUnknown, nil
	case string(PhaseDraining):
		return PhaseDraining, nil
	case string(PhaseError):
		return PhaseError, nil
	case string(PhaseCompleted):
		return PhaseCompleted, nil
	default:
		return "", errors.Errorf("invalid phase %q", value)
	}
}
