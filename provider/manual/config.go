// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
)

var (
	configFields   = schema.Fields{}
	configDefaults = schema.Defaults{}
)

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func newModelConfig(config *config.Config, attrs map[string]interface{}) *environConfig {
	return &environConfig{Config: config, attrs: attrs}
}
