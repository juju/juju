// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	jujuos "github.com/juju/os"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/caas/kubernetes/provider"
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
	paths := uniter.NewPaths(dataDir, unitTag, false)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-application-323"),
		Runtime: uniter.RuntimePaths{
			JujuRunSocket:     sockets.Socket{Network: "unix", Address: `\\.\pipe\unit-some-application-323-run`},
			JujucServerSocket: sockets.Socket{Network: "unix", Address: `\\.\pipe\unit-some-application-323-agent`},
		},
		State: uniter.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			OperationsFile:  relAgent("state", "uniter"),
			RelationsDir:    relAgent("state", "relations"),
			BundlesDir:      relAgent("state", "bundles"),
			DeployerDir:     relAgent("state", "deployer"),
			StorageDir:      relAgent("state", "storage"),
			MetricsSpoolDir: relAgent("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestWorkerPathsWindows(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Windows })

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-application/323")
	worker := "some-worker"
	paths := uniter.NewWorkerPaths(dataDir, unitTag, worker, false)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-application-323"),
		Runtime: uniter.RuntimePaths{
			JujuRunSocket:     sockets.Socket{Network: "unix", Address: `\\.\pipe\unit-some-application-323-some-worker-run`},
			JujucServerSocket: sockets.Socket{Network: "unix", Address: `\\.\pipe\unit-some-application-323-some-worker-agent`},
		},
		State: uniter.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			OperationsFile:  relAgent("state", "uniter"),
			RelationsDir:    relAgent("state", "relations"),
			BundlesDir:      relAgent("state", "bundles"),
			DeployerDir:     relAgent("state", "deployer"),
			StorageDir:      relAgent("state", "storage"),
			MetricsSpoolDir: relAgent("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestOther(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Unknown })

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-application/323")
	paths := uniter.NewPaths(dataDir, unitTag, false)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-application-323"),
		Runtime: uniter.RuntimePaths{
			JujuRunSocket:     sockets.Socket{Network: "unix", Address: relAgent("run.socket")},
			JujucServerSocket: sockets.Socket{Network: "unix", Address: "@" + relAgent("agent.socket")},
		},
		State: uniter.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			OperationsFile:  relAgent("state", "uniter"),
			RelationsDir:    relAgent("state", "relations"),
			BundlesDir:      relAgent("state", "bundles"),
			DeployerDir:     relAgent("state", "deployer"),
			StorageDir:      relAgent("state", "storage"),
			MetricsSpoolDir: relAgent("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestTCPRemoteEnvVar(c *gc.C) {
	defer os.Setenv(provider.OperatorPodIPEnvName, os.Getenv(provider.OperatorPodIPEnvName))
	os.Setenv(provider.OperatorPodIPEnvName, "1.1.1.1")
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Unknown })

	dataDir := c.MkDir()
	s.assertTCPRemote(c, dataDir)
}

func (s *PathsSuite) TestTCPRemoteYamlFile(c *gc.C) {
	dataDir := c.MkDir()

	unitTag := names.NewUnitTag("some-application/323")
	ipAddrFile := filepath.Join(dataDir, "agents", unitTag.String(), "operator.yaml")
	err := os.MkdirAll(filepath.Dir(ipAddrFile), 0700)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(ipAddrFile, []byte("operator-address: 1.1.1.1"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.assertTCPRemote(c, dataDir)
}

func (s *PathsSuite) assertTCPRemote(c *gc.C, dataDir string) {
	unitTag := names.NewUnitTag("some-application/323")
	paths := uniter.NewPaths(dataDir, unitTag, true)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-application-323"),
		Runtime: uniter.RuntimePaths{
			JujuRunSocket:     sockets.Socket{Network: "tcp", Address: "1.1.1.1:30666"},
			JujucServerSocket: sockets.Socket{Network: "tcp", Address: "1.1.1.1:30323"},
		},
		State: uniter.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			OperationsFile:  relAgent("state", "uniter"),
			RelationsDir:    relAgent("state", "relations"),
			BundlesDir:      relAgent("state", "bundles"),
			DeployerDir:     relAgent("state", "deployer"),
			StorageDir:      relAgent("state", "storage"),
			MetricsSpoolDir: relAgent("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestWorkerPaths(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Unknown })

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-application/323")
	worker := "worker-id"
	paths := uniter.NewWorkerPaths(dataDir, unitTag, worker, false)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-application-323"))
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-application-323"),
		Runtime: uniter.RuntimePaths{
			JujuRunSocket:     sockets.Socket{Network: "unix", Address: relAgent(worker + "-run.socket")},
			JujucServerSocket: sockets.Socket{Network: "unix", Address: "@" + relAgent(worker+"-agent.socket")},
		},
		State: uniter.StatePaths{
			BaseDir:         relAgent(),
			CharmDir:        relAgent("charm"),
			OperationsFile:  relAgent("state", "uniter"),
			RelationsDir:    relAgent("state", "relations"),
			BundlesDir:      relAgent("state", "bundles"),
			DeployerDir:     relAgent("state", "deployer"),
			StorageDir:      relAgent("state", "storage"),
			MetricsSpoolDir: relAgent("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestContextInterface(c *gc.C) {
	paths := uniter.Paths{
		ToolsDir: "/path/to/tools",
		Runtime: uniter.RuntimePaths{
			JujucServerSocket: sockets.Socket{Network: "unix", Address: "/path/to/socket"},
		},
		State: uniter.StatePaths{
			CharmDir:        "/path/to/charm",
			MetricsSpoolDir: "/path/to/spool/metrics",
		},
	}
	c.Assert(paths.GetToolsDir(), gc.Equals, "/path/to/tools")
	c.Assert(paths.GetCharmDir(), gc.Equals, "/path/to/charm")
	c.Assert(paths.GetJujucSocket(), gc.DeepEquals, sockets.Socket{Address: "/path/to/socket", Network: "unix"})
	c.Assert(paths.GetMetricsSpoolDir(), gc.Equals, "/path/to/spool/metrics")
}
