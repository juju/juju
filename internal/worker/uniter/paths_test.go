// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/uniter"
	"github.com/juju/juju/juju/sockets"
)

type PathsSuite struct {
	testhelpers.IsolationSuite
}

func TestPathsSuite(t *stdtesting.T) { tc.Run(t, &PathsSuite{}) }
func relPathFunc(base string) func(parts ...string) string {
	return func(parts ...string) string {
		allParts := append([]string{base}, parts...)
		return filepath.Join(allParts...)
	}
}

func (s *PathsSuite) TestOther(c *tc.C) {
	s.PatchValue(&jujuos.HostOS, func() ostype.OSType { return ostype.Unknown })

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-application/323")

	paths := uniter.NewPaths(dataDir, unitTag, nil)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))

	localRunSocket := sockets.Socket{Network: "unix", Address: relAgent("run.socket")}
	localJujucSocket := sockets.Socket{Network: "unix", Address: relAgent("agent.socket")}
	c.Assert(paths, tc.DeepEquals, uniter.Paths{
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

func (s *PathsSuite) TestWorkerPaths(c *tc.C) {
	s.PatchValue(&jujuos.HostOS, func() ostype.OSType { return ostype.Unknown })

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-application/323")
	worker := "worker-id"
	paths := uniter.NewWorkerPaths(dataDir, unitTag, worker, nil)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))
	localRunSocket := sockets.Socket{Network: "unix", Address: relAgent(worker + "-run.socket")}
	localJujucSocket := sockets.Socket{Network: "unix", Address: relAgent(worker + "-agent.socket")}
	c.Assert(paths, tc.DeepEquals, uniter.Paths{
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

func (s *PathsSuite) TestContextInterface(c *tc.C) {
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
	c.Assert(paths.GetToolsDir(), tc.Equals, "/path/to/tools")
	c.Assert(paths.GetCharmDir(), tc.Equals, "/path/to/charm")
	c.Assert(paths.GetJujucServerSocket(), tc.DeepEquals, sockets.Socket{Address: "/path/to/socket", Network: "unix"})
	c.Assert(paths.GetMetricsSpoolDir(), tc.Equals, "/path/to/spool/metrics")
}
