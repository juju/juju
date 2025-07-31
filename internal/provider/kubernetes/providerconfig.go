// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"

	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
)

var configSchema = configschema.Fields{}

var providerConfigFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

var providerConfigDefaults = schema.Defaults{}

type brokerConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (p kubernetesEnvironProvider) Validate(ctx context.Context, cfg, old *config.Config) (*config.Config, error) {
	newCfg, err := validateConfig(ctx, cfg, old)
	if err != nil {
		return nil, fmt.Errorf("invalid k8s provider config: %v", err)
	}
	return newCfg.Apply(newCfg.attrs)
}

func (p kubernetesEnvironProvider) newConfig(ctx context.Context, cfg *config.Config) (*brokerConfig, error) {
	valid, err := p.Validate(ctx, cfg, nil)
	if err != nil {
		return nil, err
	}
	return &brokerConfig{valid, valid.UnknownAttrs()}, nil
}

// Schema returns the configuration schema for an environment.
func (kubernetesEnvironProvider) Schema() configschema.Fields {
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

// ModelConfigDefaults provides a set of default model config attributes that
// should be set on a models config if they have not been specified by the user.
func (p kubernetesEnvironProvider) ModelConfigDefaults(_ context.Context) (map[string]any, error) {
	return map[string]any{
		config.StorageDefaultFilesystemSourceKey: constants.StorageProviderType,
	}, nil
}

func validateConfig(ctx context.Context, cfg, old *config.Config) (*brokerConfig, error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(ctx, cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(providerConfigFields, providerConfigDefaults)
	if err != nil {
		return nil, err
	}

	bcfg := &brokerConfig{cfg, validated}
	return bcfg, nil
}
