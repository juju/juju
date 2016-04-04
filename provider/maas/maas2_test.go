// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/gomaasapi"
)

type fakeController struct {
	gomaasapi.Controller
	bootResources      []gomaasapi.BootResource
	bootResourcesError error
}

func (c *fakeController) BootResources() ([]gomaasapi.BootResource, error) {
	if c.bootResourcesError != nil {
		return nil, c.bootResourcesError
	}
	return c.bootResources, nil
}

type fakeBootResource struct {
	gomaasapi.BootResource
	name         string
	architecture string
}

func (r *fakeBootResource) Name() string {
	return r.name
}

func (r *fakeBootResource) Architecture() string {
	return r.architecture
}

type fakeMachine struct {
	gomaasapi.Machine
	zoneName      string
	hostname      string
	systemID      string
	ipAddresses   []string
	statusName    string
	statusMessage string
}

func (m *fakeMachine) SystemID() string {
	return m.systemID
}

func (m *fakeMachine) Hostname() string {
	return m.hostname
}
