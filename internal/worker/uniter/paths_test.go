// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"path/filepath"

	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/internal/worker/uniter"
	"github.com/juju/juju/juju/sockets"
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

func (s *PathsSuite) TestOther(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() ostype.OSType { return ostype.Unknown })

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-application/323")

	paths := uniter.NewPaths(dataDir, unitTag, nil)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))

	localRunSocket := sockets.Socket{Network: "unix", Address: relAgent("run.socket")}
	localJujucSocket := sockets.Socket{Network: "unix", Address: relAgent("agent.socket")}
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-application-323"),
		Runtime: uniter.RuntimePaths{
			LocalJujuExecSocket:    uniter.SocketPair{localRunSocket, localRunSocket},
			LocalJujucServerSocket: uniter.SocketPair{localJujucSocket, localJujucSocket},
		},
		State: uniter.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			ResourcesDir:    relAgent("resources"),
			BundlesDir:      relAgent("state", "bundles"),
			DeployerDir:     relAgent("state", "deployer"),
			MetricsSpoolDir: relAgent("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestWorkerPaths(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() ostype.OSType { return ostype.Unknown })

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-application/323")
	worker := "worker-id"
	paths := uniter.NewWorkerPaths(dataDir, unitTag, worker, nil)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))
	localRunSocket := sockets.Socket{Network: "unix", Address: relAgent(worker + "-run.socket")}
	localJujucSocket := sockets.Socket{Network: "unix", Address: relAgent(worker + "-agent.socket")}
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-application-323"),
		Runtime: uniter.RuntimePaths{
			LocalJujuExecSocket:    uniter.SocketPair{localRunSocket, localRunSocket},
			LocalJujucServerSocket: uniter.SocketPair{localJujucSocket, localJujucSocket},
		},
		State: uniter.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			ResourcesDir:    relAgent("resources"),
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
	c.Assert(paths.GetJujucServerSocket(), gc.DeepEquals, sockets.Socket{Address: "/path/to/socket", Network: "unix"})
	c.Assert(paths.GetMetricsSpoolDir(), gc.Equals, "/path/to/spool/metrics")
}
