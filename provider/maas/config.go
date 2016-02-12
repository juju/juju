// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
)

var configSchema = environschema.Fields{
	"maas-server": {
		Description: "maas-server specifies the location of the MAAS server. It must specify the base path.",
		Type:        environschema.Tstring,
		Example:     "http://192.168.1.1/MAAS/",
	},
	"maas-oauth": {
		Description: "maas-oauth holds the OAuth credentials from MAAS.",
		Type:        environschema.Tstring,
	},
	"maas-agent-name": {
		Description: "maas-agent-name is an optional UUID to group the instances acquired from MAAS, to support multiple models per MAAS user.",
		Type:        environschema.Tstring,
	},
}

var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

var configDefaults = schema.Defaults{
	// For backward-compatibility, maas-agent-name is the empty string
	// by default. However, new environments should all use a UUID.
	"maas-agent-name": "",
}

type maasModelConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (cfg *maasModelConfig) maasServer() string {
	return cfg.attrs["maas-server"].(string)
}

func (cfg *maasModelConfig) maasOAuth() string {
	return cfg.attrs["maas-oauth"].(string)
}

func (cfg *maasModelConfig) maasAgentName() string {
	if uuid, ok := cfg.attrs["maas-agent-name"].(string); ok {
		return uuid
	}
	return ""
}

func (prov maasEnvironProvider) newConfig(cfg *config.Config) (*maasModelConfig, error) {
	validCfg, err := prov.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	result := new(maasModelConfig)
	result.Config = validCfg
	result.attrs = validCfg.UnknownAttrs()
	return result, nil
}

// Schema returns the configuration schema for an environment.
func (maasEnvironProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

var errMalformedMaasOAuth = errors.New("malformed maas-oauth (3 items separated by colons)")

func (prov maasEnvironProvider) Validate(cfg, oldCfg *config.Config) (*config.Config, error) {
	// Validate base configuration change before validating MAAS specifics.
	err := config.Validate(cfg, oldCfg)
	if err != nil {
		return nil, err
	}

	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}

	// Add MAAS specific defaults.
	providerDefaults := make(map[string]interface{})

	// Storage.
	if _, ok := cfg.StorageDefaultBlockSource(); !ok {
		providerDefaults[config.StorageDefaultBlockSourceKey] = maasStorageProviderType
	}
	if len(providerDefaults) > 0 {
		if cfg, err = cfg.Apply(providerDefaults); err != nil {
			return nil, err
		}
	}

	if oldCfg != nil {
		oldAttrs := oldCfg.UnknownAttrs()
		validMaasAgentName := false
		if oldName, ok := oldAttrs["maas-agent-name"]; !ok || oldName == nil {
			// If maas-agent-name was nil (because the config was
			// generated pre-1.16.2 the only correct value for it is ""
			// See bug #1256179
			validMaasAgentName = (validated["maas-agent-name"] == "")
		} else {
			validMaasAgentName = (validated["maas-agent-name"] == oldName)
		}
		if !validMaasAgentName {
			return nil, fmt.Errorf("cannot change maas-agent-name")
		}
	}
	envCfg := new(maasModelConfig)
	envCfg.Config = cfg
	envCfg.attrs = validated
	server := envCfg.maasServer()
	serverURL, err := url.Parse(server)
	if err != nil || serverURL.Scheme == "" || serverURL.Host == "" {
		return nil, fmt.Errorf("malformed maas-server URL '%v': %s", server, err)
	}
	oauth := envCfg.maasOAuth()
	if strings.Count(oauth, ":") != 2 {
		return nil, errMalformedMaasOAuth
	}
	return cfg.Apply(envCfg.attrs)
}
