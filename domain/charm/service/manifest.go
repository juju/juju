// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"

	"github.com/juju/juju/domain/charm"
	internalcharm "github.com/juju/juju/internal/charm"
)

// Conversion code is used to convert charm.Manifest code to non-domain
// charm.Manifest code. The domain charm.Manifest code is used as the
// normalisation layer for charm manifest. The persistence layer will ensure
// that the charm manifest is stored in the correct format.

func convertManifest(manifest charm.Manifest) (internalcharm.Manifest, error) {
	bases, err := convertManifestBases(manifest.Bases)
	if err != nil {
		return internalcharm.Manifest{}, fmt.Errorf("convert bases: %w", err)
	}

	return internalcharm.Manifest{
		Bases: bases,
	}, nil
}

func convertManifestBases(bases []charm.Base) ([]internalcharm.Base, error) {
	var converted []internalcharm.Base
	for _, base := range bases {
		convertedBase, err := convertManifestBase(base)
		if err != nil {
			return nil, fmt.Errorf("convert base: %w", err)
		}
		converted = append(converted, convertedBase)
	}
	return converted, nil
}

func convertManifestBase(base charm.Base) (internalcharm.Base, error) {
	channel, err := convertManifestChannel(base.Channel)
	if err != nil {
		return internalcharm.Base{}, fmt.Errorf("convert channel: %w", err)
	}

	return internalcharm.Base{
		Name:          base.Name,
		Channel:       channel,
		Architectures: base.Architectures,
	}, nil
}

func convertManifestChannel(channel charm.Channel) (internalcharm.Channel, error) {
	risk, err := convertManifestRisk(channel.Risk)
	if err != nil {
		return internalcharm.Channel{}, fmt.Errorf("convert risk: %w", err)
	}

	return internalcharm.Channel{
		Track:  channel.Track,
		Risk:   risk,
		Branch: channel.Branch,
	}, nil
}

func convertManifestRisk(risk charm.ChannelRisk) (internalcharm.Risk, error) {
	switch risk {
	case charm.RiskStable:
		return internalcharm.Stable, nil
	case charm.RiskCandidate:
		return internalcharm.Candidate, nil
	case charm.RiskBeta:
		return internalcharm.Beta, nil
	case charm.RiskEdge:
		return internalcharm.Edge, nil
	default:
		return "", fmt.Errorf("unknown risk: %q", risk)
	}
}
