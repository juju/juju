// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package types

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

// Model represents the state of a model.
type Model struct {
	// Name returns the human friendly name of the model.
	Name string

	// UUID is the universally unique identifier of the model.
	UUID string

	// ModelType is the type of model.
	ModelType ModelType
}
