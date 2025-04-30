// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/internal/errors"
)

func decodeCharmLocator(c CharmLocatorDetails) (charm.CharmLocator, error) {
	source, err := decodeCharmSource(c.CharmSourceID)
	if err != nil {
		return charm.CharmLocator{}, errors.Errorf("decoding charm source: %w", err)
	}

	architecture, err := decodeArchitecture(c.CharmArchitectureID)
	if err != nil {
		return charm.CharmLocator{}, errors.Errorf("decoding architecture: %w", err)
	}

	return charm.CharmLocator{
		Name:         c.CharmReferenceName,
		Revision:     c.CharmRevision,
		Source:       source,
		Architecture: architecture,
	}, nil
}

func decodeCharmSource(source int) (charm.CharmSource, error) {
	switch source {
	case 1:
		return charm.CharmHubSource, nil
	case 0:
		return charm.LocalSource, nil
	default:
		return "", errors.Errorf("unsupported charm source: %d", source)
	}
}

func decodeArchitecture(arch sql.NullInt64) (architecture.Architecture, error) {
	if !arch.Valid {
		return architecture.Unknown, nil
	}

	switch arch.Int64 {
	case 0:
		return architecture.AMD64, nil
	case 1:
		return architecture.ARM64, nil
	case 2:
		return architecture.PPC64EL, nil
	case 3:
		return architecture.S390X, nil
	case 4:
		return architecture.RISCV64, nil
	default:
		return -1, errors.Errorf("unsupported architecture: %d", arch.Int64)
	}
}

func decodePlatform(channel string, os, arch sql.NullInt64) (deployment.Platform, error) {
	osType, err := decodeOSType(os)
	if err != nil {
		return deployment.Platform{}, errors.Errorf("decoding os type: %w", err)
	}

	archType, err := decodeArchitecture(arch)
	if err != nil {
		return deployment.Platform{}, errors.Errorf("decoding architecture: %w", err)
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

	riskType, err := decodeRisk(risk.V)
	if err != nil {
		return nil, errors.Errorf("decoding risk: %w", err)
	}

	return &deployment.Channel{
		Track:  track,
		Risk:   riskType,
		Branch: branch,
	}, nil
}

func decodeRisk(risk string) (deployment.ChannelRisk, error) {
	switch risk {
	case "stable":
		return deployment.RiskStable, nil
	case "candidate":
		return deployment.RiskCandidate, nil
	case "beta":
		return deployment.RiskBeta, nil
	case "edge":
		return deployment.RiskEdge, nil
	default:
		return "", errors.Errorf("unknown risk %q", risk)
	}
}

func decodeOSType(osType sql.NullInt64) (deployment.OSType, error) {
	if !osType.Valid {
		return 0, errors.Errorf("os type is null")
	}

	switch osType.Int64 {
	case 0:
		return deployment.Ubuntu, nil
	default:
		return -1, errors.Errorf("unknown os type %v", osType)
	}
}
