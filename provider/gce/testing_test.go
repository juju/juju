// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"encoding/base64"
	"fmt"
	"strings"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// These values are fake GCE auth credentials for use in tests.
const (
	ClientName  = "ba9876543210-0123456789abcdefghijklmnopqrstuv"
	ClientID    = ClientName + ".apps.googleusercontent.com"
	ClientEmail = ClientName + "@developer.gserviceaccount.com"
	PrivateKey  = `-----BEGIN PRIVATE KEY-----
...
...
...
...
...
...
...
...
...
...
...
...
...
...
-----END PRIVATE KEY-----
`
)

// These are fake config values for use in tests.
var (
	AuthFile = fmt.Sprintf(`{
  "private_key_id": "abcdef0123456789abcdef0123456789abcdef01",
  "private_key": "%s",
  "client_email": "%s",
  "client_id": "%s",
  "type": "service_account"
}`, strings.Replace(PrivateKey, "\n", "\\n", -1), ClientEmail, ClientID)

	ConfigAttrs = testing.FakeConfig().Merge(testing.Attrs{
		"type":           "gce",
		"auth-file":      "",
		"private-key":    PrivateKey,
		"client-id":      ClientID,
		"client-email":   ClientEmail,
		"region":         "home",
		"project-id":     "my-juju",
		"image-endpoint": "https://www.googleapis.com",
		"uuid":           "2d02eeac-9dbb-11e4-89d3-123b93f75cba",
	})
)

type BaseSuiteUnpatched struct {
	gitjujutesting.IsolationSuite

	Config    *config.Config
	EnvConfig *environConfig
	Env       *environ
	Prefix    string

	Addresses     []network.Address
	BaseInstance  *google.Instance
	BaseDisk      *google.Disk
	Instance      *environInstance
	InstName      string
	Metadata      map[string]string
	StartInstArgs environs.StartInstanceParams
	InstanceType  instances.InstanceType

	Ports []network.PortRange
}

var _ environs.Environ = (*environ)(nil)
var _ simplestreams.HasRegion = (*environ)(nil)
var _ instance.Instance = (*environInstance)(nil)

func (s *BaseSuiteUnpatched) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.initEnv(c)
	s.initInst(c)
	s.initNet(c)
}

func (s *BaseSuiteUnpatched) initEnv(c *gc.C) {
	s.Env = &environ{
		name: "google",
	}
	cfg := s.NewConfig(c, nil)
	s.setConfig(c, cfg)
}

func (s *BaseSuiteUnpatched) initInst(c *gc.C) {
	tools := []*tools.Tools{{
		Version: version.Binary{Arch: arch.AMD64, Series: "trusty"},
		URL:     "https://example.org",
	}}

	cons := constraints.Value{InstanceType: &allInstanceTypes[0].Name}

	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(cons, "trusty")
	c.Assert(err, jc.ErrorIsNil)

	instanceConfig.Tools = tools[0]
	instanceConfig.AuthorizedKeys = s.Config.AuthorizedKeys()

	userData, err := providerinit.ComposeUserData(instanceConfig, nil)
	c.Assert(err, jc.ErrorIsNil)
	b64UserData := base64.StdEncoding.EncodeToString([]byte(userData))

	authKeys, err := google.FormatAuthorizedKeys(instanceConfig.AuthorizedKeys, "ubuntu")
	c.Assert(err, jc.ErrorIsNil)

	s.Metadata = map[string]string{
		metadataKeyIsState:   metadataValueTrue,
		metadataKeyCloudInit: b64UserData,
		metadataKeyEncoding:  "base64",
		metadataKeySSHKeys:   authKeys,
	}
	s.Addresses = []network.Address{{
		Value: "10.0.0.1",
		Type:  network.IPv4Address,
		Scope: network.ScopeCloudLocal,
	}}
	s.Instance = s.NewInstance(c, "spam")
	s.BaseInstance = s.Instance.base
	s.InstName = s.Prefix + "machine-spam"

	s.StartInstArgs = environs.StartInstanceParams{
		InstanceConfig: instanceConfig,
		Tools:          tools,
		Constraints:    cons,
		//Placement: "",
		//DistributionGroup: nil,
	}

	s.InstanceType = allInstanceTypes[0]
	// Storage
	s.BaseDisk = &google.Disk{
		Id:     1234567,
		Name:   "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
		Zone:   "home-zone",
		Status: google.StatusReady,
		Size:   1024,
	}
}

