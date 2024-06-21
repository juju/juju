// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"

	"github.com/juju/juju/domain/charm"
	internalcharm "github.com/juju/juju/internal/charm"
)

// Conversion code is used to decode charm.Manifest code to non-domain
// charm.Manifest code. The domain charm.Manifest code is used as the
// normalisation layer for charm manifest. The persistence layer will ensure
// that the charm manifest is stored in the correct format.

func decodeManifest(manifest charm.Manifest) (internalcharm.Manifest, error) {
	bases, err := decodeManifestBases(manifest.Bases)
	if err != nil {
		return internalcharm.Manifest{}, fmt.Errorf("decode bases: %w", err)
	}

	return internalcharm.Manifest{
		Bases: bases,
	}, nil
}

func decodeManifestBases(bases []charm.Base) ([]internalcharm.Base, error) {
	var decodeed []internalcharm.Base
	for _, base := range bases {
		decodeedBase, err := decodeManifestBase(base)
		if err != nil {
			return nil, fmt.Errorf("decode base: %w", err)
		}
		decodeed = append(decodeed, decodeedBase)
	}
	return decodeed, nil
}

func decodeManifestBase(base charm.Base) (internalcharm.Base, error) {
	channel, err := decodeManifestChannel(base.Channel)
	if err != nil {
		return internalcharm.Base{}, fmt.Errorf("decode channel: %w", err)
	}

	return internalcharm.Base{
		Name:          base.Name,
		Channel:       channel,
		Architectures: base.Architectures,
	}, nil
}

func decodeManifestChannel(channel charm.Channel) (internalcharm.Channel, error) {
	risk, err := decodeManifestRisk(channel.Risk)
	if err != nil {
		return internalcharm.Channel{}, fmt.Errorf("decode risk: %w", err)
	}

	return internalcharm.Channel{
		Track:  channel.Track,
		Risk:   risk,
		Branch: channel.Branch,
	}, nil
}

func decodeManifestRisk(risk charm.ChannelRisk) (internalcharm.Risk, error) {
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
