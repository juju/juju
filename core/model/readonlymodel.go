// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/controller"
	"github.com/juju/juju/core/user"
)

// ReadOnlyModel represents the state of a read-only model found in the
// model database, not the controller database.
// All the fields are are denormalized from the model database.
type ReadOnlyModel struct {
	// UUID represents the model UUID.
	UUID UUID

	// AgentVersion reports the current target agent version for the model.
	AgentVersion version.Number

	// ControllerUUID represents the controller UUID.
	ControllerUUID controller.UUID

	// Name is the name of the model.
	Name string

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
}
