// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/providerinit"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/gce/google"
	"github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
)

// Ensure GCE provider supports the expected interfaces.
var (
	_ config.ConfigSchemaSource = (*environProvider)(nil)
)

// These values are fake GCE auth credentials for use in tests.
const (
	ClientName  = "ba9876543210-0123456789abcdefghijklmnopqrstuv"
	ClientID    = ClientName + ".apps.googleusercontent.com"
	ClientEmail = ClientName + "@developer.gserviceaccount.com"
	ProjectID   = "my-juju"
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
		"type":            "gce",
		"uuid":            "2d02eeac-9dbb-11e4-89d3-123b93f75cba",
		"controller-uuid": "bfef02f1-932a-425a-a102-62175dcabd1d",
	})
)

func MakeTestCloudSpec() environscloudspec.CloudSpec {
	cred := MakeTestCredential()
	return environscloudspec.CloudSpec{
		Type:       "gce",
		Name:       "google",
		Region:     "us-east1",
		Endpoint:   "https://www.googleapis.com",
		Credential: &cred,
	}
}

func MakeTestCredential() cloud.Credential {
	return cloud.NewCredential(
		cloud.OAuth2AuthType,
		map[string]string{
			"project-id":   ProjectID,
			"client-id":    ClientID,
			"client-email": ClientEmail,
			"private-key":  PrivateKey,
		},
	)
}

type credentialInvalidator func(ctx context.Context, reason environs.CredentialInvalidReason) error

func (c credentialInvalidator) InvalidateCredentials(ctx context.Context, reason environs.CredentialInvalidReason) error {
	return c(ctx, reason)
}

type BaseSuiteUnpatched struct {
	jujutesting.IsolationSuite

	ControllerUUID string
	Config         *config.Config
	EnvConfig      *environConfig
	Env            *environ

	Addresses      network.ProviderAddresses
	BaseInstance   *google.Instance
	BaseDisk       *google.Disk
	Instance       *environInstance
	InstName       string
	UbuntuMetadata map[string]string
	StartInstArgs  environs.StartInstanceParams
	InstanceType   instances.InstanceType

	Rules firewall.IngressRules

	credentialInvalidator  credentialInvalidator
	InvalidatedCredentials bool
}

var _ environs.Environ = (*environ)(nil)

var _ simplestreams.HasRegion = (*environ)(nil)

var _ instances.Instance = (*environInstance)(nil)

func (s *BaseSuiteUnpatched) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.ControllerUUID = testing.FakeControllerConfig().ControllerUUID()
	s.initEnv(c)
	s.initInst(c)
	s.initNet(c)

	s.credentialInvalidator = func(ctx context.Context, reason environs.CredentialInvalidReason) error {
		s.InvalidatedCredentials = true
		return nil
	}
}

func (s *BaseSuiteUnpatched) TearDownTest(c *tc.C) {
	s.IsolationSuite.TearDownTest(c)
	s.InvalidatedCredentials = false
}

func (s *BaseSuiteUnpatched) Prefix() string {
	return s.Env.namespace.Prefix()
}

func (s *BaseSuiteUnpatched) initEnv(c *tc.C) {
	s.Env = &environ{
		CredentialInvalidator: common.NewCredentialInvalidator(s.credentialInvalidator, google.IsAuthorisationFailure),
		name:                  "google",
		cloud:                 MakeTestCloudSpec(),
	}
	cfg := s.NewConfig(c, nil)
	s.setConfig(c, cfg)
}

