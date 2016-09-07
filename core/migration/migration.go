// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
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
	Tools map[version.Binary]string // version -> tools URI
}

// ModelInfo is used to report basic details about a model.
type ModelInfo struct {
	UUID         string
	Owner        names.UserTag
	Name         string
	AgentVersion version.Number
}

func (i *ModelInfo) Validate() error {
	if i.UUID == "" {
		return errors.NotValidf("empty UUID")
	}
	if i.Owner.Name() == "" {
		return errors.NotValidf("empty Owner")
	}
	if i.Name == "" {
		return errors.NotValidf("empty Name")
	}
	if i.AgentVersion.Compare(version.Number{}) == 0 {
		return errors.NotValidf("empty Version")
	}
	return nil
}
