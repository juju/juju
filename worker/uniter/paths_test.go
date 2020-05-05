// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"crypto/tls"
	"path/filepath"

	"github.com/juju/names/v4"
	jujuos "github.com/juju/os"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/worker/uniter"
)

type PathsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&PathsSuite{})

func relPathFunc(base string) func(parts ...string) string {
	return func(parts ...string) string {
		allParts := append([]string{base}, parts...)
		return filepath.Join(allParts...)
	}
}

func (s *PathsSuite) TestWindows(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Windows })

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-application/323")
	paths := uniter.NewPaths(dataDir, unitTag, nil)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))
	localRunSocket := sockets.Socket{Network: "unix", Address: `\\.\pipe\unit-some-application-323-run`}
	localJujucSocket := sockets.Socket{Network: "unix", Address: `\\.\pipe\unit-some-application-323-agent`}
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-application-323"),
		Runtime: uniter.RuntimePaths{
			LocalJujuRunSocket:     uniter.SocketPair{localRunSocket, localRunSocket},
			LocalJujucServerSocket: uniter.SocketPair{localJujucSocket, localJujucSocket},
		},
		State: uniter.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			BundlesDir:      relAgent("state", "bundles"),
			DeployerDir:     relAgent("state", "deployer"),
			MetricsSpoolDir: relAgent("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestWorkerPathsWindows(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Windows })

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-application/323")
	worker := "some-worker"
	paths := uniter.NewWorkerPaths(dataDir, unitTag, worker, nil)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))

	localRunSocket := sockets.Socket{Network: "unix", Address: `\\.\pipe\unit-some-application-323-some-worker-run`}
	localJujucSocket := sockets.Socket{Network: "unix", Address: `\\.\pipe\unit-some-application-323-some-worker-agent`}
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-application-323"),
		Runtime: uniter.RuntimePaths{
			LocalJujuRunSocket:     uniter.SocketPair{localRunSocket, localRunSocket},
			LocalJujucServerSocket: uniter.SocketPair{localJujucSocket, localJujucSocket},
		},
		State: uniter.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			BundlesDir:      relAgent("state", "bundles"),
			DeployerDir:     relAgent("state", "deployer"),
			MetricsSpoolDir: relAgent("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestOther(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Unknown })

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-application/323")

	paths := uniter.NewPaths(dataDir, unitTag, nil)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))

	localRunSocket := sockets.Socket{Network: "unix", Address: relAgent("run.socket")}
	localJujucSocket := sockets.Socket{Network: "unix", Address: "@" + relAgent("agent.socket")}
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-application-323"),
		Runtime: uniter.RuntimePaths{
			LocalJujuRunSocket:     uniter.SocketPair{localRunSocket, localRunSocket},
			LocalJujucServerSocket: uniter.SocketPair{localJujucSocket, localJujucSocket},
		},
		State: uniter.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			BundlesDir:      relAgent("state", "bundles"),
			DeployerDir:     relAgent("state", "deployer"),
			MetricsSpoolDir: relAgent("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestTCPRemote(c *gc.C) {
	unitTag := names.NewUnitTag("some-application/323")

	socketConfig := uniter.SocketConfig{
		ServiceAddress:  "127.0.0.1",
		OperatorAddress: "127.0.0.2",
		TLSConfig: &tls.Config{
			ServerName: "test",
		},
	}

	dataDir := c.MkDir()
	paths := uniter.NewPaths(dataDir, unitTag, &socketConfig)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))
	localRunSocket := sockets.Socket{Network: "unix", Address: relAgent("run.socket")}
	localJujucSocket := sockets.Socket{Network: "unix", Address: "@" + relAgent("agent.socket")}
	remoteRunServerSocket := sockets.Socket{Network: "tcp", Address: ":30666", TLSConfig: socketConfig.TLSConfig}
	remoteRunClientSocket := sockets.Socket{Network: "tcp", Address: "127.0.0.1:30666", TLSConfig: socketConfig.TLSConfig}
	remoteJujucServerSocket := sockets.Socket{Network: "tcp", Address: ":30323", TLSConfig: socketConfig.TLSConfig}
	remoteJujucClientSocket := sockets.Socket{Network: "tcp", Address: "127.0.0.2:30323", TLSConfig: socketConfig.TLSConfig}
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-application-323"),
		Runtime: uniter.RuntimePaths{
			LocalJujuRunSocket:      uniter.SocketPair{localRunSocket, localRunSocket},
			LocalJujucServerSocket:  uniter.SocketPair{localJujucSocket, localJujucSocket},
			RemoteJujuRunSocket:     uniter.SocketPair{remoteRunServerSocket, remoteRunClientSocket},
			RemoteJujucServerSocket: uniter.SocketPair{remoteJujucServerSocket, remoteJujucClientSocket},
		},
		State: uniter.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			BundlesDir:      relAgent("state", "bundles"),
			DeployerDir:     relAgent("state", "deployer"),
			MetricsSpoolDir: relAgent("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestWorkerPaths(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Unknown })

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-application/323")
	worker := "worker-id"
	paths := uniter.NewWorkerPaths(dataDir, unitTag, worker, nil)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))
	localRunSocket := sockets.Socket{Network: "unix", Address: relAgent(worker + "-run.socket")}
	localJujucSocket := sockets.Socket{Network: "unix", Address: "@" + relAgent(worker+"-agent.socket")}
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-application-323"),
		Runtime: uniter.RuntimePaths{
			LocalJujuRunSocket:     uniter.SocketPair{localRunSocket, localRunSocket},
			LocalJujucServerSocket: uniter.SocketPair{localJujucSocket, localJujucSocket},
		},
		State: uniter.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			BundlesDir:      relAgent("state", "bundles"),
			DeployerDir:     relAgent("state", "deployer"),
			MetricsSpoolDir: relAgent("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestContextInterface(c *gc.C) {
	paths := uniter.Paths{
		ToolsDir: "/path/to/tools",
		Runtime: uniter.RuntimePaths{
			LocalJujucServerSocket: uniter.SocketPair{Server: sockets.Socket{Network: "unix", Address: "/path/to/socket"}},
		},
		State: uniter.StatePaths{
			CharmDir:        "/path/to/charm",
			MetricsSpoolDir: "/path/to/spool/metrics",
		},
	}
	c.Assert(paths.GetToolsDir(), gc.Equals, "/path/to/tools")
	c.Assert(paths.GetCharmDir(), gc.Equals, "/path/to/charm")
	c.Assert(paths.GetJujucServerSocket(false), gc.DeepEquals, sockets.Socket{Address: "/path/to/socket", Network: "unix"})
	c.Assert(paths.GetMetricsSpoolDir(), gc.Equals, "/path/to/spool/metrics")
}