func (s *BaseSuiteUnpatched) initNet(c *gc.C) {
	s.Ports = []network.PortRange{{
		FromPort: 80,
		ToPort:   80,
		Protocol: "tcp",
	}}
}

func (s *BaseSuiteUnpatched) setConfig(c *gc.C, cfg *config.Config) {
	s.Config = cfg
	ecfg, err := newValidConfig(cfg, configDefaults)
	c.Assert(err, jc.ErrorIsNil)
	s.EnvConfig = ecfg
	uuid, _ := cfg.UUID()
	s.Env.uuid = uuid
	s.Env.ecfg = s.EnvConfig
	s.Prefix = "juju-" + uuid + "-"
}

func (s *BaseSuiteUnpatched) NewConfig(c *gc.C, updates testing.Attrs) *config.Config {
	var err error
	cfg := testing.EnvironConfig(c)
	cfg, err = cfg.Apply(ConfigAttrs)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = cfg.Apply(updates)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func (s *BaseSuiteUnpatched) UpdateConfig(c *gc.C, attrs map[string]interface{}) {
	cfg, err := s.Config.Apply(attrs)
	c.Assert(err, jc.ErrorIsNil)
	s.setConfig(c, cfg)
}

func (s *BaseSuiteUnpatched) NewBaseInstance(c *gc.C, id string) *google.Instance {
	diskSpec := google.DiskSpec{
		SizeHintGB: 15,
		ImageURL:   "some/image/path",
		Boot:       true,
		Scratch:    false,
		Readonly:   false,
		AutoDelete: true,
	}
	instanceSpec := google.InstanceSpec{
		ID:                id,
		Type:              "mtype",
		Disks:             []google.DiskSpec{diskSpec},
		Network:           google.NetworkSpec{Name: "somenetwork"},
		NetworkInterfaces: []string{"somenetif"},
		Metadata:          s.Metadata,
		Tags:              []string{id},
	}
	summary := google.InstanceSummary{
		ID:        id,
		ZoneName:  "home-zone",
		Status:    google.StatusRunning,
		Metadata:  s.Metadata,
		Addresses: s.Addresses,
	}
	return google.NewInstance(summary, &instanceSpec)
}

func (s *BaseSuiteUnpatched) NewInstance(c *gc.C, id string) *environInstance {
	base := s.NewBaseInstance(c, id)
	return newInstance(base, s.Env)
}

type BaseSuite struct {
	BaseSuiteUnpatched

	FakeConn    *fakeConn
	FakeCommon  *fakeCommon
	FakeEnviron *fakeEnviron
	FakeImages  *fakeImages
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuiteUnpatched.SetUpTest(c)

	s.FakeConn = &fakeConn{}
	s.FakeCommon = &fakeCommon{}
	s.FakeEnviron = &fakeEnviron{}
	s.FakeImages = &fakeImages{}

	// Patch out all expensive external deps.
	s.Env.gce = s.FakeConn
	s.PatchValue(&newConnection, func(*environConfig) (gceConnection, error) {
		return s.FakeConn, nil
	})
	s.PatchValue(&supportedArchitectures, s.FakeCommon.SupportedArchitectures)
	s.PatchValue(&bootstrap, s.FakeCommon.Bootstrap)
	s.PatchValue(&destroyEnv, s.FakeCommon.Destroy)
	s.PatchValue(&availabilityZoneAllocations, s.FakeCommon.AvailabilityZoneAllocations)
	s.PatchValue(&buildInstanceSpec, s.FakeEnviron.BuildInstanceSpec)
	s.PatchValue(&getHardwareCharacteristics, s.FakeEnviron.GetHardwareCharacteristics)
	s.PatchValue(&newRawInstance, s.FakeEnviron.NewRawInstance)
	s.PatchValue(&findInstanceSpec, s.FakeEnviron.FindInstanceSpec)
	s.PatchValue(&getInstances, s.FakeEnviron.GetInstances)
	s.PatchValue(&imageMetadataFetch, s.FakeImages.ImageMetadataFetch)
}

func (s *BaseSuite) CheckNoAPI(c *gc.C) {
	c.Check(s.FakeConn.Calls, gc.HasLen, 0)
}

// TODO(ericsnow) Move fakeCallArgs, fakeCall, and fake to the testing repo?

type FakeCallArgs map[string]interface{}

type FakeCall struct {
	FuncName string
	Args     FakeCallArgs
}

type fake struct {
	calls []FakeCall

	Err        error
	FailOnCall int
}

func (f *fake) err() error {
	if len(f.calls) != f.FailOnCall+1 {
		return nil
	}
	return f.Err
}

func (f *fake) addCall(funcName string, args FakeCallArgs) {
	f.calls = append(f.calls, FakeCall{
		FuncName: funcName,
		Args:     args,
	})
}

func (f *fake) CheckCalls(c *gc.C, expected []FakeCall) {
	c.Check(f.calls, jc.DeepEquals, expected)
}

type fakeCommon struct {
	fake

	Arches      []string
	Arch        string
	Series      string
	BSFinalizer environs.BootstrapFinalizer
	AZInstances []common.AvailabilityZoneInstances
}

func (fc *fakeCommon) SupportedArchitectures(env environs.Environ, cons *imagemetadata.ImageConstraint) ([]string, error) {
	fc.addCall("SupportedArchitectures", FakeCallArgs{
		"env":  env,
		"cons": cons,
	})
	return fc.Arches, fc.err()
}

func (fc *fakeCommon) Bootstrap(ctx environs.BootstrapContext, env environs.Environ, params environs.BootstrapParams) (string, string, environs.BootstrapFinalizer, error) {
	fc.addCall("Bootstrap", FakeCallArgs{
		"ctx":    ctx,
		"env":    env,
		"params": params,
	})
	return fc.Arch, fc.Series, fc.BSFinalizer, fc.err()
}

func (fc *fakeCommon) Destroy(env environs.Environ) error {
	fc.addCall("Destroy", FakeCallArgs{
		"env": env,
	})
	return fc.err()
}

func (fc *fakeCommon) AvailabilityZoneAllocations(env common.ZonedEnviron, group []instance.Id) ([]common.AvailabilityZoneInstances, error) {
	fc.addCall("AvailabilityZoneAllocations", FakeCallArgs{
		"env":   env,
		"group": group,
	})
	return fc.AZInstances, fc.err()
}

type fakeEnviron struct {
	fake

	Inst  *google.Instance
	Insts []instance.Instance
	Hwc   *instance.HardwareCharacteristics
	Spec  *instances.InstanceSpec
}

func (fe *fakeEnviron) GetInstances(env *environ) ([]instance.Instance, error) {
	fe.addCall("GetInstances", FakeCallArgs{
		"env": env,
	})
	return fe.Insts, fe.err()
}

func (fe *fakeEnviron) BuildInstanceSpec(env *environ, args environs.StartInstanceParams) (*instances.InstanceSpec, error) {
	fe.addCall("BuildInstanceSpec", FakeCallArgs{
		"env":  env,
		"args": args,
	})
	return fe.Spec, fe.err()
}

func (fe *fakeEnviron) GetHardwareCharacteristics(env *environ, spec *instances.InstanceSpec, inst *environInstance) *instance.HardwareCharacteristics {
	fe.addCall("GetHardwareCharacteristics", FakeCallArgs{
		"env":  env,
		"spec": spec,
		"inst": inst,
	})
	return fe.Hwc
}

func (fe *fakeEnviron) NewRawInstance(env *environ, args environs.StartInstanceParams, spec *instances.InstanceSpec) (*google.Instance, error) {
	fe.addCall("NewRawInstance", FakeCallArgs{
		"env":  env,
		"args": args,
		"spec": spec,
	})
	return fe.Inst, fe.err()
}

func (fe *fakeEnviron) FindInstanceSpec(env *environ, stream string, ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	fe.addCall("FindInstanceSpec", FakeCallArgs{
		"env":    env,
		"stream": stream,
		"ic":     ic,
	})
	return fe.Spec, fe.err()
}

type fakeImages struct {
	fake

	Metadata    []*imagemetadata.ImageMetadata
	ResolveInfo *simplestreams.ResolveInfo
}

func (fi *fakeImages) ImageMetadataFetch(sources []simplestreams.DataSource, cons *imagemetadata.ImageConstraint, onlySigned bool) ([]*imagemetadata.ImageMetadata, *simplestreams.ResolveInfo, error) {
	return fi.Metadata, fi.ResolveInfo, fi.err()
}

// TODO(ericsnow) Refactor fakeConnCall and fakeConn to embed fakeCall and fake.

type fakeConnCall struct {
	FuncName string

	ID           string
	IDs          []string
	ZoneName     string
	ZoneNames    []string
	Prefix       string
	Statuses     []string
	InstanceSpec google.InstanceSpec
	FirewallName string
	PortRanges   []network.PortRange
	Region       string
	Disks        []google.DiskSpec
	VolumeName   string
	InstanceId   string
	Mode         string
}

type fakeConn struct {
	Calls []fakeConnCall

	Inst       *google.Instance
	Insts      []google.Instance
	PortRanges []network.PortRange
	Zones      []google.AvailabilityZone

	GoogleDisks   []*google.Disk
	GoogleDisk    *google.Disk
	AttachedDisk  *google.AttachedDisk
	AttachedDisks []*google.AttachedDisk

	Err        error
	FailOnCall int
}

func (fc *fakeConn) err() error {
	if len(fc.Calls) != fc.FailOnCall+1 {
		return nil
	}
	return fc.Err
}

func (fc *fakeConn) VerifyCredentials() error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "",
	})
	return fc.err()
}