func (s *BaseSuiteUnpatched) initInst(c *tc.C) {
	tools := []*coretools.Tools{{
		Version: semversion.Binary{Arch: arch.AMD64, Release: "ubuntu"},
		URL:     "https://example.org",
	}}

	var instType = "n1-standard-1"
	cons := constraints.Value{InstanceType: &instType}

	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(testing.FakeControllerConfig(), cons, cons,
		jujuversion.DefaultSupportedLTSBase(), "", nil)
	c.Assert(err, tc.ErrorIsNil)

	err = instanceConfig.SetTools(tools)
	c.Assert(err, tc.ErrorIsNil)
	instanceConfig.AuthorizedKeys = s.Config.AuthorizedKeys()

	userData, err := providerinit.ComposeUserData(instanceConfig, nil, GCERenderer{})
	c.Assert(err, tc.ErrorIsNil)

	s.UbuntuMetadata = map[string]string{
		tags.JujuIsController: "true",
		tags.JujuController:   s.ControllerUUID,
		metadataKeyCloudInit:  string(userData),
		metadataKeyEncoding:   "base64",
	}
	instanceConfig.Tags = map[string]string{
		tags.JujuIsController: "true",
		tags.JujuController:   s.ControllerUUID,
	}
	s.Addresses = []network.ProviderAddress{
		network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
	}
	s.Instance = s.NewInstance(c, "spam")
	s.BaseInstance = s.Instance.base
	s.InstName, err = s.Env.namespace.Hostname("42")
	c.Assert(err, tc.ErrorIsNil)

	s.StartInstArgs = environs.StartInstanceParams{
		ControllerUUID: s.ControllerUUID,
		InstanceConfig: instanceConfig,
		Tools:          tools,
		Constraints:    cons,
	}

	s.InstanceType = instances.InstanceType{
		Name:     instType,
		Arch:     arch.AMD64,
		CpuCores: 1,
		CpuPower: instances.CpuPower(275),
		Mem:      3750,
		VirtType: &virtType,
	}

	// Storage
	eUUID := s.Env.Config().UUID()
	s.BaseDisk = &google.Disk{
		Id:               1234567,
		Name:             "home-zone--c930380d-8337-4bf5-b07a-9dbb5ae771e4",
		Zone:             "home-zone",
		Status:           google.StatusReady,
		Size:             1024,
		Description:      eUUID,
		LabelFingerprint: "foo",
		Labels: map[string]string{
			"yodel":                "eh",
			"juju-model-uuid":      eUUID,
			"juju-controller-uuid": s.ControllerUUID,
		},
	}
}

func (s *BaseSuiteUnpatched) initNet(c *tc.C) {
	s.Rules = firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp")),
	}
}

func (s *BaseSuiteUnpatched) setConfig(c *tc.C, cfg *config.Config) {
	s.Config = cfg
	ecfg, err := newConfig(context.Background(), cfg, nil)
	c.Assert(err, tc.ErrorIsNil)
	s.EnvConfig = ecfg
	uuid := cfg.UUID()
	s.Env.uuid = uuid
	s.Env.ecfg = s.EnvConfig
	namespace, err := instance.NewNamespace(uuid)
	c.Assert(err, tc.ErrorIsNil)
	s.Env.namespace = namespace
}

func (s *BaseSuiteUnpatched) NewConfig(c *tc.C, updates testing.Attrs) *config.Config {
	var err error
	cfg := testing.ModelConfig(c)
	cfg, err = cfg.Apply(ConfigAttrs)
	c.Assert(err, tc.ErrorIsNil)
	cfg, err = cfg.Apply(updates)
	c.Assert(err, tc.ErrorIsNil)
	return cfg
}

func (s *BaseSuiteUnpatched) UpdateConfig(c *tc.C, attrs map[string]interface{}) {
	cfg, err := s.Config.Apply(attrs)
	c.Assert(err, tc.ErrorIsNil)
	s.setConfig(c, cfg)
}

func (s *BaseSuiteUnpatched) NewBaseInstance(c *tc.C, id string) *google.Instance {
	diskSpec := google.DiskSpec{
		OS:         "ubuntu",
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
		Metadata:          s.UbuntuMetadata,
		Tags:              []string{id},
	}
	summary := google.InstanceSummary{
		ID:        id,
		ZoneName:  "home-zone",
		Status:    google.StatusRunning,
		Metadata:  s.UbuntuMetadata,
		Addresses: s.Addresses,
		NetworkInterfaces: []*compute.NetworkInterface{{
			Name:       "somenetif",
			NetworkIP:  "10.0.10.3",
			Network:    "https://www.googleapis.com/compute/v1/projects/sonic-youth/global/networks/go-team",
			Subnetwork: "https://www.googleapis.com/compute/v1/projects/sonic-youth/regions/asia-east1/subnetworks/go-team",
		}},
	}
	return google.NewInstance(summary, &instanceSpec)
}

func (s *BaseSuiteUnpatched) NewInstance(c *tc.C, id string) *environInstance {
	base := s.NewBaseInstance(c, id)
	return newInstance(base, s.Env)
}

