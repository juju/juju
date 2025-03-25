// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/errors"
)

// MigrationStatus returns the details for a migration as needed by
// the migrationmaster worker.
type MigrationStatus struct {
	// MigrationId hold the unique id for the migration.
	MigrationId string

	// ModelUUID holds the UUID of the model being migrated.
	ModelUUID string

	// Phases indicates the current migration phase.
	Phase Phase

	// PhaseChangedTime indicates the time the phase was changed to
	// its current value.
	PhaseChangedTime time.Time

	// TargetInfo contains the details of how to connect to the target
	// controller.
	TargetInfo TargetInfo
}

// SerializedModel wraps a buffer contain a serialised Juju model as
// well as containing metadata about the charms and tools used by the
// model.
type SerializedModel struct {
	// Bytes contains the serialized data for the model.
	Bytes []byte

	// Charms lists the charm URLs in use in the model.
	Charms []string

	// Tools lists the tools versions in use with the model along with
	// their URIs. The URIs can be used to download the tools from the
	// source controller.
	Tools map[semversion.Binary]string // version -> tools URI

	// Resources represents all the resources in use in the model.
	Resources []resource.Resource
}

// ModelInfo is used to report basic details about a model.
type ModelInfo struct {
	UUID                   string
	Owner                  names.UserTag
	Name                   string
	AgentVersion           semversion.Number
	ControllerAgentVersion semversion.Number
	ModelDescription       description.Model
}

// SourceControllerInfo holds the details required to connect
// to a migration's source controller.
type SourceControllerInfo struct {
	ControllerTag   names.ControllerTag
	ControllerAlias string
	Addrs           []string
	CACert          string
}

func (i *ModelInfo) Validate() error {
	if i.UUID == "" {
		return errors.Errorf("empty UUID %w", coreerrors.NotValid)
	}
	if i.Owner.Name() == "" {
		return errors.Errorf("empty Owner %w", coreerrors.NotValid)
	}
	if i.Name == "" {
		return errors.Errorf("empty Name %w", coreerrors.NotValid)
	}
<<<<<<< HEAD
	if i.AgentVersion.Compare(semversion.Number{}) == 0 {
		return errors.NotValidf("empty Version")
=======
	if i.AgentVersion.Compare(version.Number{}) == 0 {
		return errors.Errorf("empty Version %w", coreerrors.NotValid)
>>>>>>> bd7c573581 (refactor: remove juju/errors from core)
	}
	return nil
}
