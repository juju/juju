// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

// TODO(ericsnow) This file needs a once-over to verify correctness.

import (
	"net/mail"

	"github.com/juju/errors"
	"github.com/juju/schema"

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

var configFields = schema.Fields{
	cfgPrivateKey:  schema.String(),
	cfgClientId:    schema.String(),
	cfgClientEmail: schema.String(),
}

var configDefaults = schema.Defaults{}

var configSecretFields = []string{
	cfgPrivateKey,
}

var configImmutableFields = []string{
	// TODO(ericsnow) Do these really belong here?
	cfgPrivateKey,
	cfgClientId,
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (c *environConfig) privateKey() string {
	return c.attrs[cfgPrivateKey].(string)
}

func (c *environConfig) clientID() string {
	return c.attrs[cfgClientId].(string)
}

func (c *environConfig) clientEmail() string {
	return c.attrs[cfgClientEmail].(string)
}

func validateConfig(cfg, old *config.Config) (*environConfig, error) {
	// Check for valid changes and coerce the values (base config first
	// then custom).
	if err := config.Validate(cfg, old); err != nil {
		return nil, errors.Trace(err)
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ecfg := &environConfig{cfg, validated}

	// TODO(ericsnow) Support pulling ID/PK from shell environment variables.

	// Check that no immutable fields have changed.
	if old != nil {
		attrs := old.UnknownAttrs()
		for _, field := range configImmutableFields {
			if attrs[field] != validated[field] {
				return nil, errors.Errorf("%s: cannot change from %v to %v", field, attrs[field], validated[field])
			}
		}
	}

	// Check sanity of GCE fields.
	if ecfg.privateKey() == "" {
		return nil, errors.Errorf("%s: must not be empty", cfgPrivateKey)
	}
	if ecfg.clientID() == "" {
		return nil, errors.Errorf("%s: must not be empty", cfgClientId)
	}
	if ecfg.clientEmail() == "" {
		return nil, errors.Errorf("%s: must not be empty", cfgClientEmail)
	} else {
		if _, err := mail.ParseAddress(ecfg.clientEmail()); err != nil {
			return nil, errors.Annotatef(err, "invalid %q in config", cfgClientEmail)
		}
	}

	return ecfg, nil
}
