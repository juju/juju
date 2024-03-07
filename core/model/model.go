// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/uuid"
)

// ModelType indicates a model type.
type ModelType string

// UUID represents a model unique identifier.
type UUID string

const (
	// IAAS is the type for IAAS models.
	IAAS ModelType = "iaas"

	// CAAS is the type for CAAS models.
	CAAS ModelType = "caas"
)

// Model represents the state of a model.
type Model struct {
	// Name returns the human friendly name of the model.
	Name string

	// UUID is the universally unique identifier of the model.
	UUID string

	// ModelType is the type of model.
	ModelType ModelType
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

// NewUUID is a convince function for generating a new model uuid.
func NewUUID() (UUID, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return UUID(""), err
	}
	return UUID(uuid.String()), nil
}

// String returns m as a string.
func (m ModelType) String() string {
	return string(m)
}

// String implements the stringer interface for UUID.
func (u UUID) String() string {
	return string(u)
}

// Validate ensures the consistency of the UUID. If the uuid is invalid an error
// satisfying [errors.NotValid] will be returned.
func (u UUID) Validate() error {
	if u == "" {
		return fmt.Errorf("%wuuid cannot be empty", errors.Hide(errors.NotValid))
	}
	if !uuid.IsValidUUIDString(string(u)) {
		return fmt.Errorf("uuid %q %w", u, errors.NotValid)
	}
	return nil
}
