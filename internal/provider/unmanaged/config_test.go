// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unmanaged

import (
	"testing"

	"github.com/juju/tc"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
)

type configSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

func TestConfigSuite(t *testing.T) {
	tc.Run(t, &configSuite{})
}

func CloudSpec() environscloudspec.CloudSpec {
	return environscloudspec.CloudSpec{
		Name:     "unmanaged",
		Type:     "unmanaged",
		Endpoint: "hostname",
	}
}

func MinimalConfigValues() map[string]any {
	return map[string]any{
		"name":            "test",
		"type":            "unmanaged",
		"uuid":            coretesting.ModelTag.Id(),
		"controller-uuid": coretesting.ControllerTag.Id(),
		"firewall-mode":   "instance",
		"secret-backend":  "auto",
		// While the ca-cert bits aren't entirely minimal, they avoid the need
		// to set up a fake home.
		"ca-cert":        coretesting.CACert,
		"ca-private-key": coretesting.CAKey,
	}
}

func MinimalConfig(c *tc.C) *config.Config {
	minimal := MinimalConfigValues()
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, tc.ErrorIsNil)
	return testConfig
}

// TestSchema verifies that Schema() returns all standard model config fields,
// satisfying the ModelConfigProvider interface used by ProviderModelConfigGetter.
func (s *configSuite) TestSchema(c *tc.C) {
	fields := UnmanagedProvider{}.Schema()

	globalFields, err := config.Schema(nil)
	c.Assert(err, tc.ErrorIsNil)
	for name, field := range globalFields {
		c.Check(fields[name], tc.DeepEquals, field)
	}
}