func (fc *fakeConn) Instance(id, zone string) (google.Instance, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "Instance",
		ID:       id,
		ZoneName: zone,
	})
	return *fc.Inst, fc.err()
}

func (fc *fakeConn) Instances(prefix string, statuses ...string) ([]google.Instance, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "Instances",
		Prefix:   prefix,
		Statuses: statuses,
	})
	return fc.Insts, fc.err()
}

func (fc *fakeConn) AddInstance(spec google.InstanceSpec, zones ...string) (*google.Instance, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "AddInstance",
		InstanceSpec: spec,
		ZoneNames:    zones,
	})
	return fc.Inst, fc.err()
}

func (fc *fakeConn) RemoveInstances(prefix string, ids ...string) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "RemoveInstances",
		Prefix:   prefix,
		IDs:      ids,
	})
	return fc.err()
}

func (fc *fakeConn) Ports(fwname string) ([]network.PortRange, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "Ports",
		FirewallName: fwname,
	})
	return fc.PortRanges, fc.err()
}

func (fc *fakeConn) OpenPorts(fwname string, ports ...network.PortRange) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "OpenPorts",
		FirewallName: fwname,
		PortRanges:   ports,
	})
	return fc.err()
}

func (fc *fakeConn) ClosePorts(fwname string, ports ...network.PortRange) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "ClosePorts",
		FirewallName: fwname,
		PortRanges:   ports,
	})
	return fc.err()
}

