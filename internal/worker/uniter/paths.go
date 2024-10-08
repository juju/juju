// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"crypto/tls"
	"fmt"
	"path/filepath"

	"github.com/juju/names/v5"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/juju/sockets"
)

// Paths represents the set of filesystem paths a uniter worker has reason to
// care about.
type Paths struct {

	// ToolsDir is the directory containing the jujud executable running this
	// process; and also containing jujuc tool symlinks to that executable. It's
	// the only path in this struct that is not typically pointing inside the
	// directory reserved for the exclusive use of this worker (typically
	// /var/lib/juju/agents/$UNIT_TAG/ )
	ToolsDir string

	// Runtime represents the set of paths that are relevant at runtime.
	Runtime RuntimePaths

	// State represents the set of paths that hold persistent local state for
	// the uniter.
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

// GetResourcesDir exists to satisfy the context.Paths interface.
func (paths Paths) GetResourcesDir() string {
	return paths.State.ResourcesDir
}

// GetJujucClientSocket exists to satisfy the context.Paths interface.
func (paths Paths) GetJujucClientSocket() sockets.Socket {
	return paths.Runtime.LocalJujucServerSocket.Client
}

// GetJujucServerSocket exists to satisfy the context.Paths interface.
func (paths Paths) GetJujucServerSocket() sockets.Socket {
	return paths.Runtime.LocalJujucServerSocket.Server
}

// GetMetricsSpoolDir exists to satisfy the runner.Paths interface.
func (paths Paths) GetMetricsSpoolDir() string {
	return paths.State.MetricsSpoolDir
}

// SocketPair is a server+client pair of socket descriptors.
type SocketPair struct {
	Server sockets.Socket
	Client sockets.Socket
}

// RuntimePaths represents the set of paths that are relevant at runtime.
type RuntimePaths struct {
	// LocalJujuExecSocket listens for juju-exec invocations, and is always
	// active.
	LocalJujuExecSocket SocketPair

	// JujucServerSocket listens for jujuc invocations, and is only
	// active when supporting a jujuc execution context.
	LocalJujucServerSocket SocketPair
}

// StatePaths represents the set of paths that hold persistent local state for
// the uniter.
type StatePaths struct {
	// BaseDir is the unit agent's base directory.
	BaseDir string

	// CharmDir is the directory to which the charm the uniter runs is deployed.
	CharmDir string

	// ResourcesDir is the directory to which the charm the uniter runs is deployed.
	ResourcesDir string

	// BundlesDir holds downloaded charms.
	BundlesDir string

	// DeployerDir holds metadata about charms that are installing or have
	// been installed.
	DeployerDir string

	// MetricsSpoolDir acts as temporary storage for metrics being sent from
	// the uniter to state.
	MetricsSpoolDir string
}

// SocketConfig specifies information for remote sockets.
type SocketConfig struct {
	ServiceAddress  string
	OperatorAddress string
	TLSConfig       *tls.Config
}

// NewPaths returns the set of filesystem paths that the supplied unit should
// use, given the supplied root juju data directory path.
// If socketConfig is specified, all sockets will be TLS over TCP.
func NewPaths(dataDir string, unitTag names.UnitTag, socketConfig *SocketConfig) Paths {
	return NewWorkerPaths(dataDir, unitTag, "", socketConfig)
}

// NewWorkerPaths returns the set of filesystem paths that the supplied unit worker should
// use, given the supplied root juju data directory path and worker identifier.
// Distinct worker identifiers ensure that runtime paths of different worker do not interfere.
// If socketConfig is specified, all sockets will be TLS over TCP.
func NewWorkerPaths(dataDir string, unitTag names.UnitTag, worker string, socketConfig *SocketConfig) Paths {
	baseDir := agent.Dir(dataDir, unitTag)
	join := filepath.Join
	stateDir := join(baseDir, "state")

	toolsDir := tools.ToolsDir(dataDir, unitTag.String())
	return Paths{
		ToolsDir: filepath.FromSlash(toolsDir),
		Runtime: RuntimePaths{
			LocalJujuExecSocket:    newUnixSocket(baseDir, unitTag, worker, "run"),
			LocalJujucServerSocket: newUnixSocket(baseDir, unitTag, worker, "agent"),
		},
		State: StatePaths{
			BaseDir:         baseDir,
			CharmDir:        join(baseDir, "charm"),
			ResourcesDir:    join(baseDir, "resources"),
			BundlesDir:      join(stateDir, "bundles"),
			DeployerDir:     join(stateDir, "deployer"),
			MetricsSpoolDir: join(stateDir, "spool", "metrics"),
		},
	}
}

func newUnixSocket(baseDir string, unitTag names.UnitTag, worker string, name string) SocketPair {
	socket := sockets.Socket{Network: "unix"}
	path := filepath.Join(baseDir, name+".socket")
	if worker != "" {
		path = filepath.Join(baseDir, fmt.Sprintf("%s-%s.socket", worker, name))
	}
	socket.Address = path
	return SocketPair{socket, socket}
}
