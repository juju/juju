// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/uuid"
)

// ModelInfo represents the state of a model found in the  model database.
type ModelInfo struct {
	// UUID represents the model UUID.
	UUID UUID

	// ControllerUUID represents the controller UUID.
	ControllerUUID uuid.UUID

	// Name is the name of the model.
	Name string

	// Qualifier disambiguates the model name.
	Qualifier Qualifier

	// Type is the type of the model.
	Type ModelType

	// Cloud is the name of the cloud to associate with the model.
	Cloud string

	// CloudType is the type of the underlying cloud (e.g. lxd, azure, ...)
	CloudType string

	// CloudRegion is the region that the model will use in the cloud.
	CloudRegion string

	// CredentialOwner is the owner of the model.
	CredentialOwner user.Name

	// Credential name is the name of the credential to use for the model.
	CredentialName string

	// IsControllerModel is a boolean value that indicates if the model is the
	// controller model.
	IsControllerModel bool

	// AgentVersion is the Juju version for agent binaries in this model.
	AgentVersion semversion.Number
}

// ModelMetrics represents the metrics information set in the database.
type ModelMetrics struct {
	// Model is the detail from the model database.
	Model ModelInfo

	// ApplicationCount is the number of applications in the model.
	ApplicationCount int

	// MachineCount is the number of machines in the model.
	MachineCount int

	// UnitCount is the number of units in the model.
	UnitCount int
}
