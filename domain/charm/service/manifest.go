// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"

	"github.com/juju/juju/domain/charm"
	charmerrors "github.com/juju/juju/domain/charm/errors"
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

func encodeManifest(manifest *internalcharm.Manifest) (charm.Manifest, error) {
	if manifest == nil {
		return charm.Manifest{}, charmerrors.ManifestNotValid
	}

	bases, err := encodeManifestBases(manifest.Bases)
	if err != nil {
		return charm.Manifest{}, fmt.Errorf("encode bases: %w", err)
	}

	return charm.Manifest{
		Bases: bases,
	}, nil
}

func encodeManifestBases(bases []internalcharm.Base) ([]charm.Base, error) {
	var encoded []charm.Base
	for _, base := range bases {
		encodedBase, err := encodeManifestBase(base)
		if err != nil {
			return nil, fmt.Errorf("encode base: %w", err)
		}
		encoded = append(encoded, encodedBase)
	}
	return encoded, nil
}

func encodeManifestBase(base internalcharm.Base) (charm.Base, error) {
	channel, err := encodeManifestChannel(base.Channel)
	if err != nil {
		return charm.Base{}, fmt.Errorf("encode channel: %w", err)
	}

	return charm.Base{
		Name:          base.Name,
		Channel:       channel,
		Architectures: base.Architectures,
	}, nil
}

func encodeManifestChannel(channel internalcharm.Channel) (charm.Channel, error) {
	risk, err := encodeManifestRisk(channel.Risk)
	if err != nil {
		return charm.Channel{}, fmt.Errorf("encode risk: %w", err)
	}

	return charm.Channel{
		Track:  channel.Track,
		Risk:   risk,
		Branch: channel.Branch,
	}, nil
}

func encodeManifestRisk(risk internalcharm.Risk) (charm.ChannelRisk, error) {
	switch risk {
	case internalcharm.Stable:
		return charm.RiskStable, nil
	case internalcharm.Candidate:
		return charm.RiskCandidate, nil
	case internalcharm.Beta:
		return charm.RiskBeta, nil
	case internalcharm.Edge:
		return charm.RiskEdge, nil
	default:
		return "", fmt.Errorf("unknown risk: %q", risk)
	}
}
