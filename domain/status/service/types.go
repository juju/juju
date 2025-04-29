// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	internalcharm "github.com/juju/juju/internal/charm"
)

// Application represents the status of an application.
type Application struct {
	Life          life.Value
	Status        status.StatusInfo
	Relations     []relation.UUID
	Subordinate   bool
	CharmLocator  charm.CharmLocator
	CharmVersion  string
	Platform      deployment.Platform
	Channel       *deployment.Channel
	Exposed       bool
	LXDProfile    *internalcharm.LXDProfile
	Scale         *int
	K8sProviderID *string
	Units         map[unit.Name]Unit
}

// Unit represents the status of a unit.
type Unit struct {
	Life           life.Value
	AgentStatus    status.StatusInfo
	WorkloadStatus status.StatusInfo
}
