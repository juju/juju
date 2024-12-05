// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"encoding/json"

	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

func decodeLXDProfile(profile []byte) (internalcharm.LXDProfile, error) {
	if len(profile) == 0 {
		return internalcharm.LXDProfile{}, nil
	}

	var result lxdProfile
	if err := json.Unmarshal(profile, &result); err != nil {
		return internalcharm.LXDProfile{}, errors.Errorf("unmarshal lxd profile: %w", err)
	}

	return internalcharm.LXDProfile{
		Config:      result.Config,
		Description: result.Description,
		Devices:     result.Devices,
	}, nil
}

func encodeLXDProfile(profile *internalcharm.LXDProfile) ([]byte, error) {
	if profile == nil || (profile.Empty() && profile.Description == "") {
		return nil, nil
	}

	result, err := json.Marshal(lxdProfile{
		Config:      profile.Config,
		Description: profile.Description,
		Devices:     profile.Devices,
	})
	if err != nil {
		return nil, errors.Errorf("marshal lxd profile: %w", err)
	}

	return result, nil
}

type lxdProfile struct {
	Config      map[string]string            `json:"config,omitempty"`
	Description string                       `json:"description,omitempty"`
	Devices     map[string]map[string]string `json:"devices,omitempty"`
}
