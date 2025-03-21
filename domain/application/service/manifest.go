// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"

	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// Conversion code is used to decode charm.Manifest code to non-domain
// charm.Manifest code. The domain charm.Manifest code is used as the
// normalisation layer for charm manifest. The persistence layer will ensure
// that the charm manifest is stored in the correct format.

func decodeManifest(manifest charm.Manifest) (internalcharm.Manifest, error) {
	bases, err := decodeManifestBases(manifest.Bases)
	if err != nil {
		return internalcharm.Manifest{}, errors.Errorf("decode bases: %w", err)
	}

	return internalcharm.Manifest{
		Bases: bases,
	}, nil
}

func decodeManifestBases(bases []charm.Base) ([]internalcharm.Base, error) {
	var decoded []internalcharm.Base
	for _, base := range bases {
		decodedBase, err := decodeManifestBase(base)
		if err != nil {
			return nil, errors.Errorf("decode base: %w", err)
		}
		decoded = append(decoded, decodedBase)
	}
	return decoded, nil
}

func decodeManifestBase(base charm.Base) (internalcharm.Base, error) {
	channel, err := decodeManifestChannel(base.Channel)
	if err != nil {
		return internalcharm.Base{}, errors.Errorf("decode channel: %w", err)
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
		return internalcharm.Channel{}, errors.Errorf("decode risk: %w", err)
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
		return "", errors.Errorf("unknown risk: %q", risk)
	}
}

func encodeManifest(manifest *internalcharm.Manifest) (charm.Manifest, []string, error) {
	if manifest == nil {
		return charm.Manifest{}, nil, applicationerrors.CharmManifestNotValid
	}

	bases, warnings, err := encodeManifestBases(manifest.Bases)
	if err != nil {
		return charm.Manifest{}, warnings, errors.Errorf("encode bases: %w", err)
	}

	return charm.Manifest{
		Bases: bases,
	}, warnings, nil
}

func encodeManifestBases(bases []internalcharm.Base) ([]charm.Base, []string, error) {
	var encoded []charm.Base
	var unsupportedArches []string
	for _, base := range bases {
		encodedBase, unsupported, err := encodeManifestBase(base)
		if err != nil {
			return nil, unsupportedArches, errors.Errorf("encode base: %w", err)
		}
		encoded = append(encoded, encodedBase)
		unsupportedArches = append(unsupportedArches, unsupported...)
	}
	return encoded, unsupportedArches, nil
}

func encodeManifestBase(base internalcharm.Base) (charm.Base, []string, error) {
	if base.Name == "" {
		return charm.Base{}, nil, applicationerrors.CharmBaseNameNotValid
	}
	// Juju only supports Ubuntu bases.
	baseType, err := ostype.ParseOSType(base.Name)
	if err != nil {
		return charm.Base{}, nil, errors.Errorf("parse base name: %w", err)
	} else if baseType != ostype.Ubuntu {
		return charm.Base{}, nil, applicationerrors.CharmBaseNameNotSupported
	}

	channel, err := encodeManifestChannel(base.Channel)
	if err != nil {
		return charm.Base{}, nil, errors.Errorf("encode channel: %w", err)
	}

	arches := set.NewStrings()
	unsupported := set.NewStrings()
	for _, arch := range base.Architectures {
		// Ignore empty architectures (this should be done at the wire protocol
		// level, but we do it here as well to be safe).
		if arch == "" {
			continue
		}

		// Normalise the architecture, to ensure that it is in the correct
		// format.
		arch = corearch.NormaliseArch(arch)

		// If the architecture is not supported, add it to the list of
		if !corearch.IsSupportedArch(arch) {
			unsupported.Add(arch)
			continue
		}

		arches.Add(arch)
	}

	var warnings []string
	if unsupported.Size() > 0 {
		arches := strings.Join(unsupported.SortedValues(), ", ")
		warnings = append(warnings, fmt.Sprintf("unsupported architectures: %s for %q with channel: %q", arches, base.Name, base.Channel.String()))
	}

	return charm.Base{
		Name:          base.Name,
		Channel:       channel,
		Architectures: arches.SortedValues(),
	}, warnings, nil
}

func encodeManifestChannel(channel internalcharm.Channel) (charm.Channel, error) {
	risk, err := encodeManifestRisk(channel.Risk)
	if err != nil {
		return charm.Channel{}, errors.Errorf("encode risk: %w", err)
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
		return "", errors.Errorf("unknown risk: %q", risk)
	}
}
