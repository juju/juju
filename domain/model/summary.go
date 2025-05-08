// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"time"

	coremodel "github.com/juju/juju/core/model"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/user"
)

type ModelInfoSummary struct {
	// Name is the model name.
	Name string

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

	// OwnerName is the tag of the user that owns the model.
	OwnerName user.Name

	// AgentVersion is the agent version for this model.
	AgentVersion semversion.Number

	// MachineCount is the number of machines this model contains.
	MachineCount int64

	// CoreCount is the number of CPU cores used by this model.
	CoreCount int64

	// UnitCount is the number of application units in this model.
	UnitCount int64
}

type ModelSummary struct {
	State ModelState
}

type UserModelSummary struct {
	ModelSummary

	UserAccess corepermission.Access

	UserLastConnection *time.Time
}
