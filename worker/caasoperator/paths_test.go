// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/caasoperator"
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

func (s *PathsSuite) TestPaths(c *gc.C) {
	dataDir := c.MkDir()
	paths := caasoperator.NewPaths(dataDir)

	relData := relPathFunc(dataDir)
	c.Assert(paths, jc.DeepEquals, caasoperator.Paths{
		ToolsDir: relData("tools"),
		Runtime: caasoperator.RuntimePaths{
			JujuRunSocket:           relData("run.socket"),
			HookCommandServerSocket: "@" + relData("agent.socket"),
		},
		State: caasoperator.StatePaths{
			BaseDir:         relData(),
			CharmDir:        relData("charm"),
			RelationsDir:    relData("state", "relations"),
			MetricsSpoolDir: relData("state", "spool", "metrics"),
		},
	})
}

func (s *PathsSuite) TestContextInterface(c *gc.C) {
	paths := caasoperator.Paths{
		ToolsDir: "/path/to/tools",
		Runtime: caasoperator.RuntimePaths{
			HookCommandServerSocket: "/path/to/socket",
		},
		State: caasoperator.StatePaths{
			CharmDir:        "/path/to/charm",
			MetricsSpoolDir: "/path/to/spool/metrics",
		},
	}
	c.Assert(paths.GetToolsDir(), gc.Equals, "/path/to/tools")
	c.Assert(paths.GetCharmDir(), gc.Equals, "/path/to/charm")
	c.Assert(paths.GetHookCommandSocket(), gc.Equals, "/path/to/socket")
	c.Assert(paths.GetMetricsSpoolDir(), gc.Equals, "/path/to/spool/metrics")
}
