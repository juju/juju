// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

// FakeAuthorizer implements the common.Authorizer interface.
type FakeAuthorizer struct {
	Tag            names.Tag
	LoggedIn       bool
	EnvironManager bool
	MachineAgent   bool
	UnitAgent      bool
	Client         bool
	Entity         state.Entity
}

func (fa FakeAuthorizer) AuthOwner(tag string) bool {
	return fa.Tag == mustParseTag(tag)
}

// temporary method until common/Authorizer.AuthOwner takes a names.Tag not a string.
func mustParseTag(tag string) names.Tag {
	t, err := names.ParseTag(tag)
	if err != nil {
		panic(err)
	}
	return t
}

func (fa FakeAuthorizer) AuthEnvironManager() bool {
	return fa.EnvironManager
}

func (fa FakeAuthorizer) AuthMachineAgent() bool {
	return fa.MachineAgent
}

func (fa FakeAuthorizer) AuthUnitAgent() bool {
	return fa.UnitAgent
}

func (fa FakeAuthorizer) AuthClient() bool {
	return fa.Client
}

func (fa FakeAuthorizer) GetAuthTag() names.Tag {
	return fa.Tag
}

func (fa FakeAuthorizer) GetAuthEntity() state.Entity {
	return fa.Entity
}
