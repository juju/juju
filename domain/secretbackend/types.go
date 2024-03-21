// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	coremodel "github.com/juju/juju/core/model"
)

// Model represents a single subset of a row from the state database's model_metadata table.
type Model struct {
	// ID is the unique identifier for the model.
	ID coremodel.UUID
	// Name is the name of the model.
	Name string
	// Type is the type of the model.
	Type coremodel.ModelType
	// SecretBackendID is the unique identifier for the secret backend configured for the model.
	SecretBackendID string
}
