// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"github.com/juju/description"

	"github.com/juju/juju/state"
)

type Backend interface {
	ExportPartial(cfg state.ExportConfig) (description.Model, error)
	GetExportConfig() state.ExportConfig
}

type stateShim struct {
	*state.State
}

// GetExportConfig implements Backend.GetExportConfig.
func (m *stateShim) GetExportConfig() state.ExportConfig {
	var cfg state.ExportConfig
	cfg.SkipActions = true
	cfg.SkipCloudImageMetadata = true
	cfg.SkipCredentials = true
	cfg.SkipIPAddresses = true
	cfg.SkipSSHHostKeys = true
	cfg.SkipStatusHistory = true
	cfg.SkipLinkLayerDevices = true
	cfg.SkipRelationScope = true
	cfg.SkipMachineAgentBinaries = true
	cfg.SkipUnitAgentBinaries = true
	cfg.SkipInstanceData = true
	cfg.SkipSettings = true

	return cfg
}

// NewStateShim creates new state shim to be used by bundle Facade.
func NewStateShim(st *state.State) Backend {
	return &stateShim{st}
}
