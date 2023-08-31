// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import "github.com/juju/juju/core/constraints"

// CloudInstanceTypesConstraints contains a slice of CloudInstanceTypesConstraint.
type CloudInstanceTypesConstraints struct {
	Constraints []CloudInstanceTypesConstraint `json:"constraints"`
}

// CloudInstanceTypesConstraint contains the cloud information and constraints
// necessary to query for instance types on a given cloud.
type CloudInstanceTypesConstraint struct {
	// CloudTag is the tag of the cloud for which instances types
	// should be returned.
	CloudTag string `json:"cloud-tag"`

	// CloudRegion is the name of the region for which instance
	// types should be returned.
	CloudRegion string `json:"region"`

	// Constraints, if specified, contains the constraints to filter
	// the instance types by. If Constraints is not specified, then
	// no filtering by constraints will take place: all instance
	// types supported by the region will be returned.
	Constraints *constraints.Value `json:"constraints,omitempty"`
}

// ModelInstanceTypesConstraints contains a slice of InstanceTypesConstraint.
type ModelInstanceTypesConstraints struct {
	// Constraints, if specified, contains the constraints to filter
	// the instance types by. If Constraints is not specified, then
	// no filtering by constraints will take place: all instance
	// types supported by the model will be returned.
	Constraints []ModelInstanceTypesConstraint `json:"constraints"`
}

// ModelInstanceTypesConstraint contains a constraint applied when filtering instance types.
type ModelInstanceTypesConstraint struct {
	// Value, if specified, contains the constraints to filter
	// the instance types by. If Value is not specified, then
	// no filtering by constraints will take place: all instance
	// types supported by the region will be returned.
	Value *constraints.Value `json:"value,omitempty"`
}

// InstanceTypesResults contains the bulk result of prompting a cloud for its instance types.
type InstanceTypesResults struct {
	Results []InstanceTypesResult `json:"results"`
}

// InstanceTypesResult contains the result of prompting a cloud for its instance types.
type InstanceTypesResult struct {
	InstanceTypes []InstanceType `json:"instance-types,omitempty"`
	CostUnit      string         `json:"cost-unit,omitempty"`
	CostCurrency  string         `json:"cost-currency,omitempty"`
	// CostDivisor Will be present only when the Cost is not expressed in CostUnit.
	CostDivisor uint64 `json:"cost-divisor,omitempty"`
	Error       *Error `json:"error,omitempty"`
}

// InstanceType represents an available instance type in a cloud.
type InstanceType struct {
	Name         string   `json:"name,omitempty"`
	Arches       []string `json:"arches"`
	CPUCores     int      `json:"cpu-cores"`
	Memory       int      `json:"memory"`
	RootDiskSize int      `json:"root-disk,omitempty"`
	VirtType     string   `json:"virt-type,omitempty"`
	Cost         int      `json:"cost,omitempty"`
}
