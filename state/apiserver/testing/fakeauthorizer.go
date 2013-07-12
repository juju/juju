// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

// FakeAuthorizer implements the common.Authorizer interface.
type FakeAuthorizer struct {
	Tag          string
	LoggedIn     bool
	Manager      bool
	MachineAgent bool
	Client       bool
}

func (fa FakeAuthorizer) AuthOwner(tag string) bool {
	return fa.Tag == tag
}

func (fa FakeAuthorizer) AuthEnvironManager() bool {
	return fa.Manager
}

func (fa FakeAuthorizer) AuthMachineAgent() bool {
	return fa.MachineAgent
}

func (fa FakeAuthorizer) AuthClient() bool {
	return fa.Client
}

func (fa FakeAuthorizer) GetAuthTag() string {
	return fa.Tag
}
