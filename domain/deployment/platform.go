// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployment

import (
	"strings"

	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/internal/errors"
)

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
	Unknown OSType = iota - 1
	Ubuntu
)

func (o OSType) String() string {
	switch o {
	case Ubuntu:
		return "ubuntu"
	default:
		return "unknown"
	}
}

// Platform contains parameters for an application's platform.
type Platform struct {
	Channel      string
	OSType       OSType
	Architecture Architecture
}

// Architecture represents the architecture of a application charm.
type Architecture = architecture.Architecture

// EncodeOSType converts an OS type name string into a deployment OSType.
// The snap base name "ubuntu-core" is normalized to Ubuntu, since
// snap-confined controllers may surface it in place of the host OS name.
func EncodeOSType(os string) (OSType, error) {
	if strings.EqualFold(os, "ubuntu-core") {
		return Ubuntu, nil
	}

	switch ostype.OSTypeForName(os) {
	case ostype.Ubuntu:
		return Ubuntu, nil
	default:
		return Unknown, errors.Errorf("unknown os type %q, expected ubuntu", os)
	}
}
