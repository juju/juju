// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile

import (
	"strings"

	"github.com/juju/collections/set"

	"github.com/juju/juju/internal/errors"
)

type LXDProfiles struct {
	Profile Profile
}

// LXDProfile implements LXDProfiler interface.
func (p LXDProfiles) LXDProfile() LXDProfile {
	return p.Profile
}

// ProfilePost is a close representation of lxd api
// ProfilesPost
type ProfilePost struct {
	Name    string
	Profile *Profile
}

// Profile is a representation of charm.v6 LXDProfile
type Profile struct {
	Config      map[string]string
	Description string
	Devices     map[string]map[string]string
}

// Empty implements LXDProfile interface.
func (p Profile) Empty() bool {
	return len(p.Devices) < 1 && len(p.Config) < 1
}

// ValidateConfigDevices implements LXDProfile interface.
func (p Profile) ValidateConfigDevices() error {
	for _, val := range p.Devices {
		goodDevs := set.NewStrings("unix-char", "unix-block", "gpu", "usb")
		if devType, ok := val["type"]; ok {
			if !goodDevs.Contains(devType) {
				return errors.Errorf("invalid lxd-profile: contains device type %q", devType)
			}
		}
	}
	for key := range p.Config {
		if strings.HasPrefix(key, "boot") ||
			strings.HasPrefix(key, "limits") ||
			strings.HasPrefix(key, "migration") {
			return errors.Errorf("invalid lxd-profile: contains config value %q", key)
		}
	}
	return nil
}
