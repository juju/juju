// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/life"
)

// Application represents the status of an application.
type Application struct {
	Life         life.Life
	Status       StatusInfo[WorkloadStatusType]
	Units        map[unit.Name]Unit
	Relations    []relation.UUID
	Subordinate  bool
	CharmLocator charm.CharmLocator
	CharmVersion string
	Platform     Platform
	Channel      *Channel
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

// Channel represents the channel of a application charm.
// Do not confuse this with a channel that is in the manifest file found
// in the charm package. They represent different concepts, yet hold the
// same data.
type Channel struct {
	Track  string
	Risk   ChannelRisk
	Branch string
}

// ChannelRisk describes the type of risk in a current channel.
type ChannelRisk string

const (
	RiskStable    ChannelRisk = "stable"
	RiskCandidate ChannelRisk = "candidate"
	RiskBeta      ChannelRisk = "beta"
	RiskEdge      ChannelRisk = "edge"
)

// OSType represents the type of an application's OS.
type OSType int

const (
	Ubuntu OSType = iota
)

// Platform contains parameters for an application's platform.
type Platform struct {
	Channel      string
	OSType       OSType
	Architecture architecture.Architecture
}
