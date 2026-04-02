// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"database/sql"

	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/internal/errors"
)

func decodeCharmLocator(c CharmLocatorDetails) (charm.CharmLocator, error) {
	arch := architecture.Unknown
	if c.CharmArchitectureID.Valid {
		arch = architecture.Architecture(c.CharmArchitectureID.V)
	}

	return charm.CharmLocator{
		Name:         c.CharmReferenceName,
		Revision:     c.CharmRevision,
		Source:       charm.CharmSource(c.CharmSource),
		Architecture: arch,
	}, nil
}

func decodePlatform(channel string, os, arch sql.Null[int64]) (deployment.Platform, error) {
	osType, err := decodeOSType(os)
	if err != nil {
		return deployment.Platform{}, errors.Errorf("decoding os type: %w", err)
	}

	archType := architecture.Unknown
	if arch.Valid {
		archType = architecture.Architecture(arch.V)
	}

	return deployment.Platform{
		Channel:      channel,
		OSType:       osType,
		Architecture: archType,
	}, nil
}

func decodeChannel(track string, risk sql.Null[string], branch string) (*deployment.Channel, error) {
	if !risk.Valid {
		return nil, nil
	}

	return &deployment.Channel{
		Track:  track,
		Risk:   deployment.ChannelRisk(risk.V),
		Branch: branch,
	}, nil
}

func decodeOSType(osType sql.Null[int64]) (deployment.OSType, error) {
	if !osType.Valid {
		return -1, nil
	}

	return deployment.OSType(osType.V), nil
}
