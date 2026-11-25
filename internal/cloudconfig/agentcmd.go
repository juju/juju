// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudconfig

import (
	"strings"
	"time"
)

// AgentStartOptions describes the command that starts jujuagentd to
// initialise controller state. It exists so that every consumer of that
// command derives the mode-dependent parts from one place.
type AgentStartOptions struct {
	// ToolPath is the path to the jujuagentd binary. It is emitted
	// verbatim, so callers supply any quoting their shell context needs.
	ToolPath string
	// DataDir is the agent data directory, emitted verbatim for
	// --data-dir. Callers pre-quote it or pass a shell variable.
	DataDir string
	// LoggingOption is the agent logging flag, --debug or --show-log.
	LoggingOption string
	// Timeout bounds the fresh state initialisation. It is ignored in
	// recovery mode, where the subcommand accepts no such flag.
	Timeout time.Duration
	// MachineID is the archived machine identity, passed as --machine-id
	// in recovery mode only: bootstrap-state defines no such flag.
	MachineID string
	// FeatureFlagsEnv is a KEY=value prefix for the command, empty for
	// no prefix.
	FeatureFlagsEnv string
	// RecoveryMode selects recover-state over bootstrap-state.
	RecoveryMode bool
}

// AgentStartCommand returns the shell command that runs jujuagentd's
// state initialisation. It is the single owner of the differences between
// a fresh bootstrap and a controller recovery: recover-state must never be
// passed --timeout, and bootstrap-state must never be passed --machine-id,
// because neither command defines the other mode's flag.
func AgentStartCommand(options AgentStartOptions) string {
	subcommand := "bootstrap-state"
	if options.RecoveryMode {
		subcommand = "recover-state"
	}
	args := []string{options.ToolPath, subcommand, "--data-dir", options.DataDir, options.LoggingOption}
	if options.RecoveryMode {
		if options.MachineID != "" {
			args = append(args, "--machine-id", options.MachineID)
		}
	} else {
		args = append(args, "--timeout", options.Timeout.String())
	}
	cmd := strings.Join(args, " ")
	if options.FeatureFlagsEnv != "" {
		cmd = options.FeatureFlagsEnv + " " + cmd
	}
	return cmd
}

// RecoveryMarkerCommand returns the command that creates the recovery
// marker file. The caller supplies markerPath quoted for its shell context.
func RecoveryMarkerCommand(markerPath string) string {
	return "install -D -m 600 /dev/null " + markerPath
}
