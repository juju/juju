// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/errors"
)

// CreationArgs supplies the information required for establishing the presence
// of a new model in the controller.
type CreationArgs struct {
	// Cloud is the name of the cloud to associate with the model.
	// Must not be empty for a valid struct.
	Cloud string

	// CloudRegion is the region that the model will use in the cloud.
	CloudRegion string

	// Credential is the id attributes for the credential to be associated with
	// model. Credential must be for the same cloud as that of the model.
	// Credential can be the zero value of the struct to not have a credential
	// associated with the model.
	Credential corecredential.Key

	// Name is the name of the model.
	// Must not be empty for a valid struct.
	Name string

	// Owner is the uuid of the user that owns this model in the Juju controller.
	Owner coreuser.UUID

	// SecretBackend dictates the secret backend to be used for the newly
	// created model. SecretBackend can be left empty and a default will be
	// chosen at creation time.
	SecretBackend string
}

// ImportArgs supplies the information required for importing a model into a
// controller during model migration.
type ImportArgs struct {
	// CreationArgs is the import information about the new model.
	CreationArgs

	// UUID is the uuid to assign to the new model being imported.
	UUID coremodel.UUID
}

// Validate is responsible for checking all of the required fields of
// [CreationArgs] are in a set state that is valid for use.
//
// The following errors can be expected:
// - [coreerrors.NotValid] when a member of [CreationArgs] has not been set to
// a valid value that is required for creating new models.
func (c CreationArgs) Validate() error {
	if c.Cloud == "" {
		return errors.New("cloud cannot be empty").Add(coreerrors.NotValid)
	}
	if c.Name == "" {
		return errors.New("name cannot be empty").Add(coreerrors.NotValid)
	}
	if err := c.Owner.Validate(); err != nil {
		return errors.Errorf("validating model owner: %w", err).Add(coreerrors.NotValid)
	}
	if !c.Credential.IsZero() {
		if err := c.Credential.Validate(); err != nil {
			return errors.Errorf("validating model credential: %w", err).Add(coreerrors.NotValid)
		}
	}
	return nil
}
