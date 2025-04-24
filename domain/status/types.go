// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/life"
)

// Application represents the status of an application.
type Application struct {
	Life      life.Life
	Status    StatusInfo[WorkloadStatusType]
	Units     map[unit.Name]Unit
	Relations []relation.UUID
}

// Unit represents the status of a unit.
type Unit struct {
	AgentStatus     StatusInfo[UnitAgentStatusType]
	WorkloadStatus  StatusInfo[WorkloadStatusType]
	WorkloadVersion string
	Leader          bool
	Machine         machine.Name
	OpenedPorts     []string
	PublicAddress   string
	Subordinates    []unit.Name
	Present         bool
}
