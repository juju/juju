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

/*

// AuthMachineAgent returns whether the current client is a machine agent.
func (r *srvRoot) AuthMachineAgent() bool {
        _, isMachine := r.GetAuthTag().(names.MachineTag)
        return isMachine
}

// AuthUnitAgent returns whether the current client is a unit agent.
func (r *srvRoot) AuthUnitAgent() bool {
        _, isUnit := r.GetAuthTag().(names.UnitTag)
        return isUnit
}

// AuthOwner returns whether the authenticated user's tag matches the
// given entity tag.
func (r *srvRoot) AuthOwner(tag string) bool {
        return r.GetAuthTag().String() == tag
}

// AuthEnvironManager returns whether the authenticated user is a
// machine with running the ManageEnviron job.
func (r *srvRoot) AuthEnvironManager() bool {
        return isMachineWithJob(r.entity, state.JobManageEnviron)
}

*/

func (fa FakeAuthorizer) AuthEnvironManager() bool {
	return fa.EnvironManager
}

func (fa FakeAuthorizer) AuthMachineAgent() bool {
	return fa.MachineAgent
}

func (fa FakeAuthorizer) AuthUnitAgent() bool {
	return fa.UnitAgent
}

// AuthClient returns whether the authenticated entity is a client
// user.
func (fa FakeAuthorizer) AuthClient() bool {
	_, isUser := fa.GetAuthTag().(names.UserTag)
	return isUser
}

func (fa FakeAuthorizer) GetAuthTag() names.Tag {
	return fa.Tag
}

func (fa FakeAuthorizer) GetAuthEntity() state.Entity {
	return fa.Entity
}
