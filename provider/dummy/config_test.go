// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy_test

import (
	stdcontext "context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/environs"
	"github.com/juju/juju/v2/environs/bootstrap"
	"github.com/juju/juju/v2/environs/config"
	"github.com/juju/juju/v2/environs/context"
	envtesting "github.com/juju/juju/v2/environs/testing"
	"github.com/juju/juju/v2/jujuclient"
	"github.com/juju/juju/v2/provider/dummy"
	"github.com/juju/juju/v2/testing"
)

var _ = gc.Suite(&ConfigSuite{})

type ConfigSuite struct {
	testing.BaseSuite
}

func (s *ConfigSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	dummy.Reset(c)
}

var firewallModeTests = []struct {
	configFirewallMode string
	firewallMode       string
	errorMsg           string
}{
	{
		// Empty value leads to default value.
		firewallMode: config.FwInstance,
	}, {
		// Explicit default value.
		configFirewallMode: "",
		firewallMode:       config.FwInstance,
	}, {
		// Instance mode.
		configFirewallMode: "instance",
		firewallMode:       config.FwInstance,
	}, {
		// Global mode.
		configFirewallMode: "global",
		firewallMode:       config.FwGlobal,
	}, {
		// Invalid mode.
		configFirewallMode: "invalid",
		errorMsg:           `firewall-mode: expected one of \[instance global none], got "invalid"`,
	},
}

func (s *ConfigSuite) TestFirewallMode(c *gc.C) {
	for i, test := range firewallModeTests {
		c.Logf("test %d: %s", i, test.configFirewallMode)
		attrs := dummy.SampleConfig()
		if test.configFirewallMode != "" {
			attrs = attrs.Merge(testing.Attrs{
				"firewall-mode": test.configFirewallMode,
			})
		}
		cfg, err := config.New(config.NoDefaults, attrs)
		if err != nil {
			c.Assert(err, gc.ErrorMatches, test.errorMsg)
			continue
		}
		ctx := envtesting.BootstrapContext(stdcontext.TODO(), c)
		e, err := bootstrap.PrepareController(
			false,
			ctx, jujuclient.NewMemStore(),
			bootstrap.PrepareParams{
				ControllerConfig: testing.FakeControllerConfig(),
				ControllerName:   cfg.Name(),
				ModelConfig:      cfg.AllAttrs(),
				Cloud:            dummy.SampleCloudSpec(),
				AdminSecret:      AdminSecret,
			},
		)
		if test.errorMsg != "" {
			c.Assert(err, gc.ErrorMatches, test.errorMsg)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		env := e.(environs.Environ)
		defer env.Destroy(context.NewEmptyCloudCallContext())

		firewallMode := env.Config().FirewallMode()
		c.Assert(firewallMode, gc.Equals, test.firewallMode)

		s.TearDownTest(c)
	}
}
