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

type ProfileCharm interface {
	LXDProfile() *charm.LXDProfile
	Meta() *charm.Meta
	Revision() int
}

type profileMachineShim struct {
	*state.Machine
}

//go:generate mockgen -package mocks -destination mocks/profile_mock.go github.com/juju/juju/apiserver/facades/agent/provisioner ProfileMachine,ProfileBackend,ProfileCharm
type ProfileMachine interface {
	CharmProfiles() ([]string, error)
	ModelName() string
	Id() string
}
