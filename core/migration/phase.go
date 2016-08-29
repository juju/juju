// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

// Phase values specify model migration phases.
type Phase int

// Enumerate all possible migration phases.
const (
	UNKNOWN Phase = iota
	NONE
	QUIESCE
	IMPORT
	VALIDATION
	SUCCESS
	LOGTRANSFER
	REAP
	REAPFAILED
	DONE
	ABORT
	ABORTDONE
)

var phaseNames = []string{
	"UNKNOWN", // To catch uninitialised fields.
	"NONE",    // For watchers to indicate there's never been a migration attempt.
	"QUIESCE",
	"IMPORT",
	"VALIDATION",
	"SUCCESS",
	"LOGTRANSFER",
	"REAP",
	"REAPFAILED",
	"DONE",
	"ABORT",
	"ABORTDONE",
}

// String returns the name of an model migration phase constant.
func (p Phase) String() string {
	i := int(p)
	if i >= 0 && i < len(phaseNames) {
		return phaseNames[i]
	}
	return "UNKNOWN"
}

// CanTransitionTo returns true if the given phase is a valid next
// model migration phase.
func (p Phase) CanTransitionTo(targetPhase Phase) bool {
	nextPhases, exists := validTransitions[p]
	if !exists {
		return false
	}
	for _, nextPhase := range nextPhases {
		if nextPhase == targetPhase {
			return true
		}
	}
	return false
}

// IsTerminal returns true if the phase is one which signifies the end
// of a migration.
func (p Phase) IsTerminal() bool {
	for _, t := range terminalPhases {
		if p == t {
			return true
		}
	}
	return false
}

// IsRunning returns true if the phase indicates the migration is
// active and up to or at the SUCCESS phase. It returns false if the
// phase is one of the final cleanup phases or indicates an failed
// migration.
func (p Phase) IsRunning() bool {
	if p.IsTerminal() {
		return false
	}
	switch p {
	case QUIESCE, IMPORT, VALIDATION, SUCCESS:
		return true
	default:
		return false
	}
}

// Define all possible phase transitions.
//
// The keys are the "from" states and the values enumerate the
// possible "to" states.
var validTransitions = map[Phase][]Phase{
	QUIESCE:     {IMPORT, ABORT},
	IMPORT:      {VALIDATION, ABORT},
	VALIDATION:  {SUCCESS, ABORT},
	SUCCESS:     {LOGTRANSFER},
	LOGTRANSFER: {REAP},
	REAP:        {DONE, REAPFAILED},
	ABORT:       {ABORTDONE},
}

var terminalPhases []Phase

func init() {
	// Compute the terminal phases.
	for p := 0; p <= len(phaseNames); p++ {
		phase := Phase(p)
		if _, exists := validTransitions[phase]; !exists {
			terminalPhases = append(terminalPhases, phase)
		}
	}
}

// ParsePhase converts a string model migration phase name
// to its constant value.
func ParsePhase(target string) (Phase, bool) {
	for p, name := range phaseNames {
		if target == name {
			return Phase(p), true
		}
	}
	return UNKNOWN, false
}
