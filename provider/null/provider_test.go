// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/instance"
	_ "launchpad.net/juju-core/provider/null"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type providerSuite struct {
	testbase.LoggingSuite
	provider environs.EnvironProvider
}

var _ = gc.Suite(&providerSuite{})

func minimalConfigValues() map[string]interface{} {
	return map[string]interface{}{
		"name":             "test",
		"type":             "null",
		"bootstrap-host":   "hostname",
		"storage-auth-key": "whatever",
		// While the ca-cert bits aren't entirely minimal, they avoid the need
		// to set up a fake home.
		"ca-cert":        coretesting.CACert,
		"ca-private-key": coretesting.CAKey,
	}
}

func minimalConfig(c *gc.C) *config.Config {
	minimal := minimalConfigValues()
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, gc.IsNil)
	return testConfig
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	var err error
	s.provider, err = environs.Provider("null")
	c.Assert(err, gc.IsNil)

	// install a fake ssh to respond to the
	// series/hardware detection script.
	restore := manual.InstallDetectionFakeSSH(c, "edgy", "i386")
	s.AddCleanup(func(*gc.C) { restore() })
}

func (s *providerSuite) TestDetectBootstrapSeries(c *gc.C) {
	env, err := s.provider.Prepare(minimalConfig(c))
	c.Assert(err, gc.IsNil)

	cfg := env.Config()
	attrs := cfg.UnknownAttrs()
	c.Assert(attrs["bootstrap-series"], gc.Equals, "edgy")
	hc, err := instance.ParseHardware(attrs["bootstrap-hardware"].(string))
	c.Assert(err, gc.IsNil)
	c.Assert(hc.Arch, gc.NotNil)
	c.Assert(*hc.Arch, gc.Equals, "i386")

	// Now for the important bit: default-series should
	// be the same as bootstrap-series, so we get the
	// appropriate tools for bootstrapping.
	c.Assert(cfg.DefaultSeries(), gc.Equals, attrs["bootstrap-series"])
}

func (s *providerSuite) TestBootstrapSeriesSpecified(c *gc.C) {
	// If bootstrap-series is specified ahead of time, use that
	// and update default-series. bootstrap-hardware will be
	// deferred to the actual Bootstrap.
	cfg, err := minimalConfig(c).Apply(map[string]interface{}{
		"bootstrap-series": "raring",
	})
	c.Assert(err, gc.IsNil)

	env, err := s.provider.Prepare(cfg)
	c.Assert(err, gc.IsNil)

	cfg = env.Config()
	attrs := cfg.UnknownAttrs()
	c.Assert(attrs["bootstrap-series"], gc.Equals, "raring")
	c.Assert(attrs["bootstrap-hardware"], gc.IsNil) // detection deferrred

	// Now for the important bit: default-series should
	// be the same as bootstrap-series, so we get the
	// appropriate tools for bootstrapping.
	c.Assert(cfg.DefaultSeries(), gc.Equals, attrs["bootstrap-series"])
}

func (s *providerSuite) TestBootstrapSeriesPrepared(c *gc.C) {
	// bootstrap-series is only detected in Prepare, not Open,
	// as it's only required during bootstrapping.
	env, err := s.provider.Open(minimalConfig(c))
	c.Assert(err, gc.IsNil)
	cfg := env.Config()
	attrs := cfg.UnknownAttrs()
	c.Assert(attrs["bootstrap-series"], gc.IsNil)
}
