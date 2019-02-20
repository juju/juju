// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile

import (
	"github.com/juju/errors"
	apicharms "github.com/juju/juju/api/charms"
	charm "gopkg.in/juju/charm.v6"
)

//go:generate mockgen -package lxdprofile_test -destination lxdprofile_mock_test.go gopkg.in/juju/charm.v6 LXDProfiler

// ValidateCharmLXDProfile will attempt to validate a charm.Charm
// lxd profile. The LXDProfile is an optional method on the charm.Charm, so
// testing to check that it conforms to a LXDProfiler first is required.
// Failure to conform to the LXDProfiler will return no error.
func ValidateCharmLXDProfile(ch charm.Charm) error {
	// Check if the charm conforms to the LXDProfiler, as it's optional and in
	// theory the charm.Charm doesn't have to provide a LXDProfile method we
	// can ignore it if it's missing and assume it is therefore valid.
	if profiler, ok := ch.(charm.LXDProfiler); ok {
		return ValidateLXDProfile(profiler)
	}
	return nil
}

// ValidateLXDProfile will validate the profile to determin if the configuration
// is valid or not before passing continuing on.
func ValidateLXDProfile(profiler charm.LXDProfiler) error {
	// Profile from the api could be nil, so check that it isn't
	if profile := profiler.LXDProfile(); profile != nil {
		err := profile.ValidateConfigDevices()
		return errors.Trace(err)
	}
	return nil
}

// ValidateCharmInfoLXDProfile will validate the charm info to determin if the
// information provided is valid or not.
func ValidateCharmInfoLXDProfile(info *apicharms.CharmInfo) error {
	if profile := info.LXDProfile; profile != nil {
		err := profile.ValidateConfigDevices()
		return errors.Trace(err)
	}
	return nil
}

// NotEmpty will return false if the profiler containers a profile, that is
// empty. If the profile is empty, we'll return false.
// If there is no valid profile in the profiler, it will return false
func NotEmpty(profiler charm.LXDProfiler) bool {
	if profile := profiler.LXDProfile(); profile != nil {
		return !profile.Empty()
	}
	return false
}