func (fc *fakeConn) AvailabilityZones(region string) ([]google.AvailabilityZone, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "AvailabilityZones",
		Region:   region,
	})
	return fc.Zones, fc.err()
}

func (fc *fakeConn) CreateDisks(zone string, disks []google.DiskSpec) ([]*google.Disk, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "CreateDisks",
		ZoneName: zone,
		Disks:    disks,
	})
	return fc.GoogleDisks, fc.err()
}

func (fc *fakeConn) Disks(zone string) ([]*google.Disk, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "Disks",
		ZoneName: zone,
	})
	return fc.GoogleDisks, fc.err()
}

func (fc *fakeConn) RemoveDisk(zone, id string) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "RemoveDisk",
		ZoneName: zone,
		ID:       id,
	})
	return fc.err()
}

func (fc *fakeConn) Disk(zone, id string) (*google.Disk, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "Disk",
		ZoneName: zone,
		ID:       id,
	})
	return fc.GoogleDisk, fc.err()
}

func (fc *fakeConn) AttachDisk(zone, volumeName, instanceId string, mode google.DiskMode) (*google.AttachedDisk, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:   "AttachDisk",
		ZoneName:   zone,
		VolumeName: volumeName,
		InstanceId: instanceId,
		Mode:       string(mode),
	})
	return fc.AttachedDisk, fc.err()
}

func (fc *fakeConn) DetachDisk(zone, instanceId, volumeName string) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:   "DetachDisk",
		ZoneName:   zone,
		InstanceId: instanceId,
		VolumeName: volumeName,
	})
	return fc.err()
}

func (fc *fakeConn) InstanceDisks(zone, instanceId string) ([]*google.AttachedDisk, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:   "InstanceDisks",
		ZoneName:   zone,
		InstanceId: instanceId,
	})
	return fc.AttachedDisks, fc.err()
}

func (fc *fakeConn) WasCalled(funcName string) (bool, []fakeConnCall) {
	var calls []fakeConnCall
	called := false
	for _, call := range fc.Calls {
		if call.FuncName == funcName {
			called = true
			calls = append(calls, call)
		}
	}
	return called, calls
}