func (s *BaseSuiteUnpatched) NewInstanceFromBase(base *google.Instance) *environInstance {
	return newInstance(base, s.Env)
}

type BaseSuite struct {
	BaseSuiteUnpatched

	FakeConn    *fakeConn
	FakeCommon  *fakeCommon
	FakeEnviron *fakeEnviron
}

func (s *BaseSuite) SetUpTest(c *tc.C) {
	s.BaseSuiteUnpatched.SetUpTest(c)

	s.FakeConn = &fakeConn{}
	s.FakeCommon = &fakeCommon{}
	s.FakeEnviron = &fakeEnviron{}

	// Patch out all expensive external deps.
	s.Env.gce = s.FakeConn
	s.PatchValue(&newConnection, func(context.Context, google.ConnectionConfig, *google.Credentials) (gceConnection, error) {
		return s.FakeConn, nil
	})
	s.PatchValue(&bootstrap, s.FakeCommon.Bootstrap)
	s.PatchValue(&destroyEnv, s.FakeCommon.Destroy)
	s.PatchValue(&buildInstanceSpec, s.FakeEnviron.BuildInstanceSpec)
	s.PatchValue(&getHardwareCharacteristics, s.FakeEnviron.GetHardwareCharacteristics)
	s.PatchValue(&newRawInstance, s.FakeEnviron.NewRawInstance)
	s.PatchValue(&findInstanceSpec, s.FakeEnviron.FindInstanceSpec)
	s.PatchValue(&getInstances, s.FakeEnviron.GetInstances)
}

func (s *BaseSuite) CheckNoAPI(c *tc.C) {
	c.Check(s.FakeConn.Calls, tc.HasLen, 0)
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

func (f *fake) CheckCalls(c *tc.C, expected []FakeCall) {
	c.Check(f.calls, tc.DeepEquals, expected)
}

type fakeCommon struct {
	fake

	Arch        string
	Base        corebase.Base
	BSFinalizer environs.CloudBootstrapFinalizer
	AZInstances []common.AvailabilityZoneInstances
}

func (fc *fakeCommon) Bootstrap(ctx environs.BootstrapContext, env environs.Environ, params environs.BootstrapParams) (*environs.BootstrapResult, error) {
	fc.addCall("Bootstrap", FakeCallArgs{
		"ctx":    ctx,
		"switch": env,
		"params": params,
	})

	result := &environs.BootstrapResult{
		Arch:                    fc.Arch,
		Base:                    fc.Base,
		CloudBootstrapFinalizer: fc.BSFinalizer,
	}
	return result, fc.err()
}

func (fc *fakeCommon) Destroy(env environs.Environ, ctx context.Context) error {
	fc.addCall("Destroy", FakeCallArgs{
		"switch": env,
	})
	return fc.err()
}

type fakeEnviron struct {
	fake

	Inst  *google.Instance
	Insts []instances.Instance
	Hwc   *instance.HardwareCharacteristics
	Spec  *instances.InstanceSpec
}

func (fe *fakeEnviron) GetInstances(env *environ, ctx context.Context, statusFilters ...string) ([]instances.Instance, error) {
	fe.addCall("GetInstances", FakeCallArgs{
		"switch": env,
	})
	return fe.Insts, fe.err()
}

func (fe *fakeEnviron) BuildInstanceSpec(env *environ, ctx context.Context, args environs.StartInstanceParams) (*instances.InstanceSpec, error) {
	fe.addCall("BuildInstanceSpec", FakeCallArgs{
		"switch": env,
		"args":   args,
	})
	return fe.Spec, fe.err()
}

func (fe *fakeEnviron) GetHardwareCharacteristics(env *environ, spec *instances.InstanceSpec, inst *environInstance) *instance.HardwareCharacteristics {
	fe.addCall("GetHardwareCharacteristics", FakeCallArgs{
		"switch": env,
		"spec":   spec,
		"inst":   inst,
	})
	return fe.Hwc
}

func (fe *fakeEnviron) NewRawInstance(env *environ, ctx context.Context, args environs.StartInstanceParams, spec *instances.InstanceSpec) (*google.Instance, error) {
	fe.addCall("NewRawInstance", FakeCallArgs{
		"switch": env,
		"args":   args,
		"spec":   spec,
	})
	return fe.Inst, fe.err()
}

func (fe *fakeEnviron) FindInstanceSpec(
	env *environ,
	ic *instances.InstanceConstraint,
	imageMetadata []*imagemetadata.ImageMetadata,
	instanceTypes []instances.InstanceType,
) (*instances.InstanceSpec, error) {
	fe.addCall("FindInstanceSpec", FakeCallArgs{
		"switch":        env,
		"ic":            ic,
		"imageMetadata": imageMetadata,
		"instanceTypes": instanceTypes,
	})
	return fe.Spec, fe.err()
}

// TODO(ericsnow) Refactor fakeConnCall and fakeConn to embed fakeCall and fake.

type fakeConnCall struct {
	FuncName string

	ID               string
	IDs              []string
	ZoneName         string
	Prefix           string
	Statuses         []string
	InstanceSpec     google.InstanceSpec
	FirewallName     string
	Rules            firewall.IngressRules
	Region           string
	Disks            []google.DiskSpec
	VolumeName       string
	InstanceId       string
	Mode             string
	Key              string
	Value            string
	LabelFingerprint string
	Labels           map[string]string
}

type fakeConn struct {
	Calls []fakeConnCall

	Inst      *google.Instance
	Insts     []google.Instance
	Rules     firewall.IngressRules
	Zones     []google.AvailabilityZone
	Subnets   []*compute.Subnetwork
	Networks_ []*compute.Network

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

func (fc *fakeConn) AddInstance(spec google.InstanceSpec) (*google.Instance, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "AddInstance",
		InstanceSpec: spec,
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

func (fc *fakeConn) UpdateMetadata(key, value string, ids ...string) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "UpdateMetadata",
		Key:      key,
		Value:    value,
		IDs:      ids,
	})
	return fc.err()
}

