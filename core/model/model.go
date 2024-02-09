// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

// ModelType indicates a model type.
type ModelType string

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

// String returns m as a string.
func (m ModelType) String() string {
	return string(m)
}
