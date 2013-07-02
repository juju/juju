// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"os"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
)

var configChecker = schema.StrictFieldMap(
	schema.Fields{
		"shared-storage": schema.String(),
		"storage":        schema.String(),
	},
	schema.Defaults{
		"shared-storage": "",
		"storage":        "",
	},
)

type environConfig struct {
	*config.Config
	user  string
	attrs map[string]interface{}
}

func newEnvironConfig(config *config.Config, attrs map[string]interface{}) *environConfig {
	user := os.Getenv("USER")
	return &environConfig{
		Config: config,
		user:   user,
		attrs:  attrs,
	}
}

// Since it is technically possible for two different users on one machine to
// have the same local provider name, we need to have a simple way to
// namespace the file locations, but more importantly the lxc containers.
func (c *environConfig) namespace() string {
	return fmt.Sprintf("%s-%s", c.user, c.Name())
}

func (c *environConfig) publicStorageDir() string {
	return c.attrs["shared-storage"].(string)
}

func (c *environConfig) privateStorageDir() string {
	return c.attrs["storage"].(string)
}
