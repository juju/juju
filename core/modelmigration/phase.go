// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

// Phase values specify model migration phases.
type Phase int

// Enumerate all possible migration phases.
const (
	UNKNOWN Phase = iota
	QUIESCE
	READONLY
	PRECHECK
	IMPORT
	VALIDATION
	SUCCESS
	LOGTRANSFER
	REAP
	REAPFAILED
	DONE
	ABORT
)

var phaseNames = []string{
	"UNKNOWN",
	"QUIESCE",
	"READONLY",
	"PRECHECK",
	"VALIDATION",
	"IMPORT",
	"SUCCESS",
	"LOGTRANSFER",
	"REAP",
	"REAPFAILED",
	"DONE",
	"ABORT",
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

// Define all possible phase transitions.
//
// The keys are the "from" states and the values enumerate the
// possible "to" states.
var validTransitions = map[Phase][]Phase{
	QUIESCE:     {READONLY, ABORT},
	READONLY:    {PRECHECK, ABORT},
	PRECHECK:    {IMPORT, ABORT},
	IMPORT:      {VALIDATION, ABORT},
	VALIDATION:  {SUCCESS, ABORT},
	SUCCESS:     {LOGTRANSFER},
	LOGTRANSFER: {REAP},
	REAP:        {DONE, REAPFAILED},
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
