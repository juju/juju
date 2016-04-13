// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type maas2Suite struct {
	baseProviderSuite
}

func (suite *maas2Suite) SetUpTest(c *gc.C) {
	suite.baseProviderSuite.SetUpTest(c)
	suite.SetFeatureFlags(feature.MAAS2)
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
	testAttrs["maas-server"] = "http://any-old-junk.invalid/"
	testAttrs["agent-version"] = version.Current.String()
	attrs := coretesting.FakeConfig().Merge(testAttrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	env, err := NewEnviron(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
	return env
}

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
	files                    []gomaasapi.File
	filesPrefix              string
	filesError               error
	getFileFilename          string
	addFileArgs              gomaasapi.AddFileArgs
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

func (c *fakeController) Files(prefix string) ([]gomaasapi.File, error) {
	c.filesPrefix = prefix
	if c.filesError != nil {
		return nil, c.filesError
	}
	return c.files, nil
}

func (c *fakeController) GetFile(filename string) (gomaasapi.File, error) {
	c.getFileFilename = filename
	if c.filesError != nil {
		return nil, c.filesError
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
	c.addFileArgs = args
	return c.filesError
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
