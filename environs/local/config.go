// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
)

var configChecker = schema.StrictFieldMap(
	schema.Fields{
		"public-storate":  schema.String(),
		"private-storage": schema.String(),
	},
	schema.Defaults{
		"public-storate":  "",
		"private-storage": "",
	},
)

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (c *environConfig) publicStorageDir() string {
	return c.attrs["public-storate"].(string)
}

func (c *environConfig) privateStorageDir() string {
	return c.attrs["private-storage"].(string)
}
