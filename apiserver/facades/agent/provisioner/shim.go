// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/state"
)

// ProfileBackend, ProfileMachine, and ProfileCharm are solely to facilitate
// mocks in testing CharmProfileChangeInfo

type profileBackendShim struct {
	*state.State
}

type ProfileBackend interface {
	Charm(curl *charm.URL) (ProfileCharm, error)
}

func (p *profileBackendShim) Charm(curl *charm.URL) (ProfileCharm, error) {
	return p.State.Charm(curl)
}

// TODO (hml) 2018-11-08
// Replace with containerizer.ProfileCharm interface when
// PR 9428 lands
// the NewMockCharm in containerize_mock_test.go will satisfy this interface
type ProfileCharm interface {
	LXDProfile() *charm.LXDProfile
	Meta() *charm.Meta
	Revision() int
}

type profileMachineShim struct {
	*state.Machine
}

//go:generate mockgen -package provisioner_test -destination mocks/profile_mock_test.go github.com/juju/juju/apiserver/facades/agent/provisioner ProfileMachine,ProfileBackend,ProfileCharm
type ProfileMachine interface {
	UpgradeCharmProfileApplication() (string, error)
	UpgradeCharmProfileCharmURL() (string, error)
	CharmProfiles() ([]string, error)
	ModelName() string
	Id() string
}
