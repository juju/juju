// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
)

type metadataAcess interface {
	FindMetadata(cloudimagemetadata.MetadataFilter) (map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata, error)
	SaveMetadata(cloudimagemetadata.Metadata) error
}

var getState = func(st *state.State) metadataAcess {
	return stateShim{st}
}

type stateShim struct {
	*state.State
}

func (s stateShim) FindMetadata(f cloudimagemetadata.MetadataFilter) (map[cloudimagemetadata.SourceType][]cloudimagemetadata.Metadata, error) {
	return s.State.CloudImageMetadataStorage.FindMetadata(f)
}

func (s stateShim) SaveMetadata(m cloudimagemetadata.Metadata) error {
	return s.State.CloudImageMetadataStorage.SaveMetadata(m)
}
