// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd/shared/api"
)

type rawProfileClient interface {
	ProfileCreate(name string) error
	ListProfiles() ([]api.Profile, error)
	SetProfileConfigItem(profile, key, value string) error
	GetProfileConfig(profile string) (map[string]string, error)
	ProfileDelete(profile string) error
	ProfileDeviceAdd(profile, devname, devtype string, props []string) (*api.Response, error)
	ProfileConfig(profile string) (*api.Profile, error)
}

type profileClient struct {
	raw rawProfileClient
}

// ProfileDelete deletes an existing profile. No check is made to
// verify the profile exists.
func (p profileClient) ProfileDelete(profile string) error {
	if err := p.raw.ProfileDelete(profile); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ProfileDeviceAdd adds a profile device, such as a disk or a nic, to
// the specified profile. No check is made to verify the profile
// exists.
func (p profileClient) ProfileDeviceAdd(profile, devname, devtype string, props []string) (*api.Response, error) {
	resp, err := p.raw.ProfileDeviceAdd(profile, devname, devtype, props)
	if err != nil {
		return resp, errors.Trace(err)
	}
	return resp, err
}

// CreateProfile attempts to create a new lxc profile and set the given config.
func (p profileClient) CreateProfile(name string, config map[string]string) error {
	if err := p.raw.ProfileCreate(name); err != nil {
		//TODO(wwitzel3) use HasProfile to generate a more useful AlreadyExists error
		return errors.Trace(err)
	}

	for k, v := range config {
		if err := p.raw.SetProfileConfigItem(name, k, v); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// HasProfile returns true/false if the profile exists.
func (p profileClient) HasProfile(name string) (bool, error) {
	profiles, err := p.raw.ListProfiles()
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, profile := range profiles {
		if profile.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// SetProfileConfigItem updates the given profile config key to the given value.
func (p profileClient) SetProfileConfigItem(profile, key, value string) error {
	if err := p.raw.SetProfileConfigItem(profile, key, value); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// GetProfileConfig returns a map with all keys and values for the given
// profile.
func (p profileClient) GetProfileConfig(profile string) (map[string]string, error) {
	config, err := p.raw.GetProfileConfig(profile)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return config, nil
}

func (p profileClient) ProfileConfig(profile string) (*api.Profile, error) {
	return p.raw.ProfileConfig(profile)
}
