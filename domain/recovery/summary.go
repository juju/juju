// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

import (
	"time"
)

// Summary is the recovery report printed when bootstrap finishes. It tells
// the operator what was recovered and what needs attention, because the
// controller reconciles the moment it starts: there is no approval gate.
type Summary struct {
	// ControllerUUID and ControllerName are the recovered identities.
	ControllerUUID string
	ControllerName string

	// SourceAgentVersion is the agent version the archive was taken from
	// (equal to the running version; the gate enforced it).
	SourceAgentVersion string

	// BackupFinished is when the source backup completed; its age tells
	// the operator how much state drift to expect.
	BackupFinished time.Time

	// HANodes is the number of controller nodes the source had.
	HANodes int64

	// DeadControllerMachines lists the source's other controller machine
	// names (HA sources only). They load as dead machines; the operator
	// removes them after recovery.
	DeadControllerMachines []string

	// Models is the number of recovered models, including the controller
	// model.
	Models int

	// ObjectsCopied is the number of object-store blobs installed into
	// the replacement's file-backed store.
	ObjectsCopied int

	// MachinesNotAlive, ApplicationsNotAlive and UnitsNotAlive count
	// entities whose recovered life is not alive, aggregated over all
	// model databases. Their pending removals run as soon as
	// reconciliation starts.
	MachinesNotAlive     int
	ApplicationsNotAlive int
	UnitsNotAlive        int

	// PendingRemovals counts rows in the model removal tables: scheduled
	// and forced removals that execute once the controller starts.
	PendingRemovals int
}

// RecoveryCounts contains persisted lifecycle counts for one model.
type RecoveryCounts struct {
	MachinesNotAlive     int
	ApplicationsNotAlive int
	UnitsNotAlive        int
	PendingRemovals      int
}
