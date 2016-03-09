// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
)

var (
	configFields = schema.Fields{
		"bootstrap-host": schema.String(),
		"bootstrap-user": schema.String(),
		// NOTE(axw) use-sshstorage, despite its name, is now used
		// just for determining whether the code is running inside
		// or outside the Juju environment.
		"use-sshstorage": schema.Bool(),
	}
	configDefaults = schema.Defaults{
		"bootstrap-user": "",
		"use-sshstorage": true,
	}
)

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func newModelConfig(config *config.Config, attrs map[string]interface{}) *environConfig {
	return &environConfig{Config: config, attrs: attrs}
}

func (c *environConfig) useSSHStorage() bool {
	// Prior to 1.17.3, the use-sshstorage attribute
	// did not exist. We take non-existence to be
	// equivalent to false.
	useSSHStorage, _ := c.attrs["use-sshstorage"].(bool)
	return useSSHStorage
}

func (c *environConfig) bootstrapHost() string {
	return c.attrs["bootstrap-host"].(string)
}

func (c *environConfig) bootstrapUser() string {
	return c.attrs["bootstrap-user"].(string)
}
