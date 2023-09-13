// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/domain/credential"
)

// ModelCreationArgs supplies the information required for instantiating a new
// model.
type ModelCreationArgs struct {
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

	// Owner is the name of the owner for the model.
	// Must not be empty for a valid struct.
	Owner string

	// Type is the type of the model.
	// Type must satisfy IsValid() for a valid struct.
	Type Type
}

// Type represents the type of a model.
type Type string

// UUID represents a model unique identifier.
type UUID string

const (
	// TypeCAAS is the type used for CAAS models.
	TypeCAAS Type = "caas"

	// TypeIAAS is the type used for IAAS models.
	TypeIAAS Type = "iaas"
)

// IsValid returns true if the value of Type is a known valid type.
// Currently supported values are:
// - TypeCAAS
// - TypeIAAS
// - TypeNone
func (t Type) IsValid() bool {
	switch t {
	case TypeCAAS,
		TypeIAAS:
		return true
	}
	return false
}

// NewUUID is a convince function for generating new model uuid id's.
func NewUUID() (UUID, error) {
	uuid, err := utils.NewUUID()
	if err != nil {
		return UUID(""), err
	}
	return UUID(uuid.String()), nil
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
	if m.Owner == "" {
		return fmt.Errorf("%w owner cannot be empty", errors.NotValid)
	}
	if !m.Type.IsValid() {
		return fmt.Errorf("%w model type of %q", errors.NotSupported, m.Type)
	}
	if !m.Credential.IsZero() {
		if err := m.Credential.Validate(); err != nil {
			return fmt.Errorf("credential: %w", err)
		}
	}
	return nil
}

// Validate ensures the consistency of the UUID.
func (u UUID) Validate() error {
	if u == "" {
		return errors.New("empty uuid")
	}
	if !utils.IsValidUUIDString(string(u)) {
		return errors.Errorf("invalid uuid %q", u)
	}
	return nil
}

// String implements the stringer interface for Type.
func (t Type) String() string {
	return string(t)
}

// String implements the stringer interface for UUID.
func (u UUID) String() string {
	return string(u)
}
