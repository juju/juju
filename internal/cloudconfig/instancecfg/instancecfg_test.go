// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancecfg_test

import (
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
)

type instancecfgSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&instancecfgSuite{})

func (*instancecfgSuite) TestIsController(c *gc.C) {
	cfg := instancecfg.InstanceConfig{}
	c.Assert(cfg.IsController(), jc.IsFalse)
	cfg.Jobs = []model.MachineJob{model.JobManageModel}
	c.Assert(cfg.IsController(), jc.IsTrue)
}

func (*instancecfgSuite) TestInstanceTagsController(c *gc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{})
	testInstanceTags(c, cfg, true, map[string]string{
		"juju-model-uuid":      testing.ModelTag.Id(),
		"juju-controller-uuid": testing.ControllerTag.Id(),
		"juju-is-controller":   "true",
	})
	testInstanceTags(c, cfg, false, map[string]string{
		"juju-model-uuid":      testing.ModelTag.Id(),
		"juju-controller-uuid": testing.ControllerTag.Id(),
	})
}

func (*instancecfgSuite) TestInstanceTagsUserSpecified(c *gc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"resource-tags": "a=b c=",
	})
	testInstanceTags(c, cfg, false, map[string]string{
		"juju-model-uuid":      testing.ModelTag.Id(),
		"juju-controller-uuid": testing.ControllerTag.Id(),
		"a":                    "b",
		"c":                    "",
	})
}

func testInstanceTags(c *gc.C, cfg *config.Config, isController bool, expectTags map[string]string) {
	tags := instancecfg.InstanceTags(testing.ModelTag.Id(), testing.ControllerTag.Id(), cfg, isController)
	c.Assert(tags, jc.DeepEquals, expectTags)
}

func (*instancecfgSuite) TestAgentVersionZero(c *gc.C) {
	var icfg instancecfg.InstanceConfig
	c.Assert(icfg.AgentVersion(), gc.Equals, semversion.Binary{})
}

func (*instancecfgSuite) TestAgentVersion(c *gc.C) {
	var icfg instancecfg.InstanceConfig
	list := coretools.List{
		&coretools.Tools{Version: semversion.MustParseBinary("2.3.4-ubuntu-amd64")},
	}
	err := icfg.SetTools(list)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(icfg.AgentVersion(), gc.Equals, list[0].Version)
}

func (*instancecfgSuite) TestSetToolsSameVersions(c *gc.C) {
	var icfg instancecfg.InstanceConfig
	list := coretools.List{
		&coretools.Tools{Version: semversion.MustParseBinary("2.3.4-ubuntu-amd64")},
		&coretools.Tools{Version: semversion.MustParseBinary("2.3.4-ubuntu-amd64")},
	}
	err := icfg.SetTools(list)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(icfg.ToolsList(), jc.DeepEquals, list)
}

func (*instancecfgSuite) TestSetToolsDifferentVersions(c *gc.C) {
	var icfg instancecfg.InstanceConfig
	list := coretools.List{
		&coretools.Tools{Version: semversion.MustParseBinary("2.3.4-ubuntu-amd64")},
		&coretools.Tools{Version: semversion.MustParseBinary("2.3.5-ubuntu-amd64")},
	}
	err := icfg.SetTools(list)
	c.Assert(err, gc.ErrorMatches, `agent binary info mismatch.*2\.3\.4.*2\.3\.5.*`)
	c.Assert(icfg.ToolsList(), gc.HasLen, 0)
}

func (*instancecfgSuite) TestJujuTools(c *gc.C) {
	icfg := &instancecfg.InstanceConfig{
		DataDir: "/path/to/datadir/",
	}
	err := icfg.SetTools(coretools.List{
		&coretools.Tools{
			Version: semversion.MustParseBinary("2.3.4-ubuntu-amd64"),
			URL:     "/tools/2.3.4-ubuntu-amd64",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(icfg.JujuTools(), gc.Equals, "/path/to/datadir/tools/2.3.4-ubuntu-amd64")
}

func (*instancecfgSuite) TestCharmDir(c *gc.C) {
	icfg := &instancecfg.InstanceConfig{
		DataDir: "/path/to/datadir/",
	}
	c.Assert(icfg.CharmDir(), gc.Equals, "/path/to/datadir/charms")
}

func (*instancecfgSuite) TestAgentConfigLogParams(c *gc.C) {
	icfg := instancecfg.InstanceConfig{
		APIInfo: &api.Info{
			Addrs:    []string{"1.2.3.4:4321"},
			CACert:   "cert",
			ModelTag: names.NewModelTag(testing.ModelTag.Id()),
			Password: "secret123",
		},
		ControllerConfig: controller.Config{
			"agent-logfile-max-size":    "123MB",
			"agent-logfile-max-backups": 7,
		},
		ControllerTag: names.NewControllerTag(testing.ControllerTag.Id()),
		DataDir:       "/path/to/datadir/",
	}
	config, err := icfg.AgentConfig(names.NewMachineTag("foo"), semversion.MustParse("1.2.3"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(config.AgentLogfileMaxSizeMB(), gc.Equals, 123)
	c.Assert(config.AgentLogfileMaxBackups(), gc.Equals, 7)
}
