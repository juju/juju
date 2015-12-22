// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import "github.com/juju/juju/environs"

var (
	UpgradeOperations      = &upgradeOperations
	StateUpgradeOperations = &stateUpgradeOperations
)

type EnvironConfigUpdater environConfigUpdater
type EnvironConfigReader environConfigReader

func UpgradeEnvironConfig(
	reader EnvironConfigReader,
	updater EnvironConfigUpdater,
	registry environs.ProviderRegistry,
) error {
	return upgradeEnvironConfig(reader, updater, registry)
}
