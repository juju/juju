// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/gomaasapi"
)

type fakeController struct {
	gomaasapi.Controller
	bootResources            []gomaasapi.BootResource
	bootResourcesError       error
	machines                 []gomaasapi.Machine
	machinesError            error
	machinesArgsCheck        func(gomaasapi.MachinesArgs)
	zones                    []gomaasapi.Zone
	zonesError               error
	spaces                   []gomaasapi.Space
	spacesError              error
	allocateMachine          gomaasapi.Machine
	allocateMachineError     error
	allocateMachineArgsCheck func(gomaasapi.AllocateMachineArgs)
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

func (c *fakeController) AllocateMachine(args gomaasapi.AllocateMachineArgs) (gomaasapi.Machine, gomaasapi.ConstraintMatches, error) {
	matches := gomaasapi.ConstraintMatches{}
	if c.allocateMachineArgsCheck != nil {
		c.allocateMachineArgsCheck(args)
	}
	if c.allocateMachineError != nil {
		return nil, matches, c.allocateMachineError
	}
	return c.allocateMachine, matches, nil
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
	interfaceSet  []gomaasapi.Interface
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

func (m *fakeMachine) InterfaceSet() []gomaasapi.Interface {
	return m.interfaceSet
}

func (m *fakeMachine) Start(args gomaasapi.StartArgs) error {
	return nil
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
	id         int
	space      string
	vlan       gomaasapi.VLAN
	gateway    string
	cidr       string
	dnsServers []string
}

func (s fakeSubnet) ID() int {
	return s.id
}

func (s fakeSubnet) Space() string {
	return s.space
}

func (s fakeSubnet) VLAN() gomaasapi.VLAN {
	return s.vlan
}

func (s fakeSubnet) Gateway() string {
	return s.gateway
}

func (s fakeSubnet) CIDR() string {
	return s.cidr
}

func (s fakeSubnet) DNSServers() []string {
	return s.dnsServers
}

type fakeVLAN struct {
	gomaasapi.VLAN
	id  int
	vid int
	mtu int
}

func (v fakeVLAN) ID() int {
	return v.id
}

func (v fakeVLAN) VID() int {
	return v.vid
}

func (v fakeVLAN) MTU() int {
	return v.mtu
}

type fakeInterface struct {
	gomaasapi.Interface
	id         int
	name       string
	parents    []string
	children   []string
	type_      string
	enabled    bool
	vlan       gomaasapi.VLAN
	links      []gomaasapi.Link
	macAddress string
}

func (v *fakeInterface) ID() int {
	return v.id
}

func (v *fakeInterface) Name() string {
	return v.name
}

func (v *fakeInterface) Parents() []string {
	return v.parents
}

func (v *fakeInterface) Children() []string {
	return v.children
}

func (v *fakeInterface) Type() string {
	return v.type_
}

func (v *fakeInterface) Enabled() bool {
	return v.enabled
}

func (v *fakeInterface) VLAN() gomaasapi.VLAN {
	return v.vlan
}

func (v *fakeInterface) Links() []gomaasapi.Link {
	return v.links
}

func (v *fakeInterface) MACAddress() string {
	return v.macAddress
}

type fakeLink struct {
	gomaasapi.Link
	id        int
	mode      string
	subnet    gomaasapi.Subnet
	ipAddress string
}

func (l *fakeLink) ID() int {
	return l.id
}

func (l *fakeLink) Mode() string {
	return l.mode
}

func (l *fakeLink) Subnet() gomaasapi.Subnet {
	return l.subnet
}

func (l *fakeLink) IPAddress() string {
	return l.ipAddress
}
