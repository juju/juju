// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

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
	Credential credential.ID

	// Name is the name of the model.
	// Must not be empty for a valid struct.
	Name string

	// Owner is the uuid of the user that owns this model in the Juju controller.
	Owner user.UUID

	// UUID represents the unique id for the model when being created. This
	// value is optional and if omitted will be generated for the caller. Use
	// this value when you are trying to import a model during model migration.
	UUID coremodel.UUID
}

// Validate is responsible for checking all of the fields of ModelCreationArgs
// are in a set state that is valid for use.
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

	if m.UUID != "" {
		if err := m.UUID.Validate(); err != nil {
			return fmt.Errorf("uuid: %w", err)
		}
	}
	return nil
}

// AsReadOnly returns a ReadOnlyModelCreationArgs struct that is a copy of the
// ModelCreationArgs struct. This is useful when you want to create a model
// within the model database.
func (m ModelCreationArgs) AsReadOnly() ReadOnlyModelCreationArgs {
	return ReadOnlyModelCreationArgs{
		UUID:        m.UUID,
		Cloud:       m.Cloud,
		CloudRegion: m.CloudRegion,
		Name:        m.Name,
		// TODO (tlm): This is a hack that Simon or TLM will deal with in
		// coming days. See #17063 for a possible solution to this problem.
		Type: coremodel.IAAS,
	}
}

// ReadOnlyModelCreationArgs is a struct that is used to create a model
// within the model database. This struct is used to create a model with all of
// its associated metadata.
type ReadOnlyModelCreationArgs struct {
	// UUID represents the unique id for the model when being created. This
	// value is optional and if omitted will be generated for the caller. Use
	// this value when you are trying to import a model during model migration.
	UUID coremodel.UUID

	// Type is the type of the model.
	// Type must satisfy IsValid() for a valid struct.
	Type coremodel.ModelType

	// Cloud is the name of the cloud to associate with the model.
	// Must not be empty for a valid struct.
	Cloud string

	// CloudRegion is the region that the model will use in the cloud.
	CloudRegion string

	// Name is the name of the model.
	// Must not be empty for a valid struct.
	Name string
}

// Validate is responsible for checking all of the fields of ModelCreationArgs
// are in a set state that is valid for use.
func (m ReadOnlyModelCreationArgs) Validate() error {
	if m.Cloud == "" {
		return fmt.Errorf("%w cloud cannot be empty", errors.NotValid)
	}
	if m.Name == "" {
		return fmt.Errorf("%w name cannot be empty", errors.NotValid)
	}
	if !m.Type.IsValid() {
		return fmt.Errorf("%w model type of %q", errors.NotSupported, m.Type)
	}
	if err := m.UUID.Validate(); err != nil {
		return fmt.Errorf("uuid: %w", err)
	}
	return nil
}
