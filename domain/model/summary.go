// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"time"

	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
)

// ModelInfoSummary represents a summary of model information from the model's
// database information. This is different to the information about a model that
// resides with in the controller.
//
// This exists because of the split of information that we have around model's
// and the controller that model runs in. To deal with this fact we represent
// the two sources of information as different structs.
type ModelInfoSummary struct {
	// Name is the model name.
	Name string

	// Qualifier disambiguates the model name.
	Qualifier coremodel.Qualifier

	// UUID is the model unique identifier.
	UUID coremodel.UUID

	// ModelType is the model type (e.g. IAAS or CAAS).
	ModelType coremodel.ModelType

	// CloudName is the name of the model cloud.
	CloudName string

	// CloudType is the models cloud type.
	CloudType string

	// CloudRegion is the region of the model cloud.
	CloudRegion string

	// ControllerUUID is the unique identifier of the controller.
	ControllerUUID string

	// IsController indicates if the model is a controller.
	IsController bool

	// AgentVersion is the agent version for this model.
	AgentVersion semversion.Number

	// MachineCount is the number of machines this model contains.
	MachineCount int64

	// CoreCount is the number of CPU cores used by this model.
	CoreCount int64

	// UnitCount is the number of application units in this model.
	UnitCount int64
}

// ModelSummary represents the model summary information from the controller.
// This is the complementary information to that of [ModelInfoSummary] which
// represents the model summary information from the model's database.
type ModelSummary struct {
	// Life is the current model's life cycle value.
	Life corelife.Value

	// State is the state of the model for calculating the model's status.
	State ModelState
}

// UserModelSummary represents the model summary information from the controller
// from the perspective of a single user. This is supplementing the information
// returned in [ModelSummary] with access and last login information.
type UserModelSummary struct {
	ModelSummary

	// UserAccess represents the level of access the user has for the model in
	// question.
	UserAccess corepermission.Access

	// UserLastConnection is the last time the user logged into this model.
	UserLastConnection *time.Time
}
