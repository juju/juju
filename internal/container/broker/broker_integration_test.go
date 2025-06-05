// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/container/broker"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type brokerSuite struct {
	coretesting.BaseSuite
}

func TestBrokerSuite(t *testing.T) {
	tc.Run(t, &brokerSuite{})
}

func (s *brokerSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	broker.PatchNewMachineInitReader(s, newFakeMachineInitReader)
}

func (s *brokerSuite) TestCombinedCloudInitDataNoCloudInitUserData(c *tc.C) {
	obtained, err := broker.CombinedCloudInitData(nil, "ca-certs,apt-primary",
		corebase.MakeDefaultBase("ubuntu", "16.04"), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)

	assertCloudInitUserData(obtained, map[string]interface{}{
		"apt": map[string]interface{}{
			"primary": []interface{}{
				map[interface{}]interface{}{
					"arches": []interface{}{"default"},
					"uri":    "http://archive.ubuntu.com/ubuntu",
				},
			},
		},
		"ca-certs": map[interface{}]interface{}{
			"remove-defaults": true,
			"trusted":         []interface{}{"-----BEGIN CERTIFICATE-----\nYOUR-ORGS-TRUSTED-CA-CERT-HERE\n-----END CERTIFICATE-----\n"},
		},
	}, c)
}

