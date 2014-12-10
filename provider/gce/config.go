// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"
	"os"

	"github.com/juju/juju/environs/config"
)

const (
	gcePrivateKey  = "GCE_PRIVATE_KEY"
	gceClientId    = "GCE_CLIENT_ID"
	gceClientEmail = "GCE_CLIENT_EMAIL"

	cfgPrivateKey  = "private-key"
	cfgClientId    = "client-id"
	cfgClientEmail = "client-email"
)

// boilerplateConfig will be shown in help output, so please keep it up to
// date when you change environment configuration below.
var boilerplateConfig = `
gce:
  type: gce

  # Google Auth Info
  private-key: 
  client-email:
  client-id:
`[1:]

type googleAuth struct {
	PrivateKey  string
	ClientEmail string
	ClientId    string
}

func validateConfig(cfg *config.Config, old *environConfig) (*environConfig, error) {
	// TODO(ericsnow) call config.Validate and cfg.ValidateUnknownAttrs?

	// Check sanity of juju-level fields.
	var oldCfg *config.Config
	if old != nil {
		oldCfg = old.Config
	}
	if err := config.Validate(cfg, oldCfg); err != nil {
		return nil, err
	}

	auth := googleAuth{
		PrivateKey:  os.Getenv(gcePrivateKey),
		ClientEmail: os.Getenv(gceClientEmail),
		ClientId:    os.Getenv(gceClientId),
	}

	attrs := cfg.UnknownAttrs()
	if err := setAttr(&auth.PrivateKey, attrs, cfgPrivateKey); err != nil {
		return nil, err
	}
	if err := setAttr(&auth.ClientId, attrs, cfgClientId); err != nil {
		return nil, err
	}
	if err := setAttr(&auth.ClientId, attrs, cfgClientEmail); err != nil {
		return nil, err
	}

	if auth.PrivateKey == "" {
		return nil, configError(cfgPrivateKey, gcePrivateKey)
	}
	if auth.ClientEmail == "" {
		return nil, configError(cfgClientEmail, gceClientEmail)
	}
	if auth.ClientId == "" {
		return nil, configError(cfgClientId, gceClientId)
	}

	return &environConfig{cfg, auth, attrs}, nil
}

func setAttr(val *string, attrs map[string]interface{}, field string) error {
	if i, ok := attrs[field]; ok {
		if s, ok := i.(string); ok {
			*val = s
		} else {
			return fmt.Errorf("expected %q to be a string, got %T", field, i)
		}
	}
	return nil
}

type environConfig struct {
	*config.Config
	googleAuth
	attrs map[string]interface{}
}

func configError(yaml, env string) error {
	return fmt.Errorf("missing gce config value %q in environments.yaml (or specified by the environment variable %q)", yaml, env)
}
