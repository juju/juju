// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"fmt"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/secrets/provider"
)

const (
	endpointKey               = "endpoint"
	namespaceKey              = "namespace"
	serviceAccountKey         = "service-account"
	tokenKey                  = "token"
	usernameKey               = "username"
	passwordKey               = "password"
	caCertsKey                = "ca-certs"
	caCertKey                 = "ca-cert"
	clientCertKey             = "client-cert"
	clientKeyKey              = "client-key"
	skipTLSVerifyKey          = "skip-tls-verify"
	preferInClusterAddressKey = "prefer-incluster-address"
)

var configSchema = environschema.Fields{
	endpointKey: {
		Description: "The k8s api endpoint.",
		Type:        environschema.Tstring,
		Immutable:   true,
		Mandatory:   true,
	},
	namespaceKey: {
		Description: "The namespace in which to store secrets.",
		Type:        environschema.Tstring,
		Immutable:   true,
		Mandatory:   true,
	},
	serviceAccountKey: {
		Description: "The k8s access token service account.",
		Type:        environschema.Tstring,
		Mandatory:   true,
	},
	caCertsKey: {
		Description: "The k8s CA certificate(s).",
		Type:        environschema.Tlist,
	},
	caCertKey: {
		Description: "The k8s CA certificate.",
		Type:        environschema.Tstring,
	},
	tokenKey: {
		Description: "The k8s access token.",
		Type:        environschema.Tstring,
		Secret:      true,
	},
	usernameKey: {
		Description: "The k8s access username.",
		Type:        environschema.Tstring,
	},
	passwordKey: {
		Description: "The k8s access password.",
		Type:        environschema.Tstring,
		Secret:      true,
	},
	clientCertKey: {
		Description: "The k8s client certificate.",
		Type:        environschema.Tstring,
	},
	clientKeyKey: {
		Description: "The k8s client certificate key.",
		Type:        environschema.Tstring,
		Secret:      true,
	},
	skipTLSVerifyKey: {
		Description: "Do not verify the TLS certificate.",
		Type:        environschema.Tbool,
	},
	preferInClusterAddressKey: {
		Description: "Should the in cluster address be preferred.",
		Type:        environschema.Tbool,
	},
}

var configDefaults = schema.Defaults{
	usernameKey:               schema.Omit,
	passwordKey:               schema.Omit,
	serviceAccountKey:         "default",
	tokenKey:                  schema.Omit,
	caCertsKey:                schema.Omit,
	caCertKey:                 schema.Omit,
	clientCertKey:             schema.Omit,
	clientKeyKey:              schema.Omit,
	skipTLSVerifyKey:          schema.Omit,
	preferInClusterAddressKey: schema.Omit,
}

type backendConfig struct {
	validAttrs map[string]interface{}
}

func (c *backendConfig) endpoint() string {
	return c.validAttrs[endpointKey].(string)
}

func (c *backendConfig) namespace() string {
	v, _ := c.validAttrs[namespaceKey].(string)
	return v
}

func (c *backendConfig) token() string {
	v, _ := c.validAttrs[tokenKey].(string)
	return v
}

func (c *backendConfig) serviceAccount() string {
	v, _ := c.validAttrs[serviceAccountKey].(string)
	return v
}

func (c *backendConfig) username() string {
	v, _ := c.validAttrs[usernameKey].(string)
	return v
}

func (c *backendConfig) password() string {
	v, _ := c.validAttrs[passwordKey].(string)
	return v
}

func (c *backendConfig) clientCert() string {
	v, _ := c.validAttrs[clientCertKey].(string)
	return v
}

func (c *backendConfig) clientKey() string {
	v, _ := c.validAttrs[clientKeyKey].(string)
	return v
}

func (c *backendConfig) caCerts() []string {
	v, ok := c.validAttrs[caCertsKey].([]string)
	if ok {
		return v
	}
	certs, ok := c.validAttrs[caCertsKey].([]interface{})
	if ok {
		cacerts := make([]string, len(certs))
		for i, cert := range certs {
			cacerts[i] = fmt.Sprintf("%s", cert)
		}
		return cacerts
	}
	cert, ok := c.validAttrs[caCertKey].(string)
	if ok {
		return []string{cert}
	}

	return nil
}

func (c *backendConfig) skipTLSVerify() bool {
	v, _ := c.validAttrs[skipTLSVerifyKey].(bool)
	return v
}

func (c *backendConfig) preferInClusterAddress() bool {
	v, _ := c.validAttrs[preferInClusterAddressKey].(bool)
	return v
}

// ConfigSchema implements SecretBackendProvider.
func (p k8sProvider) ConfigSchema() environschema.Fields {
	return configSchema
}

// ConfigDefaults implements SecretBackendProvider.
func (p k8sProvider) ConfigDefaults() schema.Defaults {
	return schema.Defaults{}
}

// ValidateConfig implements SecretBackendProvider.
func (p k8sProvider) ValidateConfig(oldCfg, newCfg provider.ConfigAttrs) error {
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
		return errors.NotValidf("k8s config missing client key")
	}
	if clientCert == "" && clientKey != "" {
		return errors.NotValidf("k8s config missing client certificate")
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
