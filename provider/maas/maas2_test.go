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
	machines           []gomaasapi.Machine
	machinesError      error
	machinesArgsCheck  func(gomaasapi.MachinesArgs)
}

func (c *fakeController) Machines(args gomaasapi.MachinesArgs) ([]gomaasapi.Machine, error) {
	if c.machinesArgsCheck != nil {
		c.machinesArgsCheck(args)
	}
	if c.machinesError != nil {
		return nil, c.machinesError
	}
	return c.machines, nil
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

func (m *fakeMachine) IPAddresses() []string {
	return m.ipAddresses
}

func (m *fakeMachine) StatusName() string {
	return m.statusName
}

func (m *fakeMachine) StatusMessage() string {
	return m.statusMessage
}

func (m *fakeMachine) Zone() gomaasapi.Zone {
	return fakeZone{name: m.zoneName}
}

type fakeZone struct {
	gomaasapi.Zone
	name string
}

func (z fakeZone) Name() string {
	return z.name
}
