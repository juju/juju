// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"github.com/juju/description"

	"github.com/juju/juju/state"
)

type Backend interface {
	ExportPartial(cfg state.ExportConfig) (description.Model, error)
	SetExportconfig(cfg *state.ExportConfig)
}

type stateShim struct {
	*state.State
}

func (m *stateShim) SetExportconfig(cfg *state.ExportConfig) {
	logger.Criticalf("XXXXXXXXXXXXXXXXXXXXXX Iam inside SetExportconfig..")
	cfg.SkipActions = true
	cfg.SkipCloudImageMetadata = true
	cfg.SkipCredentials = true
	cfg.SkipIPAddresses = true
	cfg.SkipSSHHostKeys = true
	cfg.SkipStatusHistory = true
	cfg.SkipLinkLayerDevices = true
}

// NewStateShim creates new state shim to be used by bundle Facade.
func NewStateShim(st *state.State) Backend {
	return &stateShim{st}
}
