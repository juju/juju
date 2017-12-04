// Copyright 2017 Canonical Ltd.

// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"path/filepath"

	"github.com/juju/juju/agent/tools"
)

// Paths represents the set of filesystem paths a caasoperator worker has reason to
// care about.
type Paths struct {

	// ToolsDir is the directory containing the jujud executable running this
	// process; and also containing hook tool symlinks to that executable. It's
	// the only path in this struct that is not typically pointing inside the
	// directory reserved for the exclusive use of this worker (typically
	// /var/lib/juju/tools )
	ToolsDir string

	// Runtime represents the set of paths that are relevant at runtime.
	Runtime RuntimePaths

	// State represents the set of paths that hold persistent local state for
	// the operator.
	State StatePaths
}

// GetToolsDir exists to satisfy the context.Paths interface.
func (paths Paths) GetToolsDir() string {
	return paths.ToolsDir
}

// GetCharmDir exists to satisfy the context.Paths interface.
func (paths Paths) GetCharmDir() string {
	return paths.State.CharmDir
}

// GetHookCommandSocket exists to satisfy the context.Paths interface.
func (paths Paths) GetHookCommandSocket() string {
	return paths.Runtime.HookCommandServerSocket
}

// GetMetricsSpoolDir exists to satisfy the runner.Paths interface.
func (paths Paths) GetMetricsSpoolDir() string {
	return paths.State.MetricsSpoolDir
}

// RuntimePaths represents the set of paths that are relevant at runtime.
type RuntimePaths struct {

	// JujuRunSocket listens for juju-run invocations, and is always
	// active.
	JujuRunSocket string

	// HookCommandServerSocket listens for hook command invocations, and is only
	// active when supporting a hook execution context.
	HookCommandServerSocket string
}

// StatePaths represents the set of paths that hold persistent local state for
// the operator.
type StatePaths struct {

	// BaseDir is the operator's base directory.
	BaseDir string

	// CharmDir is the directory to which the charm the operator runs is deployed.
	CharmDir string

	// RelationsDir holds relation-specific information about what the
	// operator is doing and/or has done.
	RelationsDir string

	// MetricsSpoolDir acts as temporary storage for metrics being sent from
	// the operator to state.
	MetricsSpoolDir string
}

// NewPaths returns the set of filesystem paths that the supplied unit should
// use, given the supplied root juju data directory path.
func NewPaths(dataDir string) Paths {
	join := filepath.Join
	stateDir := join(dataDir, "state")

	socket := func(name string, abstract bool) string {
		path := join(dataDir, name+".socket")
		if abstract {
			path = "@" + path
		}
		return path
	}

	toolsDir := tools.ToolsDir(dataDir, "")
	return Paths{
		ToolsDir: filepath.FromSlash(toolsDir),
		Runtime: RuntimePaths{
			JujuRunSocket:           socket("run", false),
			HookCommandServerSocket: socket("agent", true),
		},
		State: StatePaths{
			BaseDir:         dataDir,
			CharmDir:        join(dataDir, "charm"),
			RelationsDir:    join(stateDir, "relations"),
			MetricsSpoolDir: join(stateDir, "spool", "metrics"),
		},
	}
}
