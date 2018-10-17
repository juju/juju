// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

type podcfgSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&podcfgSuite{})

func (*podcfgSuite) TestPodLabelsController(c *gc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{})
	controllerJobs := []multiwatcher.MachineJob{multiwatcher.JobManageModel}
	nonControllerJobs := []multiwatcher.MachineJob{multiwatcher.JobHostUnits}
	testPodLabels(c, cfg, controllerJobs, map[string]string{
		"juju-model-uuid":      testing.ModelTag.Id(),
		"juju-controller-uuid": testing.ControllerTag.Id(),
		"juju-is-controller":   "true",
	})
	testPodLabels(c, cfg, nonControllerJobs, map[string]string{
		"juju-model-uuid":      testing.ModelTag.Id(),
		"juju-controller-uuid": testing.ControllerTag.Id(),
	})
}

func (*podcfgSuite) TestPodLabelsUserSpecified(c *gc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"resource-tags": "a=b c=",
	})
	testPodLabels(c, cfg, nil, map[string]string{
		"juju-model-uuid":      testing.ModelTag.Id(),
		"juju-controller-uuid": testing.ControllerTag.Id(),
		"a": "b",
		"c": "",
	})
}

func testPodLabels(c *gc.C, cfg *config.Config, jobs []multiwatcher.MachineJob, expectTags map[string]string) {
	tags := podcfg.PodLabels(testing.ModelTag.Id(), testing.ControllerTag.Id(), cfg, jobs)
	c.Assert(tags, jc.DeepEquals, expectTags)
}

func (*podcfgSuite) TestAgentVersion(c *gc.C) {
	var pcfg podcfg.ControllerPodConfig
	list := coretools.List{
		&coretools.Tools{Version: version.MustParseBinary("2.3.4-trusty-amd64")},
	}
	err := pcfg.SetTools(list)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pcfg.AgentVersion(), gc.Equals, list[0].Version)
}
