// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	"github.com/juju/juju/core/charm"
	coreunit "github.com/juju/juju/core/unit"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/linklayerdevice"
)

// AddApplicationArg contains parameters for saving an application to state.
type AddApplicationArg struct {
	Charm            domaincharm.Charm
	Origin           domaincharm.CharmOrigin
	Scale            int
	Platform         Platform
	Channel          *Channel
	CharmStoragePath string
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

const (
	// HashAlgorithmSHA256 is the sha256 hash algorithm.
	// Currently it's the only one.
	HashAlgorithmSHA256 = 0
)

// PasswordInfo contains password parameters.
type PasswordInfo struct {
	PasswordHash  string
	HashAlgorithm int
}

// CloudContainer contains parameters for a unit's cloud container.
type CloudContainer struct {
	ProviderId string
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

// AddUnitArg contains parameters for adding a unit to state.
type AddUnitArg struct {
	UnitName coreunit.Name
	UnitStatusArg
}

// InsertUnitArg is used to insert a fully populated unit.
// Used by import and when registering a CAAS unit.
type InsertUnitArg struct {
	UnitName       coreunit.Name
	CloudContainer *CloudContainer
	Password       *PasswordInfo
	UnitStatusArg
}

// UnitStatusArg contains parameters for updating a unit status in state.
type UnitStatusArg struct {
	AgentStatus    UnitAgentStatusInfo
	WorkloadStatus UnitWorkloadStatusInfo
}

// StatusInfo holds details about the status of an entity.
type StatusInfo struct {
	Message string
	Data    map[string]string
	Since   time.Time
}

// CloudContainerStatusStatusInfo holds a cloud container status
// and associated information.
type CloudContainerStatusStatusInfo struct {
	StatusID CloudContainerStatusType
	StatusInfo
}

// UnitAgentStatusInfo holds a unit agent status
// and associated information.
type UnitAgentStatusInfo struct {
	StatusID UnitAgentStatusType
	StatusInfo
}

// UnitWorkloadStatusInfo holds a unit workload status
// and associated information.
type UnitWorkloadStatusInfo struct {
	StatusID UnitWorkloadStatusType
	StatusInfo
}

// ContainerImageMetadata contains the access information for an OCI image resource.
type ContainerImageMetadata struct {
	// StorageKey is the key used to look-up the metadata in state.
	StorageKey string

	// RegistryPath holds the image name (including host) of the image in the
	// oci registry.
	RegistryPath string

	// Username holds the username used to gain access to a non-public image.
	Username string

	// Password holds the password used to gain access to a non-public image.
	Password string
}

// CharmDownloadInfo contains parameters for downloading a charm.
type CharmDownloadInfo struct {
	CharmUUID charm.ID
	Name      string
	Origin    charm.Origin
}

// ResolveCharmDownload contains parameters for resolving a charm download.
type ResolveCharmDownload struct {
	CharmUUID charm.ID
	Path      string
	Size      int64
}
