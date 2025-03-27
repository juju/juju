// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"time"

	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// GlobalModelCreationArgs supplies the information required for
// recording details of a new model in the controller database.
type GlobalModelCreationArgs struct {
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

// Validate is responsible for checking all of the fields of
// GlobalModelCreationArgs are in a set state that is valid for use.
// If a validation failure happens an error satisfying [errors.NotValid]
// is returned.
func (m GlobalModelCreationArgs) Validate() error {
	if m.Cloud == "" {
		return errors.Errorf("%w cloud cannot be empty", coreerrors.NotValid)
	}
	if m.Name == "" {
		return errors.Errorf("%w name cannot be empty", coreerrors.NotValid)
	}
	if err := m.Owner.Validate(); err != nil {
		return errors.Errorf("%w owner: %w", coreerrors.NotValid, err)
	}
	if !m.Credential.IsZero() {
		if err := m.Credential.Validate(); err != nil {
			return errors.Errorf("credential: %w", err)
		}
	}
	return nil
}

// ModelDetailArgs supplies the information required for
// recording details of a new model in the model database.
type ModelDetailArgs struct {
	// UUID represents the unique id for the model when being created. This
	// value is optional and if omitted will be generated for the caller. Use
	// this value when you are trying to import a model during model migration.
	UUID coremodel.UUID

	// ControllerUUID represents the unique id for the controller that the model
	// is associated with.
	ControllerUUID uuid.UUID

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

	// AgentVersion is the target version for agents running in this model.
	AgentVersion semversion.Number
}

// ModelImportArgs supplies the information needed for importing a model into a
// Juju controller.
type ModelImportArgs struct {
	// GlobalModelCreationArgs supplies the information needed for
	// importing the new model into Juju.
	GlobalModelCreationArgs

	// ID represents the unique id of the model to import.
	ID coremodel.UUID

	// AgentVersion is the target version for agents running in this model.
	AgentVersion semversion.Number
}

// Validate is responsible for checking all of the fields of [ModelImportArgs]
// are in a set state valid for use. If a validation failure happens an error
// satisfying [errors.NotValid] is returned.
func (m ModelImportArgs) Validate() error {
	if err := m.GlobalModelCreationArgs.Validate(); err != nil {
		return errors.Errorf("GlobalModelCreationArgs %w", err)
	}

	if err := m.ID.Validate(); err != nil {
		return errors.Errorf("validating model import args id: %w", err)
	}

	return nil
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

// StatusInfo represents the current status of a model.
type StatusInfo struct {
	// Status is the current status of the model.
	Status corestatus.Status
	// Message is a human-readable message that describes the current status of the model.
	Message string
	// Reason is a human-readable message that describes the reason for the current status of the model.
	Reason string
	// Since is the time when the model entered the current status.
	Since time.Time
}

// ModelState describes the state of a model.
type ModelState struct {
	// Destroying is a boolean value that indicates if the model is being destroyed.
	Destroying bool
	// Migrating is a boolean value that indicates if the model is being migrated.
	Migrating bool
	// HasInvalidCloudCredential is a boolean value that indicates if the model's cloud credential is invalid.
	HasInvalidCloudCredential bool
	// InvalidCloudCredentialReason is a string that describes the reason for the model's cloud credential being invalid.
	InvalidCloudCredentialReason string
}
