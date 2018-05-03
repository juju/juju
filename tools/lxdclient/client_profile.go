// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd/shared/api"
)

type rawProfileClient interface {
	CreateProfile(profile api.ProfilesPost) (err error)
	GetProfileNames() (names []string, err error)
	DeleteProfile(name string) (err error)
	GetProfile(name string) (profile *api.Profile, ETag string, err error)
	UpdateProfile(name string, profile api.ProfilePut, ETag string) (err error)
}

type profileClient struct {
	raw rawProfileClient
}

// ProfileDelete deletes an existing profile. No check is made to
// verify the profile exists.
func (p profileClient) ProfileDelete(profile string) error {
	if err := p.raw.DeleteProfile(profile); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// CreateProfile attempts to create a new lxc profile and set the given config.
func (p profileClient) CreateProfile(name string, config map[string]string) error {
	req := api.ProfilesPost{
		Name: name,
		ProfilePut: api.ProfilePut{
			Config: config,
		},
	}
	if err := p.raw.CreateProfile(req); err != nil {
		//TODO(wwitzel3) use HasProfile to generate a more useful AlreadyExists error
		return errors.Trace(err)
	}
	return nil
}

// HasProfile returns true/false if the profile exists.
func (p profileClient) HasProfile(name string) (bool, error) {
	profiles, err := p.raw.GetProfileNames()
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, profile := range profiles {
		if profile == name {
			return true, nil
		}
	}
	return false, nil
}

// SetProfileConfigItem updates the given profile config key to the given value.
func (p profileClient) SetProfileConfigItem(name, key, value string) error {
	profile, _, err := p.raw.GetProfile(name)
	if err != nil {
		errors.Trace(err)
	}
	profile.Config[key] = value

	if err := p.raw.UpdateProfile(name, profile.Writable(), ""); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// GetProfileConfig returns a map with all keys and values for the given
// profile.
func (p profileClient) GetProfileConfig(name string) (map[string]string, error) {
	profile, _, err := p.raw.GetProfile(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return profile.Config, nil
}

func (p profileClient) ProfileConfig(name string) (*api.Profile, error) {
	profile, _, err := p.raw.GetProfile(name)
	return profile, err
}