func (fc *fakeConn) IngressRules(fwname string) (firewall.IngressRules, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "Ports",
		FirewallName: fwname,
	})
	return fc.Rules, fc.err()
}

func (fc *fakeConn) OpenPorts(fwname string, rules firewall.IngressRules) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "OpenPorts",
		FirewallName: fwname,
		Rules:        rules,
	})
	return fc.err()
}

func (fc *fakeConn) ClosePorts(fwname string, rules firewall.IngressRules) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "ClosePorts",
		FirewallName: fwname,
		Rules:        rules,
	})
	return fc.err()
}

func (fc *fakeConn) RemoveFirewall(fwname string) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:     "RemoveFirewall",
		FirewallName: fwname,
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

func (fc *fakeConn) Subnetworks(region string) ([]*compute.Subnetwork, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "Subnetworks",
		Region:   region,
	})
	return fc.Subnets, fc.err()
}

func (fc *fakeConn) Networks() ([]*compute.Network, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "Networks",
	})
	return fc.Networks_, fc.err()
}

func (fc *fakeConn) CreateDisks(zone string, disks []google.DiskSpec) ([]*google.Disk, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "CreateDisks",
		ZoneName: zone,
		Disks:    disks,
	})
	return fc.GoogleDisks, fc.err()
}

func (fc *fakeConn) Disks() ([]*google.Disk, error) {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName: "Disks",
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

func (fc *fakeConn) SetDiskLabels(zone, id, labelFingerprint string, labels map[string]string) error {
	fc.Calls = append(fc.Calls, fakeConnCall{
		FuncName:         "SetDiskLabels",
		ZoneName:         zone,
		ID:               id,
		LabelFingerprint: labelFingerprint,
		Labels:           labels,
	})
	return fc.err()
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

func (fc *fakeConn) ListMachineTypes(zone string) ([]google.MachineType, error) {
	call := fakeConnCall{
		FuncName: "ListMachineTypes",
		ZoneName: zone,
	}
	fc.Calls = append(fc.Calls, call)

	return []google.MachineType{
		{Name: "n1-standard-1", MemoryMb: 1024, GuestCpus: 1},
		{Name: "n1-standard-2", MemoryMb: 2048, GuestCpus: 2},
	}, nil
}

var InvalidCredentialError = &url.Error{"Get", "testbad.com", errors.New("400 Bad Request")}
