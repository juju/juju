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
		"root-dir": schema.String(),
	},
	schema.Defaults{
		"root-dir": "",
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

func (c *environConfig) rootDir() string {
	return c.attrs["root-dir"].(string)
}

func (c *environConfig) sharedStorageDir() string {
	return filepath.Join(c.rootDir(), "shared-storage")
}

func (c *environConfig) storageDir() string {
	return filepath.Join(c.rootDir(), "storage")
}

func (c *environConfig) mongoDir() string {
	return filepath.Join(c.rootDir(), "db")
}
