// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

// ReadOnlyModel represents the state of a read-only model found in the
// model database, not the controller database.
// All the fields are are denormalized from the model database.
type ReadOnlyModel struct {
	// UUID represents the model UUID.
	UUID UUID

	// ControllerUUID represents the controller UUID.
	ControllerUUID UUID

	// Name is the name of the model.
	Name string

	// Owner is the materialized name of the owner for the model.
	Owner string

	// Type is the type of the model.
	Type ModelType

	// Cloud is the name of the cloud to associate with the model.
	Cloud string

	// CloudRegion is the region that the model will use in the cloud.
	CloudRegion string

	// CredentialOwner is the owner of the model.
	CredentialOwner string

	// Credential name is the name of the credential to use for the model.
	CredentialName string
}
