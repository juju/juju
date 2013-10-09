// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null

import (
	"fmt"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/schema"
)

var (
	configFields = schema.Fields{
		"bootstrap-host":    schema.String(),
		"bootstrap-user":    schema.String(),
		"storage-listen-ip": schema.String(),
		"storage-port":      schema.Int(),
		"storage-auth-key":  schema.String(),

		// these config values are initialised
		// during Prepare, by connecting to
		// the target machine.
		"bootstrap-series":   schema.String(),
		"bootstrap-hardware": schema.String(),
	}
	configDefaults = schema.Defaults{
		"bootstrap-user":     "",
		"storage-listen-ip":  "",
		"storage-port":       8040,
		"bootstrap-series":   schema.Omit,
		"bootstrap-hardware": schema.Omit,
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

func (c *environConfig) storageListenIPAddress() string {
	return c.attrs["storage-listen-ip"].(string)
}

func (c *environConfig) storagePort() int {
	return int(c.attrs["storage-port"].(int64))
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

// bootstrapSeries returns the detected series of the bootstrap host,
// or "" if it hasn't yet been detected (by Prepare).
func (c *environConfig) bootstrapSeries() string {
	series, _ := c.attrs["bootstrap-series"].(string)
	return series
}

// bootstrapHardware returns the detected hardware characteristics of he
// bootstrap host, or nil if they haven't yet been detected (by Prepare).
func (c *environConfig) bootstrapHardware() (*instance.HardwareCharacteristics, error) {
	hardware, ok := c.attrs["bootstrap-hardware"].(string)
	if !ok {
		return nil, nil
	}
	hc, err := instance.ParseHardware(hardware)
	if err != nil {
		return nil, err
	}
	return &hc, nil
}
