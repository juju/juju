// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
)

type rawProfileClient interface {
	ProfileCreate(name string) error
	ListProfiles() ([]string, error)
	SetProfileConfigItem(name, key, value string) error
}

type profileClient struct {
	raw rawProfileClient
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
		if profile == name {
			return true, nil
		}
	}
	return false, nil
}
