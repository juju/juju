// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"fmt"

	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
)

const defaultStoragePort = 8040

var (
	configFields = schema.Fields{
		"bootstrap-host":    schema.String(),
		"bootstrap-user":    schema.String(),
		"storage-listen-ip": schema.String(),
		"storage-port":      schema.ForceInt(),
		"storage-auth-key":  schema.String(),
		"use-sshstorage":    schema.Bool(),
	}
	configDefaults = schema.Defaults{
		"bootstrap-user":    "",
		"storage-listen-ip": "",
		"storage-port":      defaultStoragePort,
		"use-sshstorage":    true,
	}
)

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func newEnvironConfig(config *config.Config, attrs map[string]interface{}) *environConfig {
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

func (c *environConfig) storageListenIPAddress() string {
	return c.attrs["storage-listen-ip"].(string)
}

func (c *environConfig) storagePort() int {
	switch val := c.attrs["storage-port"].(type) {
	case float64:
		return int(val)
	case int:
		return val
	default:
		panic(fmt.Sprintf("Unexpected %T in storage-port: %#v. Expected float64 or int.", val, val))
	}
}

func (c *environConfig) storageAuthKey() string {
	return c.attrs["storage-auth-key"].(string)
}

// storageAddr returns an address for connecting to the
// bootstrap machine's localstorage.
func (c *environConfig) storageAddr() string {
	return fmt.Sprintf("%s:%d", c.bootstrapHost(), c.storagePort())
}

// storageListenAddr returns an address for the bootstrap
// machine to listen on for its localstorage.
func (c *environConfig) storageListenAddr() string {
	return fmt.Sprintf("%s:%d", c.storageListenIPAddress(), c.storagePort())
}
