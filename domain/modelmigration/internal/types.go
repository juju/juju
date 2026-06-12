// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"time"

	"github.com/juju/juju/core/migration"
)

// ExternalControllerAddress describes an address row to persist for an
// external controller.
type ExternalControllerAddress struct {
	UUID    string
	Address string
}

// MigrationSpec describes the state-layer data required to record a new source
// export migration.
type MigrationSpec struct {
	MigrationUUID string
	ModelUUID     string

	TargetControllerUUID  string
	TargetControllerAlias string
	TargetAddrs           []ExternalControllerAddress
	TargetCACert          string
	TargetUser            string
	TargetMacaroons       string
	TargetToken           string
	TargetSkipUserChecks  bool
}

// Migration is the state-layer representation of an active export migration.
type Migration struct {
	UUID             string
	Phase            migration.Phase
	PhaseChangedTime time.Time
	Target           TargetInfo
}

// TargetInfo carries target connection details as persisted by state.
type TargetInfo struct {
	ControllerUUID  string
	ControllerAlias string
	Addrs           []string
	CACert          string
	User            string
	Macaroons       string
	Token           string
	SkipUserChecks  bool
}

// MinionReports is the aggregated set of persisted minion phase reports for a
// single migration phase.
type MinionReports struct {
	Phase     migration.Phase
	Succeeded []string
	Failed    []string
}

// MigrationAgents contains the raw names of agents that must report migration
// minion progress for a model.
type MigrationAgents struct {
	Machines     []string
	Units        []string
	Applications []string
}

// OffererModel identifies a single (offerer controller, offerer model) pair
// referenced by the model's remote applications. It is the model-database input
// used to read the matching third-party external controller rows from the
// controller database.
type OffererModel struct {
	ControllerUUID string
	ModelUUID      string
}
