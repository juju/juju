// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/linklayerdevice"
)

// AddApplicationArg contains parameters for saving an application to state.
type AddApplicationArg struct {
	Charm    domaincharm.Charm
	Origin   domaincharm.CharmOrigin
	Platform Platform
	Channel  *Channel
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

// Platform represents the platform of a application charm.
type Platform = domaincharm.Platform

// OSType represents the operating system type of a application charm.
type OSType = domaincharm.OSType

// Architecture represents the architecture of a application charm.
type Architecture = domaincharm.Architecture

// ScaleState describes the scale status of a k8s application.
type ScaleState struct {
	Scaling     bool
	Scale       int
	ScaleTarget int
}

// CloudService contains parameters for an application's cloud service.
type CloudService struct {
	ProviderId string
	Address    *ServiceAddress
}

// ServiceAddress contains parameters for a cloud service address.
// This may be from a load balancer, or cluster service etc.
type ServiceAddress struct {
	Value       string
	AddressType ipaddress.AddressType
	Scope       ipaddress.Scope
	Origin      ipaddress.Origin
	ConfigType  ipaddress.ConfigType
}

// Origin contains parameters for an application's origin.
type Origin struct {
	Revision int
}

// CloudContainer contains parameters for a unit's cloud container.
type CloudContainer struct {
	ProviderId *string
	Address    *ContainerAddress
	Ports      *[]string
}

// ContainerDevice is the placeholder link layer device
// used to tie the cloud container IP address to the container.
type ContainerDevice struct {
	Name              string
	DeviceTypeID      linklayerdevice.DeviceType
	VirtualPortTypeID linklayerdevice.VirtualPortType
}

// ContainerAddress contains parameters for a cloud container address.
// Device is an attribute of address rather than cloud container
// since it's a placeholder used to tie the address to the
// cloud container and is only needed if the address exists.
type ContainerAddress struct {
	Device      ContainerDevice
	Value       string
	AddressType ipaddress.AddressType
	Scope       ipaddress.Scope
	Origin      ipaddress.Origin
	ConfigType  ipaddress.ConfigType
}

// UpsertUnitArg contains parameters for adding a unit to state.
type UpsertUnitArg struct {
	UnitName       string
	PasswordHash   *string
	CloudContainer *CloudContainer
}

// CloudContainerStatusStatusInfo holds a cloud container status
// and associated information.
type CloudContainerStatusStatusInfo struct {
	StatusID CloudContainerStatusType
	Message  string
	Data     map[string]string
	Since    time.Time
}

// UnitAgentStatusInfo holds a unit agent status
// and associated information.
type UnitAgentStatusInfo struct {
	StatusID UnitAgentStatusType
	Message  string
	Data     map[string]string
	Since    time.Time
}

// UnitWorkloadStatusInfo holds a unit workload status
// and associated information.
type UnitWorkloadStatusInfo struct {
	StatusID UnitWorkloadStatusType
	Message  string
	Data     map[string]string
	Since    time.Time
}
