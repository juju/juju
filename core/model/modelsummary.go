// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"time"

	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
)

// UserModelSummary holds information about a model and a users access on it.
type UserModelSummary struct {
	// UserAccess is model access level for the  current user.
	UserAccess permission.Access

	// UserLastConnection contains the time when current user logged in
	// into the model last.
	UserLastConnection *time.Time

	// ModelSummary embeds the remaining model summary fields.
	ModelSummary
}

// ModelSummary holds summary about a Juju model.
type ModelSummary struct {
	// Name is the model name.
	Name string

	// Qualifier disambiguates the model name.
	Qualifier Qualifier

	// UUID is the model unique identifier.
	UUID UUID

	// ModelType is the model type (e.g. IAAS or CAAS).
	ModelType ModelType

	// CloudName is the name of the model cloud.
	CloudName string

	// CloudType is the models cloud type.
	CloudType string

	// CloudRegion is the region of the model cloud.
	CloudRegion string

	// CloudCredentialName is the name of the cloud credential.
	CloudCredentialKey credential.Key

	// ControllerUUID is the unique identifier of the controller.
	ControllerUUID string

	// IsController indicates if the model is a controller.
	IsController bool

	// Life is the current lifecycle state of the model.
	Life life.Value

	// AgentVersion is the agent version for this model.
	AgentVersion semversion.Number

	// Status is the current status of the model.
	Status status.StatusInfo

	// MachineCount is the number of machines this model contains.
	MachineCount int64
	// CoreCount is the number of CPU cores used by this model.
	CoreCount int64
	// UnitCount is the number of application units in this model.
	UnitCount int64

	// Migration contains information about the latest failed or
	// currently-running migration. It'll be nil if there isn't one.
	Migration *ModelMigrationStatus
}

// ModelMigrationStatus holds information about the progress of a (possibly
// failed) migration.
type ModelMigrationStatus struct {
	Status string     `json:"status"`
	Start  *time.Time `json:"start"`
	End    *time.Time `json:"end,omitempty"`
}
