// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/juju/charm"
	"github.com/juju/schema"
	"github.com/juju/version/v2"
	"gopkg.in/juju/environschema.v1"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/environs/config"
)

var (
	// jujuVersionForControllerStorage is the Juju version which first
	// added the ability to store charm state in the controller.
	jujuVersionForControllerStorage = version.MustParse("2.8.0")
)

// RequireOperatorStorage returns true if the specified min-juju-version
// defined by a charm is such that the charm requires operator storage.
func RequireOperatorStorage(ch charm.CharmMeta) bool {
	if charm.MetaFormat(ch) == charm.FormatV2 {
		return false
	}
	minVers := ch.Meta().MinJujuVersion
	return minVers.Compare(jujuVersionForControllerStorage) < 0
}

var configSchema = environschema.Fields{
	k8sconstants.WorkloadStorageKey: {
		Description: "The preferred storage class used to provision workload storage.",
		Type:        environschema.Tstring,
		Group:       environschema.AccountGroup,
	},
	k8sconstants.OperatorStorageKey: {
		Description: "The storage class used to provision operator storage.",
		Type:        environschema.Tstring,
		Group:       environschema.AccountGroup,
		Immutable:   true,
	},
}

var providerConfigFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

var providerConfigDefaults = schema.Defaults{
	k8sconstants.WorkloadStorageKey: "",
	k8sconstants.OperatorStorageKey: "",
}

type brokerConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (p kubernetesEnvironProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	newCfg, err := validateConfig(cfg, old)
	if err != nil {
		return nil, fmt.Errorf("invalid k8s provider config: %v", err)
	}
	return newCfg.Apply(newCfg.attrs)
}

func (p kubernetesEnvironProvider) newConfig(cfg *config.Config) (*brokerConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &brokerConfig{valid, valid.UnknownAttrs()}, nil
}

// Schema returns the configuration schema for an environment.
func (kubernetesEnvironProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

// ConfigSchema returns extra config attributes specific
// to this provider only.
func (p kubernetesEnvironProvider) ConfigSchema() schema.Fields {
	return providerConfigFields
}

// ConfigDefaults returns the default values for the
// provider specific config attributes.
func (p kubernetesEnvironProvider) ConfigDefaults() schema.Defaults {
	return providerConfigDefaults
}

func validateConfig(cfg, old *config.Config) (*brokerConfig, error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(providerConfigFields, providerConfigDefaults)
	if err != nil {
		return nil, err
	}

	bcfg := &brokerConfig{cfg, validated}
	return bcfg, nil
}
