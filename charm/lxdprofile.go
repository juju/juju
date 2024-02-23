// Copyright 2018 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

// LXDProfiler defines a way to access a LXDProfile from a charm.
type LXDProfiler interface {
	// LXDProfile returns the LXDProfile found in lxd-profile.yaml of the charm
	LXDProfile() *LXDProfile
}

// LXDProfile is the same as ProfilePut defined in github.com/lxc/lxd/shared/api/profile.go
type LXDProfile struct {
	Config      map[string]string            `json:"config" yaml:"config"`
	Description string                       `json:"description" yaml:"description"`
	Devices     map[string]map[string]string `json:"devices" yaml:"devices"`
}

// NewLXDProfile creates a LXDProfile with the internal data structures
// initialised  to non nil values.
func NewLXDProfile() *LXDProfile {
	return &LXDProfile{
		Config:  map[string]string{},
		Devices: map[string]map[string]string{},
	}
}

// ReadLXDProfile reads in a LXDProfile from a charm's lxd-profile.yaml.
// It is not validated at this point so that the caller can choose to override
// any validation.
func ReadLXDProfile(r io.Reader) (*LXDProfile, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	profile := NewLXDProfile()
	if err := yaml.Unmarshal(data, profile); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshall lxd-profile.yaml")
	}
	return profile, nil
}

// ValidateConfigDevices validates the Config and Devices properties of the LXDProfile.
// WhiteList devices: unix-char, unix-block, gpu, usb.
// BlackList config: boot*, limits* and migration*.
// An empty profile will not return an error.
func (profile *LXDProfile) ValidateConfigDevices() error {
	for _, val := range profile.Devices {
		goodDevs := set.NewStrings("unix-char", "unix-block", "gpu", "usb")
		if devType, ok := val["type"]; ok {
			if !goodDevs.Contains(devType) {
				return fmt.Errorf("invalid lxd-profile.yaml: contains device type %q", devType)
			}
		}
	}
	for key := range profile.Config {
		if strings.HasPrefix(key, "boot") ||
			strings.HasPrefix(key, "limits") ||
			strings.HasPrefix(key, "migration") {
			return fmt.Errorf("invalid lxd-profile.yaml: contains config value %q", key)
		}
	}
	return nil
}

// Empty returns true if neither devices nor config have been defined in the profile.
func (profile *LXDProfile) Empty() bool {
	return len(profile.Devices) < 1 && len(profile.Config) < 1
}
