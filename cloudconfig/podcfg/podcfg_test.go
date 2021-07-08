// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type podcfgSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&podcfgSuite{})

func (*podcfgSuite) TestPodLabelsController(c *gc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{})
	controllerJobs := []model.MachineJob{model.JobManageModel}
	nonControllerJobs := []model.MachineJob{model.JobHostUnits}
	testPodLabels(c, cfg, controllerJobs, map[string]string{
		"juju-model-uuid":      testing.ModelTag.Id(),
		"juju-controller-uuid": testing.ControllerTag.Id(),
		"juju-is-controller":   "true",
	})
	testPodLabels(c, cfg, nonControllerJobs, map[string]string{
		"juju-model-uuid":      testing.ModelTag.Id(),
		"juju-controller-uuid": testing.ControllerTag.Id(),
		"juju-is-controller":   "true",
	})
}

func (*podcfgSuite) TestPodLabelsUserSpecified(c *gc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"resource-tags": "a=b c=",
	})
	testPodLabels(c, cfg, nil, map[string]string{
		"juju-model-uuid":      testing.ModelTag.Id(),
		"juju-controller-uuid": testing.ControllerTag.Id(),
		"a":                    "b",
		"c":                    "",
		"juju-is-controller":   "true",
	})
}

func testPodLabels(c *gc.C, cfg *config.Config, jobs []model.MachineJob, expectTags map[string]string) {
	tags := podcfg.PodLabels(testing.ModelTag.Id(), testing.ControllerTag.Id(), cfg, jobs)
	c.Assert(tags, jc.DeepEquals, expectTags)
}

func (*podcfgSuite) TestOperatorImagesDefaultRepo(c *gc.C) {
	cfg := testing.FakeControllerConfig()
	podConfig, err := podcfg.NewBootstrapControllerPodConfig(
		cfg,
		"controller-1",
		"kubernetes",
		constraints.Value{},
	)
	c.Assert(err, jc.ErrorIsNil)
	podConfig.JujuVersion = version.MustParse("6.6.6")
	podConfig.OfficialBuild = 666
	path, err := podConfig.GetControllerImagePath()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.Equals, "jujusolutions/jujud-operator:6.6.6.666")
	c.Assert(podConfig.GetJujuDbOCIImagePath(), gc.Equals, "jujusolutions/juju-db:4.0")
}

func (*podcfgSuite) TestOperatorImagesCustomRepo(c *gc.C) {
	cfg := testing.FakeControllerConfig()
	cfg["caas-image-repo"] = "path/to/my/repo"
	podConfig, err := podcfg.NewBootstrapControllerPodConfig(
		cfg,
		"controller-1",
		"kubernetes",
		constraints.Value{},
	)
	c.Assert(err, jc.ErrorIsNil)
	podConfig.JujuVersion = version.MustParse("6.6.6")
	podConfig.OfficialBuild = 666
	path, err := podConfig.GetControllerImagePath()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.Equals, "path/to/my/repo/jujud-operator:6.6.6.666")
	c.Assert(podConfig.GetJujuDbOCIImagePath(), gc.Equals, "path/to/my/repo/juju-db:4.0")
}

func (*podcfgSuite) TestBootstrapConstraints(c *gc.C) {
	cfg := testing.FakeControllerConfig()
	cons := constraints.MustParse("mem=4G")
	podConfig, err := podcfg.NewBootstrapControllerPodConfig(
		cfg,
		"controller-1",
		"kubernetes",
		cons,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(podConfig.Bootstrap.BootstrapMachineConstraints, gc.DeepEquals, cons)
}

func (*podcfgSuite) TestFinishControllerPodConfig(c *gc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"type":                      "kubernetes",
		"ssl-hostname-verification": false,
		"juju-https-proxy":          "https-proxy",
	})
	podConfig, err := podcfg.NewBootstrapControllerPodConfig(
		testing.FakeControllerConfig(),
		"controller-1",
		"kubernetes",
		constraints.Value{},
	)
	c.Assert(err, jc.ErrorIsNil)
	podcfg.FinishControllerPodConfig(
		podConfig,
		cfg,
		map[string]string{"foo": "bar"},
	)
	c.Assert(podConfig.DisableSSLHostnameVerification, jc.IsTrue)
	c.Assert(podConfig.ProxySettings.Https, gc.Equals, "https-proxy")
	c.Assert(podConfig.AgentEnvironment, jc.DeepEquals, map[string]string{
		"PROVIDER_TYPE": "kubernetes",
		"foo":           "bar",
	})
}
