// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	internalcharm "github.com/juju/juju/internal/charm"
)

// Application represents the status of an application.
type Application struct {
	Life            life.Value
	Status          status.StatusInfo
	Relations       []relation.UUID
	Subordinate     bool
	CharmLocator    charm.CharmLocator
	CharmVersion    string
	Platform        deployment.Platform
	Channel         *deployment.Channel
	Exposed         bool
	LXDProfile      *internalcharm.LXDProfile
	Scale           *int
	WorkloadVersion *string
	K8sProviderID   *string
	Units           map[unit.Name]Unit
}

// Unit represents the status of a unit.
type Unit struct {
	Life             life.Value
	ApplicationName  string
	MachineName      *machine.Name
	AgentStatus      status.StatusInfo
	WorkloadStatus   status.StatusInfo
	K8sPodStatus     status.StatusInfo
	Present          bool
	Subordinate      bool
	PrincipalName    *unit.Name
	SubordinateNames []unit.Name
	CharmLocator     charm.CharmLocator
	AgentVersion     string
	WorkloadVersion  *string
	K8sProviderID    *string
}

// StatusHistoryFilter holds the parameters to filter a status history query.
type StatusHistoryFilter struct {
	Size  int
	Date  *time.Time
	Delta *time.Duration
}

// StatusHistoryRequest holds the parameters to filter a status history query.
type StatusHistoryRequest struct {
	Kind   status.HistoryKind
	Filter StatusHistoryFilter
	Tag    string
}
