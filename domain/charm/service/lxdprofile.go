// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"encoding/json"
	"fmt"

	internalcharm "github.com/juju/juju/internal/charm"
)

func decodeLXDProfile(profile []byte) (internalcharm.LXDProfile, error) {
	if len(profile) == 0 {
		return internalcharm.LXDProfile{}, nil
	}

	var result internalcharm.LXDProfile
	if err := json.Unmarshal(profile, &result); err != nil {
		return result, fmt.Errorf("unmarshal lxd profile: %w", err)
	}

	return result, nil
}
