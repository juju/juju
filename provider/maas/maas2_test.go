// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type maas2Suite struct {
	baseProviderSuite
}

func (suite *maas2Suite) injectController(controller gomaasapi.Controller) {
	mockGetController := func(maasServer, apiKey string) (gomaasapi.Controller, error) {
		return controller, nil
	}
	suite.PatchValue(&GetMAAS2Controller, mockGetController)
}

func (suite *maas2Suite) makeEnviron(c *gc.C, controller gomaasapi.Controller) *maasEnviron {
	if controller != nil {
		suite.injectController(controller)
	}
	testAttrs := coretesting.Attrs{}
	for k, v := range maasEnvAttrs {
		testAttrs[k] = v
	}
	testAttrs["agent-version"] = version.Current.String()

	cred := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{
		"maas-oauth": "a:b:c",
	})
	cloud := environs.CloudSpec{
		Type:       "maas",
		Name:       "maas",
		Endpoint:   "http://any-old-junk.invalid/",
		Credential: &cred,
	}

	attrs := coretesting.FakeConfig().Merge(testAttrs)
	suite.controllerUUID = coretesting.FakeControllerConfig().ControllerUUID()
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	env, err := NewEnviron(cloud, cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
	return env
}

type fakeController struct {
	gomaasapi.Controller
	*testing.Stub

	bootResources      []gomaasapi.BootResource
	bootResourcesError error
	machines           []gomaasapi.Machine
	machinesError      error
	machinesArgsCheck  func(gomaasapi.MachinesArgs)
	zones              []gomaasapi.Zone
	zonesError         error
	spaces             []gomaasapi.Space
	spacesError        error

	allocateMachine          gomaasapi.Machine
	allocateMachineMatches   gomaasapi.ConstraintMatches
	allocateMachineError     error
	allocateMachineArgsCheck func(gomaasapi.AllocateMachineArgs)

	files []gomaasapi.File

	devices []gomaasapi.Device
}

func newFakeController() *fakeController {
	return &fakeController{Stub: &testing.Stub{}}
}

func newFakeControllerWithErrors(errors ...error) *fakeController {
	controller := newFakeController()
	controller.SetErrors(errors...)
	return controller
}

func newFakeControllerWithFiles(files ...gomaasapi.File) *fakeController {
	return &fakeController{Stub: &testing.Stub{}, files: files}
}

func (c *fakeController) Devices(args gomaasapi.DevicesArgs) ([]gomaasapi.Device, error) {
	c.MethodCall(c, "Devices", args)
	return c.devices, c.NextErr()
}

func (c *fakeController) Machines(args gomaasapi.MachinesArgs) ([]gomaasapi.Machine, error) {
	if c.machinesArgsCheck != nil {
		c.machinesArgsCheck(args)
	}
	if c.machinesError != nil {
		return nil, c.machinesError
	}
	if len(args.SystemIDs) > 0 {
		result := []gomaasapi.Machine{}
		systemIds := set.NewStrings(args.SystemIDs...)
		for _, machine := range c.machines {
			if systemIds.Contains(machine.SystemID()) {
				result = append(result, machine)
			}
		}
		return result, nil
	}
	return c.machines, nil
}

func (c *fakeController) AllocateMachine(args gomaasapi.AllocateMachineArgs) (gomaasapi.Machine, gomaasapi.ConstraintMatches, error) {
	if c.allocateMachineArgsCheck != nil {
		c.allocateMachineArgsCheck(args)
	}
	if c.allocateMachineError != nil {
		return nil, c.allocateMachineMatches, c.allocateMachineError
	}
	return c.allocateMachine, c.allocateMachineMatches, nil
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

func (c *fakeController) Files(prefix string) ([]gomaasapi.File, error) {
	c.MethodCall(c, "Files", prefix)
	return c.files, c.NextErr()
}

func (c *fakeController) GetFile(filename string) (gomaasapi.File, error) {
	c.MethodCall(c, "GetFile", filename)
	err := c.NextErr()
	if err != nil {
		return nil, err
	}
	// Try to find the file by name (needed for testing RemoveAll)
	for _, file := range c.files {
		if file.Filename() == filename {
			return file, nil
		}
	}
	// The test forgot to set up matching files!
	return nil, errors.Errorf("no file named %v found - did you set up your test correctly?", filename)
}

func (c *fakeController) AddFile(args gomaasapi.AddFileArgs) error {
	c.MethodCall(c, "AddFile", args)
	return c.NextErr()
}

func (c *fakeController) ReleaseMachines(args gomaasapi.ReleaseMachinesArgs) error {
	c.MethodCall(c, "ReleaseMachines", args)
	return c.NextErr()
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
	*testing.Stub

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
	tags          []string
	createDevice  gomaasapi.Device
}

func newFakeMachine(systemID, architecture, statusName string) *fakeMachine {
	return &fakeMachine{
		Stub:         &testing.Stub{},
		systemID:     systemID,
		architecture: architecture,
		statusName:   statusName,
	}
}

func (m *fakeMachine) Tags() []string {
	return m.tags
}

func (m *fakeMachine) SetOwnerData(data map[string]string) error {
	m.MethodCall(m, "SetOwnerData", data)
	return m.NextErr()
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
	m.MethodCall(m, "Start", args)
	return m.NextErr()
}

func (m *fakeMachine) CreateDevice(args gomaasapi.CreateMachineDeviceArgs) (gomaasapi.Device, error) {
	m.MethodCall(m, "CreateDevice", args)
	return m.createDevice, m.NextErr()
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
	*testing.Stub

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

func (v *fakeInterface) EffectiveMTU() int {
	return 1500
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

func (v *fakeInterface) LinkSubnet(args gomaasapi.LinkSubnetArgs) error {
	v.MethodCall(v, "LinkSubnet", args)
	return v.NextErr()
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

type fakeFile struct {
	gomaasapi.File
	name     string
	url      string
	contents []byte
	deleted  bool
	error    error
}

func (f *fakeFile) Filename() string {
	return f.name
}

func (f *fakeFile) AnonymousURL() string {
	return f.url
}

func (f *fakeFile) Delete() error {
	f.deleted = true
	return f.error
}

func (f *fakeFile) ReadAll() ([]byte, error) {
	if f.error != nil {
		return nil, f.error
	}
	return f.contents, nil
}

type fakeBlockDevice struct {
	gomaasapi.BlockDevice
	name string
	path string
	size uint64
}

func (bd fakeBlockDevice) Name() string {
	return bd.name
}

func (bd fakeBlockDevice) Path() string {
	return bd.path
}

func (bd fakeBlockDevice) Size() uint64 {
	return bd.size
}

type fakeDevice struct {
	gomaasapi.Device
	*testing.Stub

	interfaceSet []gomaasapi.Interface
	systemID     string
	interface_   gomaasapi.Interface
}

func (d *fakeDevice) InterfaceSet() []gomaasapi.Interface {
	return d.interfaceSet
}

func (d *fakeDevice) SystemID() string {
	return d.systemID
}

func (d *fakeDevice) CreateInterface(args gomaasapi.CreateInterfaceArgs) (gomaasapi.Interface, error) {
	d.MethodCall(d, "CreateInterface", args)
	d.interfaceSet = append(d.interfaceSet, d.interface_)
	return d.interface_, d.NextErr()
}

func (d *fakeDevice) Delete() error {
	d.MethodCall(d, "Delete")
	return d.NextErr()
}
