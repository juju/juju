// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/juju/domain/charm"
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
			return charm.Manifest{}, fmt.Errorf("cannot decode channel: %w", err)
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
		return charm.Channel{}, fmt.Errorf("cannot decode risk: %w", err)
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
		return "", fmt.Errorf("unknown risk %q", risk)
	}
}
