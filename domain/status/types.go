// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
)

// Application represents the status of an application.
type Application struct {
	ID            application.ID
	Life          life.Life
	Status        StatusInfo[WorkloadStatusType]
	Units         map[unit.Name]Unit
	Relations     []relation.UUID
	Subordinate   bool
	CharmLocator  charm.CharmLocator
	CharmVersion  string
	LXDProfile    []byte
	Platform      deployment.Platform
	Channel       *deployment.Channel
	Exposed       bool
	Scale         *int
	K8sProviderID *string
}

// Unit represents the status of a unit.
type Unit struct {
	AgentStatus     StatusInfo[UnitAgentStatusType]
	WorkloadStatus  StatusInfo[WorkloadStatusType]
	WorkloadVersion string
	Life            life.Life
	Leader          bool
	Machine         machine.Name
	OpenedPorts     []string
	PublicAddress   string
	Subordinates    []unit.Name
	Present         bool
}
