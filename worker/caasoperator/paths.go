// Copyright 2017 Canonical Ltd.

// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"path/filepath"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/names/v4"
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

// GetBaseDir exists to satisfy the context.Paths interface.
func (paths Paths) GetBaseDir() string {
	return paths.State.BaseDir
}

// GetCharmDir exists to satisfy the context.Paths interface.
func (paths Paths) GetCharmDir() string {
	return paths.State.CharmDir
}

// GetMetricsSpoolDir exists to satisfy the runner.Paths interface.
func (paths Paths) GetMetricsSpoolDir() string {
	return paths.State.MetricsSpoolDir
}

// ComponentDir returns the filesystem path to the directory
// containing all data files for a component.
func (paths Paths) ComponentDir(name string) string {
	return filepath.Join(paths.State.BaseDir, name)
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

	// OperationsFile holds information about what the operator is doing
	// and/or has done.
	OperationsFile string

	// BundlesDir holds downloaded charms.
	BundlesDir string

	// DeployerDir holds metadata about charms that are installing or have
	// been installed.
	DeployerDir string

	// MetricsSpoolDir acts as temporary storage for metrics being sent from
	// the operator to state.
	MetricsSpoolDir string
}

// NewPaths returns the set of filesystem paths that the supplied unit should
// use, given the supplied root juju data directory path.
func NewPaths(dataDir string, applicationTag names.ApplicationTag) Paths {
	join := filepath.Join
	baseDir := join(dataDir, "agents", applicationTag.String())
	stateDir := join(baseDir, "state")

	toolsDir := tools.ToolsDir(dataDir, "")
	return Paths{
		ToolsDir: filepath.FromSlash(toolsDir),
		State: StatePaths{
			BaseDir:         baseDir,
			CharmDir:        join(baseDir, "charm"),
			BundlesDir:      join(stateDir, "bundles"),
			DeployerDir:     join(stateDir, "deployer"),
			OperationsFile:  join(stateDir, "operator"),
			MetricsSpoolDir: join(stateDir, "spool", "metrics"),
		},
	}
}
