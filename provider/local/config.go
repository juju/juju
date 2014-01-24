// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"os"
	"path/filepath"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/schema"
)

var checkIfRoot = func() bool {
	return os.Getuid() == 0
}

var (
	configFields = schema.Fields{
		"root-dir":            schema.String(),
		"bootstrap-ip":        schema.String(),
		"network-bridge":      schema.String(),
		"container":           schema.String(),
		"storage-port":        schema.ForceInt(),
		"shared-storage-port": schema.ForceInt(),
		"namespace":           schema.String(),
	}
	// The port defaults below are not entirely arbitrary.  Local user web
	// frameworks often use 8000 or 8080, so I didn't want to use either of
	// these, but did want the familiarity of using something in the 8000
	// range.
	configDefaults = schema.Defaults{
		"root-dir":            "",
		"network-bridge":      "lxcbr0",
		"container":           string(instance.LXC),
		"bootstrap-ip":        schema.Omit,
		"storage-port":        8040,
		"shared-storage-port": 8041,
		"namespace":           "",
	}
)

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func newEnvironConfig(config *config.Config, attrs map[string]interface{}) *environConfig {
	return &environConfig{
		Config: config,
		attrs:  attrs,
	}
}

// Since it is technically possible for two different users on one machine to
// have the same local provider name, we need to have a simple way to
// namespace the file locations, but more importantly the containers.
func (c *environConfig) namespace() string {
	return c.attrs["namespace"].(string)
}

func (c *environConfig) rootDir() string {
	return c.attrs["root-dir"].(string)
}

func (c *environConfig) container() instance.ContainerType {
	return instance.ContainerType(c.attrs["container"].(string))
}

func (c *environConfig) networkBridge() string {
	return c.attrs["network-bridge"].(string)
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

func (c *environConfig) logDir() string {
	return filepath.Join(c.rootDir(), "log")
}

// A config is bootstrapped if the bootstrap-ip address has been set.
func (c *environConfig) bootstrapped() bool {
	_, found := c.attrs["bootstrap-ip"]
	return found
}

func (c *environConfig) bootstrapIPAddress() string {
	addr, found := c.attrs["bootstrap-ip"]
	if found {
		return addr.(string)
	}
	return ""
}

func (c *environConfig) storagePort() int {
	return c.attrs["storage-port"].(int)
}

func (c *environConfig) sharedStoragePort() int {
	return c.attrs["shared-storage-port"].(int)
}

func (c *environConfig) storageAddr() string {
	return fmt.Sprintf("%s:%d", c.bootstrapIPAddress(), c.storagePort())
}

func (c *environConfig) sharedStorageAddr() string {
	return fmt.Sprintf("%s:%d", c.bootstrapIPAddress(), c.sharedStoragePort())
}

func (c *environConfig) configFile(filename string) string {
	return filepath.Join(c.rootDir(), filename)
}

func (c *environConfig) createDirs() error {
	for _, dirname := range []string{
		c.sharedStorageDir(),
		c.storageDir(),
		c.mongoDir(),
		c.logDir(),
	} {
		logger.Tracef("creating directory %s", dirname)
		if err := os.MkdirAll(dirname, 0755); err != nil {
			return err
		}
	}
	return nil
}
