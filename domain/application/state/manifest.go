// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/errors"
)

const (
	// Default architecture ID used when no architecture is specified.
	// The default to Juju is amd64.
	defaultArchitectureID = 0
)

// decodeManifest decodes the given manifests into a charm.Manifest.
// It should respect the order of the bases in the manifest. Although the
// order of architectures within a base is not guaranteed.
func decodeManifest(manifests []charmManifest) (charm.Manifest, error) {
	bases := make(map[int]charm.Base)

	var largestIndex int
	for _, base := range manifests {
		channel, err := decodeManifestChannel(base)
		if err != nil {
			return charm.Manifest{}, errors.Errorf("cannot decode channel: %w", err)
		}

		if b, ok := bases[base.Index]; ok {
			b.Architectures = append(b.Architectures, base.Architecture)
			bases[base.Index] = b

			continue
		}

		var architectures []string
		if base.Architecture != "" {
			architectures = append(architectures, base.Architecture)
		}

		bases[base.Index] = charm.Base{
			Name:          base.OS,
			Channel:       channel,
			Architectures: architectures,
		}

		if base.Index > largestIndex {
			largestIndex = base.Index
		}
	}

	if len(bases) == 0 {
		return charm.Manifest{}, nil
	}

	// Convert the map into a slice using the largest index as the length. This
	// means that we preserved the order of the bases even faced with holes in
	// the array.
	result := make([]charm.Base, largestIndex+1)
	for index, base := range bases {
		result[index] = base
	}

	return charm.Manifest{
		Bases: result,
	}, nil
}

// decodeManifestChannel decodes the given base into a charm.Channel.
func decodeManifestChannel(base charmManifest) (charm.Channel, error) {
	risk, err := decodeManifestChannelRisk(base.Risk)
	if err != nil {
		return charm.Channel{}, errors.Errorf("cannot decode risk: %w", err)
	}

	return charm.Channel{
		Track:  base.Track,
		Risk:   risk,
		Branch: base.Branch,
	}, nil
}

// decodeManifestChannelRisk decodes the given risk into a charm.ChannelRisk.
func decodeManifestChannelRisk(risk string) (charm.ChannelRisk, error) {
	switch risk {
	case "stable":
		return charm.RiskStable, nil
	case "candidate":
		return charm.RiskCandidate, nil
	case "beta":
		return charm.RiskBeta, nil
	case "edge":
		return charm.RiskEdge, nil
	default:
		return "", errors.Errorf("unknown risk %q", risk)
	}
}

func encodeManifest(id corecharm.ID, manifest charm.Manifest) ([]setCharmManifest, error) {
	result := make([]setCharmManifest, 0, len(manifest.Bases))
	for index, base := range manifest.Bases {
		encodedRisk, err := encodeManifestChannelRisk(base.Channel.Risk)
		if err != nil {
			return nil, errors.Errorf("cannot encode risk: %w", err)
		}

		encodedOS, err := encodeManifestOS(base.Name)
		if err != nil {
			return nil, errors.Errorf("cannot encode OS: %w", err)
		}

		// No architectures specified, use the default.
		if len(base.Architectures) == 0 {
			result = append(result, setCharmManifest{
				CharmUUID:      id.String(),
				Index:          index,
				NestedIndex:    0,
				OSID:           encodedOS,
				ArchitectureID: defaultArchitectureID,
				Track:          base.Channel.Track,
				Risk:           encodedRisk,
				Branch:         base.Channel.Branch,
			})
			continue
		}

		for i, architecture := range base.Architectures {
			encodedArch, err := encodeManifestArchitecture(architecture)
			if err != nil {
				return nil, errors.Errorf("cannot encode architecture: %w", err)
			}
			result = append(result, setCharmManifest{
				CharmUUID:      id.String(),
				Index:          index,
				NestedIndex:    i,
				OSID:           encodedOS,
				ArchitectureID: encodedArch,
				Track:          base.Channel.Track,
				Risk:           encodedRisk,
				Branch:         base.Channel.Branch,
			})
		}
	}
	return result, nil
}

func encodeManifestChannelRisk(risk charm.ChannelRisk) (string, error) {
	switch risk {
	case charm.RiskStable:
		return "stable", nil
	case charm.RiskCandidate:
		return "candidate", nil
	case charm.RiskBeta:
		return "beta", nil
	case charm.RiskEdge:
		return "edge", nil
	default:
		return "", errors.Errorf("unknown risk %q", risk)
	}
}

func encodeManifestOS(os string) (int, error) {
	switch os {
	case "ubuntu":
		return 0, nil
	default:
		return -1, errors.Errorf("unknown OS %q", os)
	}
}

func encodeManifestArchitecture(architecture string) (int, error) {
	switch architecture {
	case "amd64":
		return 0, nil
	case "arm64":
		return 1, nil
	case "ppc64el":
		return 2, nil
	case "s390x":
		return 3, nil
	case "riscv64":
		return 4, nil
	default:
		return -1, errors.Errorf("unknown architecture %q", architecture)
	}
}
