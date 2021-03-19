// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v220 = version.MustParse("2.2.0")

type steps22Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps22Suite{})

func (s *steps22Suite) TestAddNonDetachableStorageMachineId(c *gc.C) {
	step := findStateStep(c, v220, "add machineid to non-detachable storage docs")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps22Suite) TestMeterStatusFile(c *gc.C) {
	// Create a meter status file.
	dataDir := c.MkDir()
	statusFile := filepath.Join(dataDir, "meter-status.yaml")
	err := ioutil.WriteFile(statusFile, []byte("things"), 0777)
	c.Assert(err, jc.ErrorIsNil)

	step := findStep(c, v220, "remove meter status file")

	check := func() {
		context := &mockContext{
			agentConfig: &mockAgentConfig{dataDir: dataDir},
		}
		err = step.Run(context)
		c.Assert(err, jc.ErrorIsNil)

		// Status file should be gone.
		c.Check(pathExists(statusFile), jc.IsFalse)
		c.Check(pathExists(dataDir), jc.IsTrue)
	}

	check()
	check() // Check OK when file not present.
}

func (s *steps22Suite) TestAddControllerLogCollectionsSizeSettings(c *gc.C) {
	step := findStateStep(c, v220, "add controller log collection sizing config settings")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps22Suite) TestAddStatusHistoryPruneSettings(c *gc.C) {
	step := findStateStep(c, v220, "add status history pruning config settings")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps22Suite) TestAddStorageInstanceConstraints(c *gc.C) {
	step := findStateStep(c, v220, "add storage constraints to storage instance docs")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps22Suite) TestSplitLogStep(c *gc.C) {
	step := findStateStep(c, v220, "split log collections")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
