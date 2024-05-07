// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	coremodel "github.com/juju/juju/core/model"
)

// ModelSecretBackend represents a set of data about a model and its secret backend config.
type ModelSecretBackend struct {
	// ControllerUUID is the uuid of the controller.
	ControllerUUID string
	// ID is the unique identifier for the model.
	ID coremodel.UUID
	// Name is the name of the model.
	Name string
	// Type is the type of the model.
	Type coremodel.ModelType
	// SecretBackendID is the unique identifier for the secret backend configured for the model.
	SecretBackendID string
}