func (s *brokerSuite) TestCombinedCloudInitDataNoContainerInheritProperties(c *tc.C) {
	containerConfig := fakeContainerConfig()
	obtained, err := broker.CombinedCloudInitData(containerConfig.CloudInitUserData, "",
		corebase.MakeDefaultBase("ubuntu", "16.04"), loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	assertCloudInitUserData(obtained, containerConfig.CloudInitUserData, c)
}

type fakeAddr struct{ value string }

func (f *fakeAddr) Network() string { return "net" }

func (f *fakeAddr) String() string {
	if f.value != "" {
		return f.value
	}
	return "fakeAddr"
}

var _ net.Addr = (*fakeAddr)(nil)

type fakeAPI struct {
	*testhelpers.Stub

	fakeContainerConfig params.ContainerConfig
	fakeInterfaceInfo   corenetwork.InterfaceInfo
	fakeDeviceToBridge  network.DeviceToBridge
	fakePreparer        broker.PrepareHostFunc
}

var _ broker.APICalls = (*fakeAPI)(nil)

var fakeInterfaceInfo = corenetwork.InterfaceInfo{
	DeviceIndex:   0,
	MACAddress:    "aa:bb:cc:dd:ee:ff",
	InterfaceName: "dummy0",
	Addresses: corenetwork.ProviderAddresses{
		corenetwork.NewMachineAddress("0.1.2.3", corenetwork.WithCIDR("0.1.2.0/24")).AsProviderAddress(),
	},
	GatewayAddress: corenetwork.NewMachineAddress("0.1.2.1").AsProviderAddress(),
	// Explicitly set only DNSServers, but not DNSSearchDomains to test this is
	// detected and the latter populated by parsing the fake resolv.conf created
	// by patchResolvConf(). See LP bug http://pad.lv/1575940 for more info.
	DNSServers:       []string{"ns1.dummy"},
	DNSSearchDomains: nil,
}

func fakeContainerConfig() params.ContainerConfig {
	return params.ContainerConfig{
		UpdateBehavior:          &params.UpdateBehavior{EnableOSRefreshUpdate: true, EnableOSUpgrade: true},
		ProviderType:            "fake",
		AuthorizedKeys:          coretesting.FakeAuthKeys,
		SSLHostnameVerification: true,
		CloudInitUserData: map[string]interface{}{
			"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
			"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
			"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
			"package_upgrade": false,
		},
	}
}

func NewFakeAPI() *fakeAPI {
	return &fakeAPI{
		Stub:                &testhelpers.Stub{},
		fakeContainerConfig: fakeContainerConfig(),
		fakeInterfaceInfo:   fakeInterfaceInfo,
	}
}

func (f *fakeAPI) ContainerConfig(ctx context.Context) (params.ContainerConfig, error) {
	f.MethodCall(f, "ContainerConfig")
	if err := f.NextErr(); err != nil {
		return params.ContainerConfig{}, err
	}
	return f.fakeContainerConfig, nil
}

func (f *fakeAPI) PrepareContainerInterfaceInfo(ctx context.Context, tag names.MachineTag) (corenetwork.InterfaceInfos, error) {
	f.MethodCall(f, "PrepareContainerInterfaceInfo", tag)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return corenetwork.InterfaceInfos{f.fakeInterfaceInfo}, nil
}

func (f *fakeAPI) GetContainerInterfaceInfo(ctx context.Context, tag names.MachineTag) (corenetwork.InterfaceInfos, error) {
	f.MethodCall(f, "GetContainerInterfaceInfo", tag)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return corenetwork.InterfaceInfos{f.fakeInterfaceInfo}, nil
}

func (f *fakeAPI) ReleaseContainerAddresses(ctx context.Context, tag names.MachineTag) error {
	f.MethodCall(f, "ReleaseContainerAddresses", tag)
	if err := f.NextErr(); err != nil {
		return err
	}
	return nil
}

func (f *fakeAPI) SetHostMachineNetworkConfig(ctx context.Context, hostMachineTag names.MachineTag, netConfig []params.NetworkConfig) error {
	f.MethodCall(f, "SetHostMachineNetworkConfig", hostMachineTag.String(), netConfig)
	if err := f.NextErr(); err != nil {
		return err
	}
	return nil
}

func (f *fakeAPI) HostChangesForContainer(ctx context.Context, machineTag names.MachineTag) ([]network.DeviceToBridge, error) {
	f.MethodCall(f, "HostChangesForContainer", machineTag)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return []network.DeviceToBridge{f.fakeDeviceToBridge}, nil
}

func (f *fakeAPI) PrepareHost(ctx context.Context, containerTag names.MachineTag, log corelogger.Logger, abort <-chan struct{}) error {
	// This is not actually part of the API, however it is something that the
	// Brokers should be calling, and putting it here means we get a wholistic
	// view of when what function is getting called.
	f.MethodCall(f, "PrepareHost", containerTag)
	if err := f.NextErr(); err != nil {
		return err
	}
	if f.fakePreparer != nil {
		return f.fakePreparer(ctx, containerTag, log, abort)
	}
	return nil
}

func (f *fakeAPI) GetContainerProfileInfo(ctx context.Context, containerTag names.MachineTag) ([]*apiprovisioner.LXDProfileResult, error) {
	f.MethodCall(f, "GetContainerProfileInfo", containerTag)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return []*apiprovisioner.LXDProfileResult{}, nil
}

type fakeContainerManager struct {
	testhelpers.Stub
}

func (m *fakeContainerManager) CreateContainer(_ context.Context,
	instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	base corebase.Base,
	network *container.NetworkConfig,
	storage *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (instances.Instance, *instance.HardwareCharacteristics, error) {
	m.MethodCall(m, "CreateContainer", instanceConfig, cons, base, network, storage, callback)
	inst := mockInstance{id: "testinst"}
	arch := "testarch"
	hw := instance.HardwareCharacteristics{Arch: &arch}
	return &inst, &hw, m.NextErr()
}

func (m *fakeContainerManager) DestroyContainer(id instance.Id) error {
	m.MethodCall(m, "DestroyContainer", id)
	return m.NextErr()
}

func (m *fakeContainerManager) ListContainers() ([]instances.Instance, error) {
	m.MethodCall(m, "ListContainers")
	return nil, m.NextErr()
}

func (m *fakeContainerManager) Namespace() instance.Namespace {
	ns, _ := instance.NewNamespace(coretesting.ModelTag.Id())
	return ns
}

func (m *fakeContainerManager) IsInitialized() bool {
	m.MethodCall(m, "IsInitialized")
	m.PopNoErr()
	return true
}

func (m *fakeContainerManager) MaybeWriteLXDProfile(pName string, put lxdprofile.Profile) error {
	m.MethodCall(m, "MaybeWriteLXDProfile")
	return m.NextErr()
}

type mockInstance struct {
	id string
}

var _ instances.Instance = (*mockInstance)(nil)

// Id implements instances.instance.Id.
func (m *mockInstance) Id() instance.Id {
	return instance.Id(m.id)
}

// Status implements instances.Instance.Status.
func (m *mockInstance) Status(context.Context) instance.Status {
	return instance.Status{}
}

// Addresses implements instances.Instance.Addresses.
func (m *mockInstance) Addresses(context.Context) (corenetwork.ProviderAddresses, error) {
	return nil, nil
}

type patcher interface {
	PatchValue(destination, source interface{})
}

func patchResolvConf(s patcher, c *tc.C) {
	const fakeConf = `
nameserver ns1.dummy
search dummy invalid
nameserver ns2.dummy
`

	fakeResolvConf := filepath.Join(c.MkDir(), "fakeresolv.conf")
	err := os.WriteFile(fakeResolvConf, []byte(fakeConf), 0644)
	c.Assert(err, tc.ErrorIsNil)
	s.PatchValue(broker.ResolvConfFiles, []string{fakeResolvConf})
}

func makeInstanceConfig(c *tc.C, s patcher, machineId string) *instancecfg.InstanceConfig {
	machineNonce := "fake-nonce"
	// To isolate the tests from the host's architecture, we override it here.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(coretesting.ControllerTag, machineId, machineNonce,
		"released", corebase.MakeDefaultBase("ubuntu", "22.04"), apiInfo)
	c.Assert(err, tc.ErrorIsNil)
	return instanceConfig
}

func makePossibleTools() coretools.List {
	return coretools.List{&coretools.Tools{
		Version: semversion.MustParseBinary("2.3.4-ubuntu-amd64"),
		URL:     "http://tools.testing.invalid/2.3.4-ubuntu-amd64.tgz",
	}, {
		// non-host-arch tools should be filtered out by StartInstance
		Version: semversion.MustParseBinary("2.3.4-ubuntu-arm64"),
		URL:     "http://tools.testing.invalid/2.3.4-ubuntu-arm64.tgz",
	}}
}

func makeNoOpStatusCallback() func(ctx context.Context, settableStatus status.Status, info string, data map[string]interface{}) error {
	return func(_ context.Context, _ status.Status, _ string, _ map[string]interface{}) error {
		return nil
	}
}

func callStartInstance(c *tc.C, s patcher, broker environs.InstanceBroker, machineId string) (*environs.StartInstanceResult, error) {
	return broker.StartInstance(c.Context(), environs.StartInstanceParams{
		Constraints:    constraints.Value{},
		Tools:          makePossibleTools(),
		InstanceConfig: makeInstanceConfig(c, s, machineId),
		StatusCallback: makeNoOpStatusCallback(),
	})
}

func assertCloudInitUserData(obtained, expected map[string]interface{}, c *tc.C) {
	c.Assert(obtained, tc.HasLen, len(expected))
	for obtainedK, obtainedV := range obtained {
		expectedV, ok := expected[obtainedK]
		c.Assert(ok, tc.IsTrue)
		switch obtainedK {
		case "package_upgrade":
			c.Assert(obtainedV, tc.Equals, expectedV)
		case "apt", "ca-certs":
			c.Assert(obtainedV, tc.DeepEquals, expectedV)
		default:
			c.Assert(obtainedV, tc.SameContents, expectedV)
		}
	}
}

type fakeMachineInitReader struct {
	cloudconfig.InitReader
}

func (r *fakeMachineInitReader) GetInitConfig() (map[string]interface{}, error) {
	return map[string]interface{}{
		"packages":   []interface{}{"python-novaclient"},
		"fake-entry": []interface{}{"testing-garbage"},
		"apt": map[interface{}]interface{}{
			"primary": []interface{}{
				map[interface{}]interface{}{
					"arches": []interface{}{"default"},
					"uri":    "http://archive.ubuntu.com/ubuntu",
				},
			},
			"security": []interface{}{
				map[interface{}]interface{}{
					"arches": []interface{}{"default"},
					"uri":    "http://archive.ubuntu.com/ubuntu",
				},
			},
		},
		"ca-certs": map[interface{}]interface{}{
			"remove-defaults": true,
			"trusted":         []interface{}{"-----BEGIN CERTIFICATE-----\nYOUR-ORGS-TRUSTED-CA-CERT-HERE\n-----END CERTIFICATE-----\n"},
		},
	}, nil
}

var newFakeMachineInitReader = func(base corebase.Base) (cloudconfig.InitReader, error) {
	r, err := cloudconfig.NewMachineInitReader(base)
	return &fakeMachineInitReader{r}, err
}
