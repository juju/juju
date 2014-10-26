// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"path/filepath"

	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/version"
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
	s.PatchValue(&version.Current.OS, version.Windows)

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-service/323")
	paths := uniter.NewPaths(dataDir, unitTag)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-service-323"))
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-service-323"),
		Runtime: uniter.RuntimePaths{
			JujuRunSocket:     `\\.\pipe\unit-some-service-323-run`,
			JujucServerSocket: `\\.\pipe\unit-some-service-323-agent`,
		},
		State: uniter.StatePaths{
			CharmDir:       relAgent("charm"),
			OperationsFile: relAgent("state", "uniter"),
			RelationsDir:   relAgent("state", "relations"),
			BundlesDir:     relAgent("state", "bundles"),
			DeployerDir:    relAgent("state", "deployer"),
		},
	})
}

func (s *PathsSuite) TestOther(c *gc.C) {
	s.PatchValue(&version.Current.OS, version.OSType(-1))

	dataDir := c.MkDir()
	unitTag := names.NewUnitTag("some-service/323")
	paths := uniter.NewPaths(dataDir, unitTag)

	relData := relPathFunc(dataDir)
	relAgent := relPathFunc(relData("agents", "unit-some-service-323"))
	c.Assert(paths, jc.DeepEquals, uniter.Paths{
		ToolsDir: relData("tools/unit-some-service-323"),
		Runtime: uniter.RuntimePaths{
			JujuRunSocket:     relAgent("run.socket"),
			JujucServerSocket: "@" + relAgent("agent.socket"),
		},
		State: uniter.StatePaths{
			CharmDir:       relAgent("charm"),
			OperationsFile: relAgent("state", "uniter"),
			RelationsDir:   relAgent("state", "relations"),
			BundlesDir:     relAgent("state", "bundles"),
			DeployerDir:    relAgent("state", "deployer"),
		},
	})
}
