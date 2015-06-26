// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/container/lxc"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
)

var checkIfRoot = func() bool {
	return os.Getuid() == 0
}

// Attribute keys
const (
	BootstrapIpKey   = "bootstrap-ip"
	ContainerKey     = "container"
	NamespaceKey     = "namespace"
	NetworkBridgeKey = "network-bridge"
	RootDirKey       = "root-dir"
	StoragePortKey   = "storage-port"
)

// configSchema defines the schema for the configuration attributes
// defined by the local provider.
var configSchema = environschema.Fields{
	RootDirKey: {
		Description: `The directory that is used for the storage files and database. The default location is $JUJU_HOME/<env-name>. $JUJU_HOME defaults to ~/.juju. Override if needed.`,
		Type:        environschema.Tstring,
		Example:     "~/.juju/local",
	},
	BootstrapIpKey: {
		Description: `The IP address of the bootstrap machine`,
		Type:        environschema.Tstring,
		Group:       environschema.JujuGroup,
	},
	NetworkBridgeKey: {
		Description: `The name of the LXC network bridge to use. Override if the default LXC network bridge is different.`,
		Type:        environschema.Tstring,
	},
	ContainerKey: {
		Description: `The kind of container to use for machines`,
		Type:        environschema.Tstring,
		Values: []interface{}{
			string(instance.LXC),
			string(instance.KVM),
		},
	},
	StoragePortKey: {
		Description: `The port where the local provider starts the HTTP file server. Override the value if you have multiple local providers, or if the default port is used by another program.`,
		Type:        environschema.Tint,
	},
	NamespaceKey: {
		Description: `The name space to use for local provider resources. Override if you have multiple local providers`,
		Type:        environschema.Tstring,
	},
}

var (
	configFields = func() schema.Fields {
		fs, _, err := configSchema.ValidationSchema()
		if err != nil {
			panic(err)
		}
		return fs
	}()
	// The port defaults below are not entirely arbitrary.  Local user web
	// frameworks often use 8000 or 8080, so I didn't want to use either of
	// these, but did want the familiarity of using something in the 8000
	// range.
	configDefaults = schema.Defaults{
		RootDirKey:       "",
		NetworkBridgeKey: "",
		ContainerKey:     string(instance.LXC),
		BootstrapIpKey:   schema.Omit,
		StoragePortKey:   8040,
		NamespaceKey:     "",
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
	return c.attrs[NamespaceKey].(string)
}

func (c *environConfig) rootDir() string {
	return c.attrs[RootDirKey].(string)
}

func (c *environConfig) container() instance.ContainerType {
	return instance.ContainerType(c.attrs[ContainerKey].(string))
}

// setDefaultNetworkBridge sets default network bridge if none is
// provided. Default network bridge varies based on container type.
func (c *environConfig) setDefaultNetworkBridge() {
	name := c.networkBridge()
	switch c.container() {
	case instance.LXC:
		if name == "" {
			name = lxc.DefaultLxcBridge
		}
	case instance.KVM:
		if name == "" {
			name = kvm.DefaultKvmBridge
		}
	}
	c.attrs[NetworkBridgeKey] = name
}

func (c *environConfig) networkBridge() string {
	// We don't care if it's not a string, because Validate takes care
	// of that.
	return c.attrs[NetworkBridgeKey].(string)
}

func (c *environConfig) storageDir() string {
	return filepath.Join(c.rootDir(), "storage")
}

func (c *environConfig) mongoDir() string {
	return filepath.Join(c.rootDir(), "db")
}

func (c *environConfig) logDir() string {
	return fmt.Sprintf("%s-%s", agent.DefaultLogDir, c.namespace())
}

// bootstrapIPAddress returns the IP address of the bootstrap machine.
// As of 1.18 this is only set inside the environment, and not in the
// .jenv file.
func (c *environConfig) bootstrapIPAddress() string {
	addr, _ := c.attrs[BootstrapIpKey].(string)
	return addr
}

func (c *environConfig) stateServerAddr() string {
	return fmt.Sprintf("localhost:%d", c.Config.APIPort())
}

func (c *environConfig) storagePort() int {
	return c.attrs[StoragePortKey].(int)
}

func (c *environConfig) storageAddr() string {
	return fmt.Sprintf("%s:%d", c.bootstrapIPAddress(), c.storagePort())
}

func (c *environConfig) configFile(filename string) string {
	return filepath.Join(c.rootDir(), filename)
}

func (c *environConfig) createDirs() error {
	for _, dirname := range []string{
		c.storageDir(),
		c.mongoDir(),
	} {
		logger.Tracef("creating directory %s", dirname)
		if err := os.MkdirAll(dirname, 0755); err != nil {
			return err
		}
	}
	return nil
}
