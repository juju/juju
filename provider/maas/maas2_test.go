// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/gomaasapi"
)

type fakeController struct {
	gomaasapi.Controller
	bootResources         []gomaasapi.BootResource
	bootResourcesError    error
	machines              []gomaasapi.Machine
	machinesError         error
	machinesArgsCheck     func(gomaasapi.MachinesArgs)
	zones                 []gomaasapi.Zone
	zonesError            error
	spaces                []gomaasapi.Space
	spacesError           error
	releaseMachinesErrors []error
	releaseMachinesArgs   []gomaasapi.ReleaseMachinesArgs
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

func (c *fakeController) Zones() ([]gomaasapi.Zone, error) {
	if c.zonesError != nil {
		return nil, c.zonesError
	}
	return c.zones, nil
}

func (c *fakeController) Spaces() ([]gomaasapi.Space, error) {
	if c.spacesError != nil {
		return nil, c.spacesError
	}
	return c.spaces, nil
}

func (c *fakeController) ReleaseMachines(args gomaasapi.ReleaseMachinesArgs) error {
	if c.releaseMachinesErrors == nil {
		return nil
	}
	c.releaseMachinesArgs = append(c.releaseMachinesArgs, args)
	err := c.releaseMachinesErrors[0]
	c.releaseMachinesErrors = c.releaseMachinesErrors[1:]
	return err
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
	cpuCount      int
	memory        int
	architecture  string
}

func (m *fakeMachine) CPUCount() int {
	return m.cpuCount
}

func (m *fakeMachine) Memory() int {
	return m.memory
}

func (m *fakeMachine) Architecture() string {
	return m.architecture
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

type fakeSpace struct {
	gomaasapi.Space
	name    string
	id      int
	subnets []gomaasapi.Subnet
}

func (s fakeSpace) Name() string {
	return s.name
}

func (s fakeSpace) ID() int {
	return s.id
}

func (s fakeSpace) Subnets() []gomaasapi.Subnet {
	return s.subnets
}

type fakeSubnet struct {
	gomaasapi.Subnet
	id      int
	vlanVid int
	cidr    string
}

func (s fakeSubnet) ID() int {
	return s.id
}

func (s fakeSubnet) CIDR() string {
	return s.cidr
}

func (s fakeSubnet) VLAN() gomaasapi.VLAN {
	return fakeVLAN{vid: s.vlanVid}
}

type fakeVLAN struct {
	gomaasapi.VLAN
	vid int
}

func (v fakeVLAN) VID() int {
	return v.vid
}
