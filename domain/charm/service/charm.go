// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"

	"github.com/juju/juju/domain/charm"
	internalcharm "github.com/juju/juju/internal/charm"
)

// encodeCharm encodes a charm to the service representation.
// Returns an error if the charm metadata cannot be encoded.
func encodeCharm(ch internalcharm.Charm) (charm.Charm, error) {
	metadata, err := encodeMetadata(ch.Meta())
	if err != nil {
		return charm.Charm{}, fmt.Errorf("encode metadata: %w", err)
	}

	manifest, err := encodeManifest(ch.Manifest())
	if err != nil {
		return charm.Charm{}, fmt.Errorf("encode manifest: %w", err)
	}

	actions, err := encodeActions(ch.Actions())
	if err != nil {
		return charm.Charm{}, fmt.Errorf("encode actions: %w", err)
	}

	config, err := encodeConfig(ch.Config())
	if err != nil {
		return charm.Charm{}, fmt.Errorf("encode config: %w", err)
	}

	var profile []byte
	if lxdProfile, ok := ch.(internalcharm.LXDProfiler); ok {
		profile, err = encodeLXDProfile(lxdProfile.LXDProfile())
		if err != nil {
			return charm.Charm{}, fmt.Errorf("encode lxd profile: %w", err)
		}
	}

	return charm.Charm{
		Metadata:   metadata,
		Manifest:   manifest,
		Actions:    actions,
		Config:     config,
		LXDProfile: profile,
	}, nil
}
