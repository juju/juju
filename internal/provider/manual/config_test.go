// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	stdtesting "testing"

	"github.com/juju/tc"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
)

type configSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

func TestConfigSuite(t *stdtesting.T) {
	tc.Run(t, &configSuite{})
}

func CloudSpec() environscloudspec.CloudSpec {
	return environscloudspec.CloudSpec{
		Name:     "manual",
		Type:     "manual",
		Endpoint: "hostname",
	}
}

func MinimalConfigValues() map[string]interface{} {
	return map[string]interface{}{
		"name":            "test",
		"type":            "manual",
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
