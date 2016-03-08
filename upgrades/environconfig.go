// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// environConfigUpdater is an interface used atomically write environment
// config changes to the global state.
type environConfigUpdater interface {
	// UpdateModelConfig atomically updates and removes environment
	// config attributes to the global state.
	UpdateModelConfig(map[string]interface{}, []string, state.ValidateConfigFunc) error
}

// environConfigReader is an interface used to read the current environment
// config from global state.
type environConfigReader interface {
	// ModelConfig reads the current environment config from global
	// state.
	ModelConfig() (*config.Config, error)
}

func upgradeModelConfig(
	reader environConfigReader,
	updater environConfigUpdater,
	registry environs.ProviderRegistry,
) error {
	cfg, err := reader.ModelConfig()
	if err != nil {
		return errors.Annotate(err, "reading model config")
	}
	provider, err := registry.Provider(cfg.Type())
	if err != nil {
		return errors.Annotate(err, "getting provider")
	}

	upgrader, ok := provider.(environs.ModelConfigUpgrader)
	if !ok {
		logger.Debugf("provider %q has no upgrades", cfg.Type())
		return nil
	}
	newCfg, err := upgrader.UpgradeConfig(cfg)
	if err != nil {
		return errors.Annotate(err, "upgrading config")
	}

	newAttrs := newCfg.AllAttrs()
	var removedAttrs []string
	for key := range cfg.AllAttrs() {
		if _, ok := newAttrs[key]; !ok {
			removedAttrs = append(removedAttrs, key)
		}
	}
	if err := updater.UpdateModelConfig(newAttrs, removedAttrs, nil); err != nil {
		return errors.Annotate(err, "updating config in state")
	}
	return nil
}
