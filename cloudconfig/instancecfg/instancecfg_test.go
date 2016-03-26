// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancecfg_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
)

type instancecfgSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&instancecfgSuite{})

func (*instancecfgSuite) TestInstanceTagsController(c *gc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{})
	controllerJobs := []multiwatcher.MachineJob{multiwatcher.JobManageModel}
	nonControllerJobs := []multiwatcher.MachineJob{multiwatcher.JobHostUnits}
	testInstanceTags(c, cfg, controllerJobs, map[string]string{
		"juju-model-uuid":    testing.ModelTag.Id(),
		"juju-is-controller": "true",
	})
	testInstanceTags(c, cfg, nonControllerJobs, map[string]string{
		"juju-model-uuid": testing.ModelTag.Id(),
	})
}

func (*instancecfgSuite) TestInstanceTagsUserSpecified(c *gc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"resource-tags": "a=b c=",
	})
	testInstanceTags(c, cfg, nil, map[string]string{
		"juju-model-uuid": testing.ModelTag.Id(),
		"a":               "b",
		"c":               "",
	})
}

func testInstanceTags(c *gc.C, cfg *config.Config, jobs []multiwatcher.MachineJob, expectTags map[string]string) {
	tags := instancecfg.InstanceTags(cfg, jobs)
	c.Assert(tags, jc.DeepEquals, expectTags)
}

func (*instancecfgSuite) TestJujuTools(c *gc.C) {
	icfg := &instancecfg.InstanceConfig{
		DataDir: "/path/to/datadir/",
		Tools: &coretools.Tools{
			Version: version.MustParseBinary("2.3.4-trusty-amd64"),
		},
	}
	c.Assert(icfg.JujuTools(), gc.Equals, "/path/to/datadir/tools/2.3.4-trusty-amd64")
}

func (*instancecfgSuite) TestGUITools(c *gc.C) {
	icfg := &instancecfg.InstanceConfig{
		DataDir: "/path/to/datadir/",
	}
	c.Assert(icfg.GUITools(), gc.Equals, "/path/to/datadir/gui")
}
