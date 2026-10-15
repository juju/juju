// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package podcfg_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
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
	podConfig.Bootstrap.ControllerAgentInfo.APIPort = 1234
	podConfig.JujuVersion = semversion.MustParse("6.6.6")
	c.Assert(err, tc.ErrorIsNil)
	agentCfg, err := podConfig.UnitAgentConfig()
	c.Assert(err, tc.ErrorIsNil)
	apiInfo, ok := agentCfg.APIInfo()
	c.Assert(ok, tc.IsTrue)
	c.Assert(agentCfg.OldPassword(), tc.Equals, apiInfo.Password)
	c.Assert(apiInfo.Addrs, tc.DeepEquals, []string{"localhost:1234"})
}

// TestControllerPodMachineAgentConfig proves the rendered controller pod
// machine agent config carries only connectivity state: the controller
// tag, explicit API addresses, the trust anchor, and the supplied
// controller-node credential. No serving info and no Dqlite settings are
// rendered.
func (*podcfgSuite) TestControllerPodMachineAgentConfig(c *tc.C) {
	cfg := testing.FakeControllerConfig()
	podConfig, err := podcfg.NewBootstrapControllerPodConfig(
		cfg,
		"controller-1",
		"ubuntu",
		constraints.Value{},
	)
	c.Assert(err, tc.ErrorIsNil)
	podConfig.APIInfo = &api.Info{
		ModelTag: testing.ModelTag,
		CACert:   testing.CACert,
		Addrs:    []string{"10.6.6.6:17070"},
		Password: "admin-secret",
	}
	podConfig.Bootstrap.ControllerAgentInfo.APIPort = 1234
	podConfig.JujuVersion = semversion.MustParse("6.6.6")

	agentCfg, err := podConfig.AgentConfig(
		names.NewControllerAgentTag("0"), "controller-node-credential",
	)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(agentCfg.Tag(), tc.DeepEquals, names.NewControllerAgentTag("0"))
	_, isController := agentCfg.ControllerAgentInfo()
	c.Check(isController, tc.IsFalse)
	c.Check(agentCfg.OldPassword(), tc.Equals, "controller-node-credential")

	rendered, err := agentCfg.Render()
	c.Assert(err, tc.ErrorIsNil)
	content := string(rendered)
	c.Check(content, tc.Not(tc.Contains), "controllercert")
	c.Check(content, tc.Not(tc.Contains), "controllerkey")
	c.Check(content, tc.Not(tc.Contains), "caprivatekey")
	c.Check(content, tc.Not(tc.Contains), "statepassword")
	c.Check(content, tc.Not(tc.Contains), "querytracing")
	c.Check(content, tc.Not(tc.Contains), "dqlitebusytimeout")
	c.Check(content, tc.Not(tc.Contains), "dqlite-port")
	c.Check(content, tc.Contains, "apiaddresses:")
	c.Check(content, tc.Contains, "cacert:")

	apiInfo, ok := agentCfg.APIInfo()
	c.Assert(ok, tc.IsTrue)
	c.Check(apiInfo.Addrs, tc.DeepEquals, []string{
		"localhost:1234", "10.6.6.6:17070",
	})
}
