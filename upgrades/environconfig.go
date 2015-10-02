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
	// UpdateEnvironConfig atomically updates and removes environment
	// config attributes to the global state.
	UpdateEnvironConfig(map[string]interface{}, []string, state.ValidateConfigFunc) error
}

// environConfigReader is an interface used to read the current environment
// config from global state.
type environConfigReader interface {
	// EnvironConfig reads the current environment config from global
	// state.
	EnvironConfig() (*config.Config, error)
}

func upgradeEnvironConfig(
	reader environConfigReader,
	updater environConfigUpdater,
	registry environs.ProviderRegistry,
) error {
	cfg, err := reader.EnvironConfig()
	if err != nil {
		return errors.Annotate(err, "reading environment config")
	}
	provider, err := registry.Provider(cfg.Type())
	if err != nil {
		return errors.Annotate(err, "getting provider")
	}

	upgrader, ok := provider.(environs.EnvironConfigUpgrader)
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
	if err := updater.UpdateEnvironConfig(newAttrs, removedAttrs, nil); err != nil {
		return errors.Annotate(err, "updating config in state")
	}
	return nil
}
