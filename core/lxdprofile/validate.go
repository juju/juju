// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile

import "github.com/juju/juju/internal/errors"

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/lxdprofile_mock.go github.com/juju/juju/core/lxdprofile LXDProfiler,LXDProfile

// LXDProfiler represents a local implementation of a charm profile.
// This point of use interface normalises a LXDProfiler, so that we can
// validate configuration devices in one location, without having to roll
// your own implementation of what is valid for a LXDProfile. With this in
// mind shims from your existing types (charm.Charm, state.Charm,
// params.CharmInfo) will probably require a shim to massage the type into
// a LXDProfiler. This cleans up the interface for validation and keeps the
// core cleaner.
type LXDProfiler interface {
	// LXDProfile returns a charm LXDProfile
	LXDProfile() LXDProfile
}

// LXDProfile represents a local implementation of a charm profile.
type LXDProfile interface {

	// ValidateConfigDevices validates the Config and Devices properties of the LXDProfile.
	// WhiteList devices: unix-char, unix-block, gpu, usb.
	// BlackList config: boot*, limits* and migration*.
	// An empty profile will not return an error.
	ValidateConfigDevices() error

	// Empty returns true if there are no configurations or devices to be
	// applied for the LXD profile.
	// Having a description but having values in the configurations/devices
	// will still return empty, as it's what should be applied.
	Empty() bool
}

// ValidateLXDProfile will validate the profile to determin if the configuration
// is valid or not before passing continuing on.
func ValidateLXDProfile(profiler LXDProfiler) error {
	// if profiler is nil, there is no available profiler to call LXDProfile
	// then return out early
	if profiler == nil {
		return nil
	}
	// Profile from the api could be nil, so check that it isn't
	if profile := profiler.LXDProfile(); profile != nil {
		err := profile.ValidateConfigDevices()
		return errors.Capture(err)
	}
	return nil
}

// NotEmpty will return false if the profiler containers a profile, that is
// empty. If the profile is empty, we'll return false.
// If there is no valid profile in the profiler, it will return false
func NotEmpty(profiler LXDProfiler) bool {
	if profile := profiler.LXDProfile(); profile != nil {
		return !profile.Empty()
	}
	return false
}
