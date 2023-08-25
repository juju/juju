// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault

import (
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/internal/secrets/provider"
)

const (
	EndpointKey      = "endpoint"
	NamespaceKey     = "namespace"
	TokenKey         = "token"
	CACertKey        = "ca-cert"
	ClientCertKey    = "client-cert"
	ClientKeyKey     = "client-key"
	TLSServerNameKey = "tls-server-name"
)

var configSchema = environschema.Fields{
	EndpointKey: {
		Description: "The vault service endpoint.",
		Type:        environschema.Tstring,
		Immutable:   true,
		Mandatory:   true,
	},
	TokenKey: {
		Description: "The vault access token.",
		Type:        environschema.Tstring,
		Secret:      true,
	},
	NamespaceKey: {
		Description: "The namespace in which to store secrets.",
		Type:        environschema.Tstring,
	},
	CACertKey: {
		Description: "The vault CA certificate.",
		Type:        environschema.Tstring,
	},
	ClientCertKey: {
		Description: "The vault client certificate.",
		Type:        environschema.Tstring,
	},
	ClientKeyKey: {
		Description: "The vault client certificate key.",
		Type:        environschema.Tstring,
		Secret:      true,
	},
	TLSServerNameKey: {
		Description: "The vault TLS server name.",
		Type:        environschema.Tstring,
	},
}

var configDefaults = schema.Defaults{}

type backendConfig struct {
	validAttrs map[string]interface{}
}

func (c *backendConfig) endpoint() string {
	return c.validAttrs[EndpointKey].(string)
}

func (c *backendConfig) namespace() string {
	v, _ := c.validAttrs[NamespaceKey].(string)
	return v
}

func (c *backendConfig) token() string {
	v, _ := c.validAttrs[TokenKey].(string)
	return v
}

func (c *backendConfig) clientCert() string {
	v, _ := c.validAttrs[ClientCertKey].(string)
	return v
}

func (c *backendConfig) clientKey() string {
	v, _ := c.validAttrs[ClientKeyKey].(string)
	return v
}

func (c *backendConfig) caCert() string {
	v, _ := c.validAttrs[CACertKey].(string)
	return v
}

func (c *backendConfig) tlsServerName() string {
	v, _ := c.validAttrs[TLSServerNameKey].(string)
	return v
}

// ConfigSchema implements SecretBackendProvider.
func (p vaultProvider) ConfigSchema() environschema.Fields {
	return configSchema
}

// ConfigDefaults implements SecretBackendProvider.
func (p vaultProvider) ConfigDefaults() schema.Defaults {
	return schema.Defaults{}
}

// ValidateConfig implements SecretBackendProvider.
func (p vaultProvider) ValidateConfig(oldCfg, newCfg provider.ConfigAttrs) error {
	newValidCfg, err := newConfig(newCfg)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = url.Parse(newValidCfg.endpoint())
	if err != nil {
		return errors.Trace(err)
	}

	clientCert := newValidCfg.clientCert()
	clientKey := newValidCfg.clientKey()
	if clientCert != "" && clientKey == "" {
		return errors.NotValidf("vault config missing client key")
	}
	if clientCert == "" && clientKey != "" {
		return errors.NotValidf("vault config missing client certificate")
	}

	if oldCfg == nil {
		return nil
	}
	oldValidCfg, err := newConfig(oldCfg)
	if err != nil {
		return errors.Trace(err)
	}
	for n, field := range configSchema {
		if !field.Immutable {
			continue
		}
		oldV := oldValidCfg.validAttrs[n]
		newV := newValidCfg.validAttrs[n]
		if oldV != newV {
			return errors.Errorf("cannot change immutable field %q", n)
		}
	}
	return nil
}

func newConfig(attrs map[string]interface{}) (*backendConfig, error) {
	cfg, err := coreconfig.NewConfig(attrs, configSchema, configDefaults)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &backendConfig{cfg.Attributes()}, nil
}
