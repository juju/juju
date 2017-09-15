// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
)

type metadataAccess interface {
	SaveMetadata([]cloudimagemetadata.Metadata) error
	ModelConfig() (*config.Config, error)
}

type Model interface {
	CloudRegion() string
}

var getState = func(st *state.State) metadataAccess {
	return stateShim{st}
}

type stateShim struct {
	*state.State
}

func (s stateShim) SaveMetadata(m []cloudimagemetadata.Metadata) error {
	return s.State.CloudImageMetadataStorage.SaveMetadata(m)
}

func (st stateShim) ModelConfig() (*config.Config, error) {
	m, err := st.Model()
	if err != nil {
		return nil, err
	}

	return m.ModelConfig()
}
