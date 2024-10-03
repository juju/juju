// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/environs/config"
)

// ModelConfigService is an interface that provides access to model config.
type ModelConfigService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
}

// ModelInfoService is a service for interacting with the data that describes
// the current model being worked on.
type ModelInfoService interface {
	// GetModelInfo returns the information associated with the current model.
	GetModelInfo(context.Context) (coremodel.ReadOnlyModel, error)
}

// MetadataService defines methods to access and manipulate cloud image metadata.
type MetadataService interface {

	// SaveMetadata saves the provided list of cloud image metadata to the storage.
	SaveMetadata(ctx context.Context, metadata []cloudimagemetadata.Metadata) error

	// DeleteMetadataWithImageID deletes the metadata associated with the specified image ID.
	DeleteMetadataWithImageID(ctx context.Context, imageID string) error

	// FindMetadata retrieves cloud image metadata based on provided filter criteria.
	// It returns a map of metadata grouped by source.
	FindMetadata(ctx context.Context, criteria cloudimagemetadata.MetadataFilter) (map[string][]cloudimagemetadata.Metadata, error)
}
