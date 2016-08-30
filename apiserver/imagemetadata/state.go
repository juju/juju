// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/cloudimagemetadata"
	names "gopkg.in/juju/names.v2"
)

type metadataAcess interface {
	FindMetadata(cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error)
	SaveMetadata([]cloudimagemetadata.Metadata) error
	DeleteMetadata(imageId string) error
	Model() (Model, error)
	ModelConfig() (*config.Config, error)
	ControllerTag() names.ControllerTag
}

type Model interface {
	CloudRegion() string
}

var getState = func(st *state.State) metadataAcess {
	return stateShim{st}
}

type stateShim struct {
	*state.State
}

func (s stateShim) FindMetadata(f cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error) {
	return s.State.CloudImageMetadataStorage.FindMetadata(f)
}

func (s stateShim) SaveMetadata(m []cloudimagemetadata.Metadata) error {
	return s.State.CloudImageMetadataStorage.SaveMetadata(m)
}

func (s stateShim) DeleteMetadata(imageId string) error {
	return s.State.CloudImageMetadataStorage.DeleteMetadata(imageId)
}

func (s stateShim) Model() (Model, error) {
	m, err := s.State.Model()
	if err != nil {
		return nil, err
	}
	return modelShim{m}, nil
}

type modelShim struct {
	*state.Model
}
