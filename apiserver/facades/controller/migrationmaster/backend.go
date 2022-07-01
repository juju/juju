// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/juju/juju/v3/controller"
	"github.com/juju/juju/v3/migration"
	"github.com/juju/juju/v3/state"
)

// Backend defines the state functionality required by the
// migrationmaster facade.
type Backend interface {
	migration.StateExporter

	WatchForMigration() state.NotifyWatcher
	LatestMigration() (state.ModelMigration, error)
	ModelUUID() string
	ModelName() (string, error)
	ModelOwner() (names.UserTag, error)
	AgentVersion() (version.Number, error)
	RemoveExportingModelDocs() error
	ControllerConfig() (controller.Config, error)
}

// OfferConnection describes methods offer connection methods
// required for migration pre-checks.
type OfferConnection interface {
	// OfferUUID uniquely identifies the relation offer.
	OfferUUID() string

	// UserName returns the name of the user who created this connection.
	UserName() string

	// RelationId is the id of the relation to which this connection pertains.
	RelationId() int

	// SourceModelUUID is the uuid of the consuming model.
	SourceModelUUID() string

	// RelationKey is the key of the relation to which this connection pertains.
	RelationKey() string
}
