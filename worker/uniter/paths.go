// Copyright 2012-2014 Canonical Ltd.

// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	jujuos "github.com/juju/os"
	"gopkg.in/juju/names.v3"
	"gopkg.in/yaml.v2"

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

// GetCharmDir exists to satisfy the context.Paths interface.
func (paths Paths) GetCharmDir() string {
	return paths.State.CharmDir
}

// GetJujucSocket exists to satisfy the context.Paths interface.
func (paths Paths) GetJujucSocket() sockets.Socket {
	return paths.Runtime.JujucServerSocket
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

// RuntimePaths represents the set of paths that are relevant at runtime.
type RuntimePaths struct {

	// JujuRunSocket listens for juju-run invocations, and is always
	// active.
	JujuRunSocket sockets.Socket

	// JujucServerSocket listens for jujuc invocations, and is only
	// active when supporting a jujuc execution context.
	JujucServerSocket sockets.Socket
}

// StatePaths represents the set of paths that hold persistent local state for
// the uniter.
type StatePaths struct {

	// BaseDir is the unit agent's base directory.
	BaseDir string

	// CharmDir is the directory to which the charm the uniter runs is deployed.
	CharmDir string

	// OperationsFile holds information about what the uniter is doing
	// and/or has done.
	OperationsFile string

	// RelationsDir holds relation-specific information about what the
	// uniter is doing and/or has done.
	RelationsDir string

	// BundlesDir holds downloaded charms.
	BundlesDir string

	// DeployerDir holds metadata about charms that are installing or have
	// been installed.
	DeployerDir string

	// StorageDir holds storage-specific information about what the
	// uniter is doing and/or has done.
	StorageDir string

	// MetricsSpoolDir acts as temporary storage for metrics being sent from
	// the uniter to state.
	MetricsSpoolDir string
}

// NewPaths returns the set of filesystem paths that the supplied unit should
// use, given the supplied root juju data directory path.
func NewPaths(dataDir string, unitTag names.UnitTag, isRemote bool) Paths {
	return NewWorkerPaths(dataDir, unitTag, "", isRemote)
}

// TODO(caas) - move me to generic helper for reading operator config yaml
func socketIP(baseDir string) (string, error) {
	// IP address to use for the socket can either be an env var
	// (when we are the caas operator and are creating the socket to listen),
	// or inside a YAML file (when we are juju-run and need to see where
	// to connect to).
	podIP := os.Getenv(provider.OperatorPodIPEnvName)
	if podIP != "" {
		return podIP, nil
	}
	ipAddrFile := filepath.Join(baseDir, provider.OperatorInfoFile)
	ipAddrData, err := ioutil.ReadFile(ipAddrFile)
	if err != nil {
		return "", errors.Trace(err)
	}
	var socketIP string
	var data map[string]interface{}
	if err := yaml.Unmarshal(ipAddrData, &data); err == nil {
		socketIP, _ = data["operator-address"].(string)
	}
	return socketIP, nil
}

// NewWorkerPaths returns the set of filesystem paths that the supplied unit worker should
// use, given the supplied root juju data directory path and worker identifier.
// Distinct worker identifiers ensure that runtime paths of different worker do not interfere.
func NewWorkerPaths(dataDir string, unitTag names.UnitTag, worker string, isRemote bool) Paths {
	baseDir := agent.Dir(dataDir, unitTag)
	join := filepath.Join
	stateDir := join(baseDir, "state")

	newSocket := func(name string, abstract bool) sockets.Socket {
		if isRemote {
			socketIP, err := socketIP(baseDir)
			if err != nil {
				logger.Warningf("unable to get IP address for jujuc socket: %v", err)
				return sockets.Socket{}
			}
			logger.Debugf("using operator address: %v", socketIP)
			switch name {
			case "agent":
				return sockets.Socket{
					Network: "tcp",
					Address: fmt.Sprintf("%s:%d", socketIP, jujucServerSocketPort+unitTag.Number()),
				}
			case "run":
				return sockets.Socket{
					Network: "tcp",
					Address: fmt.Sprintf("%s:%d", socketIP, provider.JujuRunServerSocketPort),
				}
			default:
				logger.Warningf("caas model socket name %q, fallback to unix protocol", name)
			}
		}
		socket := sockets.Socket{Network: "unix"}
		if jujuos.HostOS() == jujuos.Windows {
			base := fmt.Sprintf("%s", unitTag)
			if worker != "" {
				base = fmt.Sprintf("%s-%s", unitTag, worker)
			}
			socket.Address = fmt.Sprintf(`\\.\pipe\%s-%s`, base, name)
			return socket
		}
		path := join(baseDir, name+".socket")
		if worker != "" {
			path = join(baseDir, fmt.Sprintf("%s-%s.socket", worker, name))
		}
		if abstract {
			path = "@" + path
		}
		socket.Address = path
		return socket
	}

	toolsDir := tools.ToolsDir(dataDir, unitTag.String())
	return Paths{
		ToolsDir: filepath.FromSlash(toolsDir),
		Runtime: RuntimePaths{
			JujuRunSocket:     newSocket("run", false),
			JujucServerSocket: newSocket("agent", true),
		},
		State: StatePaths{
			BaseDir:         baseDir,
			CharmDir:        join(baseDir, "charm"),
			OperationsFile:  join(stateDir, "uniter"),
			RelationsDir:    join(stateDir, "relations"),
			BundlesDir:      join(stateDir, "bundles"),
			DeployerDir:     join(stateDir, "deployer"),
			StorageDir:      join(stateDir, "storage"),
			MetricsSpoolDir: join(stateDir, "spool", "metrics"),
		},
	}
}
