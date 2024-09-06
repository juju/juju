// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	domaincharm "github.com/juju/juju/domain/application/charm"
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
	Address    *Address
}

// Origin contains parameters for an application's origin.
type Origin struct {
	Revision int
}

// CloudContainer contains parameters for a unit's cloud container.
type CloudContainer struct {
	ProviderId *string
	Address    *Address
	Ports      *[]string
}

// Address contains parameters for a cloud container address.
type Address struct {
	Value       string
	AddressType string
	Scope       string
	Origin      string
	SpaceID     string
}

// UpsertUnitArg contains parameters for adding a unit to state.
type UpsertUnitArg struct {
	UnitName       *string
	PasswordHash   *string
	CloudContainer *CloudContainer
}

// These aliases are used to specify secret filter terms.
type (
	ApplicationSecretOwners []string
	UnitSecretOwners        []string
)

// These consts are used to specify nil filter terms.
var (
	NilApplicationOwners = ApplicationSecretOwners(nil)
	NilUnitOwners        = UnitSecretOwners(nil)
)
