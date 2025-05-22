// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"

	"github.com/juju/juju/cloud"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
)

// ModelTypeState describes the state interface required of the controller to
// ask questins related to a model's type.
type ModelTypeState interface {
	// GetCloudType is responsible for reporting the cloud type for the cloud
	// identified by name.
	//
	// The following errors can be expected:
	// - [github.com/juju/juju/domain/cloud/errors.NotFound] when no cloud
	// exists for the supplied name.
	GetCloudType(context.Context, string) (string, error)
}

// caasCloudTypes returns a set of cloud types that are considered to be CAAS
// clouds.
func caasCloudTypes() set.Strings {
	return set.NewStrings(
		cloud.CloudTypeKubernetes,
	)
}

// DetermineModelTypeForCloud calculates the expected model type based on the
// cloud that is to be used. This information is used when creating a new model
// to set the model type.
//
// This should not be used as a replacement for determining the model type for
// a model that already exists.
//
// The following errors can be expected:
// - [github.com/juju/juju/domain/cloud/errors.NotFound] when no cloud exists
// for the supplied cloud name.
func DetermineModelTypeForCloud(
	ctx context.Context,
	state ModelTypeState,
	cloudName string,
) (coremodel.ModelType, error) {
	cloudType, err := state.GetCloudType(ctx, cloudName)
	if err != nil {
		return "", errors.Capture(err)
	}

	if caasCloudTypes().Contains(cloudType) {
		return coremodel.CAAS, nil
	}
	return coremodel.IAAS, nil
}
