// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd"
)

type rawProfileClient interface {
	ProfileCreate(name string) error
	ListProfiles() ([]string, error)
	SetProfileConfigItem(name, key, value string) error
	ProfileDelete(profile string) error
	ProfileDeviceAdd(profile, devname, devtype string, props []string) (*lxd.Response, error)
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
func (p profileClient) ProfileDeviceAdd(profile, devname, devtype string, props []string) (*lxd.Response, error) {
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
		if profile == name {
			return true, nil
		}
	}
	return false, nil
}
