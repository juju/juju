// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/version"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/migration"
	"github.com/juju/juju/state"
)

//go:generate mockgen -package mocks -destination mocks/backend.go github.com/juju/juju/apiserver/facades/controller/migrationmaster Backend
//go:generate mockgen -package mocks -destination mocks/precheckbackend.go github.com/juju/juju/migration PrecheckBackend
//go:generate mockgen -package mocks -destination mocks/state.go github.com/juju/juju/state ModelMigration,NotifyWatcher

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
}
