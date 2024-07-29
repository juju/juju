// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
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
