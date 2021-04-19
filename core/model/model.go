// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/series"
)

// ModelType indicates a model type.
type ModelType string

const (
	// IAAS is the type for IAAS models.
	IAAS ModelType = "iaas"

	// CAAS is the type for CAAS models.
	CAAS ModelType = "caas"
)

// String returns m as a string.
func (m ModelType) String() string {
	return string(m)
}

// Model represents the state of a model.
type Model struct {
	// Name returns the human friendly name of the model.
	Name string

	// UUID is the universally unique identifier of the model.
	UUID string

	// ModelType is the type of model.
	ModelType ModelType
}

// ValidateModelTarget ensures the charm is valid for the model target type.
// This works for both v1 and v2 of the charm metadata. By looking if the
// series for v1 charm contains kubernetes or by checking the existence of
// containers within the v2 metadata as a way to see if kubernetes is supported.
func ValidateModelTarget(modelType ModelType, metaSeries []string, metaContainers map[string]charm.Container) error {
	isIAAS := !(set.NewStrings(metaSeries...).Contains(series.Kubernetes.String()) || len(metaContainers) > 0)

	switch modelType {
	case CAAS:
		if isIAAS {
			return errors.NotValidf("non container-based charm for container based model type")
		}
	case IAAS:
		if !isIAAS {
			return errors.NotValidf("container-based charm for non container based model type")
		}
	default:
		return errors.Errorf("invalid model type")
	}
	return nil
}
