// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// ModelType indicates a model type.
type ModelType string

const (
	// IAAS is the type for IAAS models.
	IAAS ModelType = "iaas"

	// CAAS is the type for CAAS models.
	CAAS ModelType = "caas"
)

// String returns m as a string.
func (m ModelType) String() string {
	return string(m)
}

// IsValid returns true if the value of Type is a known valid type.
// Currently supported values are:
// - CAAS
// - IAAS
func (m ModelType) IsValid() bool {
	switch m {
	case CAAS, IAAS:
		return true
	}
	return false
}

// Model represents the state of a model.
type Model struct {
	// Name returns the human friendly name of the model.
	Name string

	// Life is the current state of the model.
	// Options are alive, dying, dead. Every model starts as alive, only
	// during the destruction of the model it transitions to dying and then
	// dead.
	Life life.Value

	// UUID is the universally unique identifier of the model.
	UUID UUID

	// ModelType is the type of model.
	ModelType ModelType

	// AgentVersion is the target version for agents running under this model.
	AgentVersion semversion.Number

	// Cloud is the name of the cloud to associate with the model.
	// Must not be empty for a valid struct.
	Cloud string

	// CloudType is the type of the underlying cloud (e.g. lxd, azure, ...)
	CloudType string

	// CloudRegion is the region that the model will use in the cloud.
	CloudRegion string

	// Credential is the id attributes for the credential to be associated with
	// model. Credential must be for the same cloud as that of the model.
	// Credential can be the zero value of the struct to not have a credential
	// associated with the model.
	Credential credential.Key

	// Owner is the uuid of the user that owns this model in the Juju controller.
	Owner user.UUID

	// OwnerName is the name of the owner in the Juju controller.
	OwnerName user.Name
}

// UUID represents a model unique identifier.
type UUID string

// NewUUID is a convince function for generating a new model uuid.
func NewUUID() (UUID, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return UUID(""), err
	}
	return UUID(uuid.String()), nil
}

// String implements the stringer interface for UUID.
func (u UUID) String() string {
	return string(u)
}

// Validate ensures the consistency of the UUID. If the uuid is invalid an error
// satisfying [errors.NotValid] will be returned.
func (u UUID) Validate() error {
	if u == "" {
		return errors.Errorf("uuid cannot be empty").Add(coreerrors.NotValid)
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return errors.Errorf("uuid %q %w", u, coreerrors.NotValid)
	}
	return nil
}
