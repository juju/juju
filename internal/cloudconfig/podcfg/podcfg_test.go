// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/testing"
)

type podcfgSuite struct {
	testing.BaseSuite
}

func TestPodcfgSuite(t *stdtesting.T) {
	tc.Run(t, &podcfgSuite{})
}

func (*podcfgSuite) TestPodLabelsController(c *tc.C) {
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

func (*podcfgSuite) TestPodLabelsUserSpecified(c *tc.C) {
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

func testPodLabels(c *tc.C, cfg *config.Config, jobs []model.MachineJob, expectTags map[string]string) {
	tags := podcfg.PodLabels(testing.ModelTag.Id(), testing.ControllerTag.Id(), cfg, jobs)
	c.Assert(tags, tc.DeepEquals, expectTags)
}

func (*podcfgSuite) TestOperatorImagesDefaultRepo(c *tc.C) {
	cfg := testing.FakeControllerConfig()
	cfg["juju-db-snap-channel"] = "9.9/stable"
	podConfig, err := podcfg.NewBootstrapControllerPodConfig(
		cfg,
		"controller-1",
		"ubuntu",
		constraints.Value{},
	)
	c.Assert(err, tc.ErrorIsNil)
	podConfig.JujuVersion = semversion.MustParse("6.6.6.666")
	path, err := podConfig.GetControllerImagePath()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.Equals, "docker.io/jujusolutions/jujud-operator:6.6.6.666")
	path, err = podConfig.GetJujuDbOCIImagePath()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.Equals, "docker.io/jujusolutions/juju-db:9.9")
}

func (*podcfgSuite) TestOperatorImagesCustomRepo(c *tc.C) {
	cfg := testing.FakeControllerConfig()
	cfg["caas-image-repo"] = "path/to/my/repo"
	cfg["juju-db-snap-channel"] = "9.9"
	podConfig, err := podcfg.NewBootstrapControllerPodConfig(
		cfg,
		"controller-1",
		"ubuntu",
		constraints.Value{},
	)
	c.Assert(err, tc.ErrorIsNil)
	podConfig.JujuVersion = semversion.MustParse("6.6.6.666")
	c.Assert(err, tc.ErrorIsNil)
	path, err := podConfig.GetControllerImagePath()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.Equals, "path/to/my/repo/jujud-operator:6.6.6.666")
	path, err = podConfig.GetJujuDbOCIImagePath()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(path, tc.Equals, "path/to/my/repo/juju-db:9.9")
}

func (*podcfgSuite) TestBootstrapConstraints(c *tc.C) {
	cfg := testing.FakeControllerConfig()
	cons := constraints.MustParse("mem=4G")
	podConfig, err := podcfg.NewBootstrapControllerPodConfig(
		cfg,
		"controller-1",
		"ubuntu",
		cons,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(podConfig.Bootstrap.BootstrapMachineConstraints, tc.DeepEquals, cons)
}

func (*podcfgSuite) TestFinishControllerPodConfig(c *tc.C) {
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"type":                      "kubernetes",
		"ssl-hostname-verification": false,
		"juju-https-proxy":          "https-proxy",
	})
	podConfig, err := podcfg.NewBootstrapControllerPodConfig(
		testing.FakeControllerConfig(),
		"controller-1",
		"ubuntu",
		constraints.Value{},
	)
	c.Assert(err, tc.ErrorIsNil)
	podcfg.FinishControllerPodConfig(
		podConfig,
		cfg,
		map[string]string{"foo": "bar"},
	)
	c.Assert(podConfig.DisableSSLHostnameVerification, tc.IsTrue)
	c.Assert(podConfig.ProxySettings.Https, tc.Equals, "https-proxy")
	c.Assert(podConfig.AgentEnvironment, tc.DeepEquals, map[string]string{
		"PROVIDER_TYPE": "kubernetes",
		"foo":           "bar",
	})
}

func (*podcfgSuite) TestUnitAgentConfig(c *tc.C) {
	cfg := testing.FakeControllerConfig()
	podConfig, err := podcfg.NewBootstrapControllerPodConfig(
		cfg,
		"controller-1",
		"ubuntu",
		constraints.Value{},
	)
	podConfig.APIInfo = &api.Info{
		ModelTag: testing.ModelTag,
		CACert:   testing.CACert,
	}
	podConfig.Bootstrap.StateServingInfo.APIPort = 1234
	podConfig.JujuVersion = semversion.MustParse("6.6.6")
	c.Assert(err, tc.ErrorIsNil)
	agentCfg, err := podConfig.UnitAgentConfig()
	c.Assert(err, tc.ErrorIsNil)
	apiInfo, ok := agentCfg.APIInfo()
	c.Assert(ok, tc.IsTrue)
	c.Assert(agentCfg.OldPassword(), tc.Equals, apiInfo.Password)
	c.Assert(apiInfo.Addrs, tc.DeepEquals, []string{"localhost:1234"})
}
