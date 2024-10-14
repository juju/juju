// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/controller"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
)

// ModelCreationArgs supplies the information required for instantiating a new
// model.
type ModelCreationArgs struct {
	// AgentVersion is the target version for agents running under this model.
	AgentVersion version.Number

	// Cloud is the name of the cloud to associate with the model.
	// Must not be empty for a valid struct.
	Cloud string

	// CloudRegion is the region that the model will use in the cloud.
	CloudRegion string

	// Credential is the id attributes for the credential to be associated with
	// model. Credential must be for the same cloud as that of the model.
	// Credential can be the zero value of the struct to not have a credential
	// associated with the model.
	Credential credential.Key

	// Name is the name of the model.
	// Must not be empty for a valid struct.
	Name string

	// Owner is the uuid of the user that owns this model in the Juju controller.
	Owner user.UUID

	// SecretBackend dictates the secret backend to be used for the newly
	// created model. SecretBackend can be left empty and a default will be
	// chosen at creation time.
	SecretBackend string
}

// Validate is responsible for checking all of the fields of ModelCreationArgs
// are in a set state that is valid for use. If a validation failure happens an
// error satisfying [errors.NotValid] is returned.
func (m ModelCreationArgs) Validate() error {
	if m.Cloud == "" {
		return fmt.Errorf("%w cloud cannot be empty", errors.NotValid)
	}
	if m.Name == "" {
		return fmt.Errorf("%w name cannot be empty", errors.NotValid)
	}
	if err := m.Owner.Validate(); err != nil {
		return fmt.Errorf("%w owner: %w", errors.NotValid, err)
	}
	if !m.Credential.IsZero() {
		if err := m.Credential.Validate(); err != nil {
			return fmt.Errorf("credential: %w", err)
		}
	}
	return nil
}

// ModelImportArgs supplies the information needed for importing a model into a
// Juju controller.
type ModelImportArgs struct {
	// ID represents the unique id of the model to import.
	ID coremodel.UUID

	// ModelCreationArgs supplies the information needed for importing the new
	// model into Juju.
	ModelCreationArgs
}

// Validate is responsible for checking all of the fields of [ModelImportArgs]
// are in a set state valid for use. If a validation failure happens an error
// satisfying [errors.NotValid] is returned.
func (m ModelImportArgs) Validate() error {
	if err := m.ModelCreationArgs.Validate(); err != nil {
		return fmt.Errorf("ModelCreationArgs %w", err)
	}

	if err := m.ID.Validate(); err != nil {
		return fmt.Errorf("validating model import args id: %w", err)
	}

	return nil
}

// ReadOnlyModelCreationArgs is a struct that is used to create a model
// within the model database. This struct is used to create a model with all of
// its associated metadata.
type ReadOnlyModelCreationArgs struct {
	// UUID represents the unique id for the model when being created. This
	// value is optional and if omitted will be generated for the caller. Use
	// this value when you are trying to import a model during model migration.
	UUID coremodel.UUID

	// AgentVersion represents the current target agent version for the model.
	AgentVersion version.Number

	// ControllerUUID represents the unique id for the controller that the model
	// is associated with.
	ControllerUUID controller.UUID

	// Name is the name of the model.
	// Must not be empty for a valid struct.
	Name string

	// Type is the type of the model.
	// Type must satisfy IsValid() for a valid struct.
	Type coremodel.ModelType

	// Cloud is the name of the cloud to associate with the model.
	// Must not be empty for a valid struct.
	Cloud string

	// CloudType is the type of the underlying cloud (e.g. lxd, azure, ...)
	CloudType string

	// CloudRegion is the region that the model will use in the cloud.
	// Optional and can be empty.
	CloudRegion string

	// CredentialOwner is the name of the credential owner for this model in
	// the Juju controller.
	// Optional and can be empty.
	CredentialOwner user.Name

	// CredentialName is the name of the credential to be associated with the
	// model.
	// Optional and can be empty.
	CredentialName string

	// IsControllerModel is a boolean value that indicates if the model is the
	// controller model.
	IsControllerModel bool
}

// DeleteModelOptions is a struct that is used to modify the behavior of the
// DeleteModel function.
type DeleteModelOptions struct {
	deleteDB bool
}

// DeleteDB returns a boolean value that indicates if the model should be
// deleted from the database.
func (o DeleteModelOptions) DeleteDB() bool {
	return o.deleteDB
}

// DefaultDeleteModelOptions returns a pointer to a DeleteModelOptions struct
// with the default values set.
func DefaultDeleteModelOptions() *DeleteModelOptions {
	return &DeleteModelOptions{
		// Until we have correctly implemented tearing down the model, we want
		// to keep the model around during normal model deletion.
		deleteDB: false,
	}
}

// DeleteModelOption is a functional option that can be used to modify the
// behavior of the DeleteModel function.
type DeleteModelOption func(*DeleteModelOptions)

// WithDeleteDB is a functional option that can be used to modify the behavior
// of the DeleteModel function to delete the model from the database.
func WithDeleteDB() DeleteModelOption {
	return func(o *DeleteModelOptions) {
		o.deleteDB = true
	}
}
