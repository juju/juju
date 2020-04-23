// Copyright 2012-2014 Canonical Ltd.

// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"crypto/tls"
	"fmt"
	"path/filepath"

	jujuos "github.com/juju/os"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/caas/kubernetes/provider"
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

// GetJujucClientSocket exists to satisfy the context.Paths interface.
func (paths Paths) GetJujucClientSocket(remote bool) sockets.Socket {
	if remote {
		return paths.Runtime.RemoteJujucServerSocket.Client
	}
	return paths.Runtime.LocalJujucServerSocket.Client
}

// GetJujucServerSocket exists to satisfy the context.Paths interface.
func (paths Paths) GetJujucServerSocket(remote bool) sockets.Socket {
	if remote {
		return paths.Runtime.RemoteJujucServerSocket.Server
	}
	return paths.Runtime.LocalJujucServerSocket.Server
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

const jujucServerSocketPort = 30000

// SocketPair is a server+client pair of socket descriptors.
type SocketPair struct {
	Server sockets.Socket
	Client sockets.Socket
}

// RuntimePaths represents the set of paths that are relevant at runtime.
type RuntimePaths struct {
	// JujuRunSocket listens for juju-run invocations, and is always
	// active.
	LocalJujuRunSocket SocketPair

	// RemoteJujuRunSocket listens for remote juju-run invocations.
	RemoteJujuRunSocket SocketPair

	// JujucServerSocket listens for jujuc invocations, and is only
	// active when supporting a jujuc execution context.
	LocalJujucServerSocket SocketPair

	// RemoteJujucServerSocket listens for remote jujuc invocations, and is only
	// active when supporting a jujuc execution context.
	RemoteJujucServerSocket SocketPair
}

// StatePaths represents the set of paths that hold persistent local state for
// the uniter.
type StatePaths struct {
	// BaseDir is the unit agent's base directory.
	BaseDir string

	// CharmDir is the directory to which the charm the uniter runs is deployed.
	CharmDir string

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

	var newSocket func(name string) SocketPair
	if socketConfig != nil {
		newSocket = func(name string) SocketPair {
			var port int
			var address string
			switch name {
			case "agent":
				port = jujucServerSocketPort + unitTag.Number()
				address = socketConfig.OperatorAddress
			case "run":
				port = provider.JujuRunServerSocketPort
				address = socketConfig.ServiceAddress
			default:
				return SocketPair{}
			}
			return SocketPair{
				Client: sockets.Socket{
					Network:   "tcp",
					Address:   fmt.Sprintf("%s:%d", address, port),
					TLSConfig: socketConfig.TLSConfig,
				},
				Server: sockets.Socket{
					Network:   "tcp",
					Address:   fmt.Sprintf(":%d", port),
					TLSConfig: socketConfig.TLSConfig,
				},
			}
		}
	} else {
		newSocket = func(name string) SocketPair {
			return SocketPair{}
		}
	}

	toolsDir := tools.ToolsDir(dataDir, unitTag.String())
	return Paths{
		ToolsDir: filepath.FromSlash(toolsDir),
		Runtime: RuntimePaths{
			RemoteJujuRunSocket:     newSocket("run"),
			RemoteJujucServerSocket: newSocket("agent"),
			LocalJujuRunSocket:      newUnixSocket(baseDir, unitTag, worker, "run", false),
			LocalJujucServerSocket:  newUnixSocket(baseDir, unitTag, worker, "agent", true),
		},
		State: StatePaths{
			BaseDir:         baseDir,
			CharmDir:        join(baseDir, "charm"),
			BundlesDir:      join(stateDir, "bundles"),
			DeployerDir:     join(stateDir, "deployer"),
			MetricsSpoolDir: join(stateDir, "spool", "metrics"),
		},
	}
}

func newUnixSocket(baseDir string, unitTag names.UnitTag, worker string, name string, abstract bool) SocketPair {
	socket := sockets.Socket{Network: "unix"}
	if jujuos.HostOS() == jujuos.Windows {
		base := fmt.Sprintf("%s", unitTag)
		if worker != "" {
			base = fmt.Sprintf("%s-%s", unitTag, worker)
		}
		socket.Address = fmt.Sprintf(`\\.\pipe\%s-%s`, base, name)
		return SocketPair{socket, socket}
	}
	path := filepath.Join(baseDir, name+".socket")
	if worker != "" {
		path = filepath.Join(baseDir, fmt.Sprintf("%s-%s.socket", worker, name))
	}
	if abstract {
		path = "@" + path
	}
	socket.Address = path
	return SocketPair{socket, socket}
}
