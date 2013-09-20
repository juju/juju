// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null

import (
	"fmt"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
)

var (
	configFields = schema.Fields{
		"bootstrap-host": schema.String(),
		"bootstrap-user": schema.String(),
		"storage-ip":     schema.String(),
		"storage-dir":    schema.String(),
		"storage-port":   schema.Int(),
	}
	configDefaults = schema.Defaults{
		"bootstrap-host": "",
		"bootstrap-user": "",
		"storage-ip":     "",
		"storage-dir":    "/var/lib/juju/storage",
		"storage-port":   8040,
	}
)

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func newEnvironConfig(config *config.Config, attrs map[string]interface{}) *environConfig {
	return &environConfig{Config: config, attrs: attrs}
}

func (c *environConfig) bootstrapHost() string {
	return c.attrs["bootstrap-host"].(string)
}

func (c *environConfig) bootstrapUser() string {
	return c.attrs["bootstrap-user"].(string)
}

func (c *environConfig) sshHost() string {
	host := c.bootstrapHost()
	if user := c.bootstrapUser(); user != "" {
		host = user + "@" + host
	}
	return host
}

func (c *environConfig) storageDir() string {
	return c.attrs["storage-dir"].(string)
}

func (c *environConfig) storageIPAddress() string {
	return c.attrs["storage-ip"].(string)
}

func (c *environConfig) storagePort() int {
	return int(c.attrs["storage-port"].(int64))
}

// storageAddr returns an address for connecting to the
// bootstrap machine's localstorage.
func (c *environConfig) storageAddr() string {
	return fmt.Sprintf("%s:%d", c.bootstrapHost(), c.storagePort())
}

// storageListenAddr returns an address for the bootstrap
// machine to listen on for its localstorage.
func (c *environConfig) storageListenAddr() string {
	return fmt.Sprintf("%s:%d", c.storageIPAddress(), c.storagePort())
}
