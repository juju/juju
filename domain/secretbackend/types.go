// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/secretbackend/internal"
)

// ModelSecretBackend represents a set of data about a model and its secret backend config.
type ModelSecretBackend struct {
	// ControllerUUID is the uuid of the controller.
	ControllerUUID string
	// ModelID is the unique identifier for the model.
	ModelID coremodel.UUID
	// ModelName is the name of the model.
	ModelName string
	// ModelType is the type of the model.
	ModelType coremodel.ModelType
	// SecretBackendOrigin is the origin of the secret backend configured for
	// the model (builtin or user)
	SecretBackendOrigin internal.Origin

	// SecretBackendName is the name of the secret backend configured for the model.
	SecretBackendName string
}

// ActiveBackendName returns the name of the active secret backend for the model.
func (m ModelSecretBackend) ActiveBackendName() string {
	if m.SecretBackendOrigin == internal.BuiltIn && m.ModelType == coremodel.CAAS {
		return internal.MakeBuiltInK8sSecretBackendName(m.ModelName)
	}
	return m.SecretBackendName
}
