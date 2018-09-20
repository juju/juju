// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"io/ioutil"
	"net"
	"path/filepath"

	"github.com/juju/loggo"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	instancetest "github.com/juju/juju/instance/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/worker/provisioner"
)

type brokerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&brokerSuite{})

func (s *brokerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.PatchValue(&provisioner.GetMachineCloudInitData, func(_ string) (map[string]interface{}, error) {
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
	})
}

func (s *brokerSuite) TestCombinedCloudInitDataNoCloudInitUserData(c *gc.C) {
	obtained, err := provisioner.CombinedCloudInitData(nil, "ca-certs,apt-primary", "xenial", loggo.Logger{})
	c.Assert(err, jc.ErrorIsNil)

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

func (s *brokerSuite) TestCombinedCloudInitDataNoContainerInheritProperties(c *gc.C) {
	containerConfig := fakeContainerConfig()
	obtained, err := provisioner.CombinedCloudInitData(containerConfig.CloudInitUserData, "", "xenial", loggo.Logger{})
	c.Assert(err, jc.ErrorIsNil)
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
	*gitjujutesting.Stub

	fakeContainerConfig params.ContainerConfig
	fakeInterfaceInfo   network.InterfaceInfo
	fakeDeviceToBridge  network.DeviceToBridge
	fakeBridger         network.Bridger
	fakePreparer        provisioner.PrepareHostFunc
}

var _ provisioner.APICalls = (*fakeAPI)(nil)

var fakeInterfaceInfo network.InterfaceInfo = network.InterfaceInfo{
	DeviceIndex:    0,
	MACAddress:     "aa:bb:cc:dd:ee:ff",
	CIDR:           "0.1.2.0/24",
	InterfaceName:  "dummy0",
	Address:        network.NewAddress("0.1.2.3"),
	GatewayAddress: network.NewAddress("0.1.2.1"),
	// Explicitly set only DNSServers, but not DNSSearchDomains to test this is
	// detected and the latter populated by parsing the fake resolv.conf created
	// by patchResolvConf(). See LP bug http://pad.lv/1575940 for more info.
	DNSServers:       network.NewAddresses("ns1.dummy"),
	DNSSearchDomains: nil,
}

var fakeDeviceToBridge network.DeviceToBridge = network.DeviceToBridge{
	DeviceName: "dummy0",
	BridgeName: "br-dummy0",
}

func fakeContainerConfig() params.ContainerConfig {
	return params.ContainerConfig{
		UpdateBehavior:          &params.UpdateBehavior{true, true},
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
		Stub:                &gitjujutesting.Stub{},
		fakeContainerConfig: fakeContainerConfig(),
		fakeInterfaceInfo:   fakeInterfaceInfo,
	}
}

func (f *fakeAPI) ContainerConfig() (params.ContainerConfig, error) {
	f.MethodCall(f, "ContainerConfig")
	if err := f.NextErr(); err != nil {
		return params.ContainerConfig{}, err
	}
	return f.fakeContainerConfig, nil
}

func (f *fakeAPI) PrepareContainerInterfaceInfo(tag names.MachineTag) ([]network.InterfaceInfo, error) {
	f.MethodCall(f, "PrepareContainerInterfaceInfo", tag)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return []network.InterfaceInfo{f.fakeInterfaceInfo}, nil
}

func (f *fakeAPI) GetContainerInterfaceInfo(tag names.MachineTag) ([]network.InterfaceInfo, error) {
	f.MethodCall(f, "GetContainerInterfaceInfo", tag)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return []network.InterfaceInfo{f.fakeInterfaceInfo}, nil
}

func (f *fakeAPI) ReleaseContainerAddresses(tag names.MachineTag) error {
	f.MethodCall(f, "ReleaseContainerAddresses", tag)
	if err := f.NextErr(); err != nil {
		return err
	}
	return nil
}

func (f *fakeAPI) SetHostMachineNetworkConfig(hostMachineTag names.MachineTag, netConfig []params.NetworkConfig) error {
	f.MethodCall(f, "SetHostMachineNetworkConfig", hostMachineTag.String(), netConfig)
	if err := f.NextErr(); err != nil {
		return err
	}
	return nil
}

func (f *fakeAPI) HostChangesForContainer(machineTag names.MachineTag) ([]network.DeviceToBridge, int, error) {
	f.MethodCall(f, "HostChangesForContainer", machineTag)
	if err := f.NextErr(); err != nil {
		return nil, 0, err
	}
	return []network.DeviceToBridge{f.fakeDeviceToBridge}, 0, nil
}

func (f *fakeAPI) PrepareHost(containerTag names.MachineTag, log loggo.Logger, abort <-chan struct{}) error {
	// This is not actually part of the API, however it is something that the
	// Brokers should be calling, and putting it here means we get a wholistic
	// view of when what function is getting called.
	f.MethodCall(f, "PrepareHost", containerTag)
	if err := f.NextErr(); err != nil {
		return err
	}
	if f.fakePreparer != nil {
		return f.fakePreparer(containerTag, log, abort)
	}
	return nil
}

func (f *fakeAPI) GetContainerProfileInfo(containerTag names.MachineTag) ([]apiprovisioner.LXDProfileResult, error) {
	f.MethodCall(f, "GetContainerProfileInfo", containerTag)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return []apiprovisioner.LXDProfileResult{}, nil
}

type fakeContainerManager struct {
	gitjujutesting.Stub
}

func (m *fakeContainerManager) CreateContainer(instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	network *container.NetworkConfig,
	storage *container.StorageConfig,
	callback environs.StatusCallbackFunc,
) (instance.Instance, *instance.HardwareCharacteristics, error) {
	m.MethodCall(m, "CreateContainer", instanceConfig, cons, series, network, storage, callback)
	inst := mockInstance{id: "testinst"}
	arch := "testarch"
	hw := instance.HardwareCharacteristics{Arch: &arch}
	return &inst, &hw, m.NextErr()
}

func (m *fakeContainerManager) DestroyContainer(id instance.Id) error {
	m.MethodCall(m, "DestroyContainer", id)
	return m.NextErr()
}

func (m *fakeContainerManager) ListContainers() ([]instance.Instance, error) {
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

func (m *fakeContainerManager) MaybeWriteLXDProfile(pName string, put *charm.LXDProfile) error {
	m.MethodCall(m, "MaybeWriteLXDProfile")
	return m.NextErr()
}

type mockInstance struct {
	id string
}

var _ instance.Instance = (*mockInstance)(nil)

// Id implements instance.Instance.Id.
func (m *mockInstance) Id() instance.Id {
	return instance.Id(m.id)
}

// Status implements instance.Instance.Status.
func (m *mockInstance) Status(context.ProviderCallContext) instance.InstanceStatus {
	return instance.InstanceStatus{}
}

// Addresses implements instance.Instance.Addresses.
func (m *mockInstance) Addresses(context.ProviderCallContext) ([]network.Address, error) {
	return nil, nil
}

type patcher interface {
	PatchValue(destination, source interface{})
}

func patchResolvConf(s patcher, c *gc.C) {
	const fakeConf = `
nameserver ns1.dummy
search dummy invalid
nameserver ns2.dummy
`

	fakeResolvConf := filepath.Join(c.MkDir(), "fakeresolv.conf")
	err := ioutil.WriteFile(fakeResolvConf, []byte(fakeConf), 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(provisioner.ResolvConfFiles, []string{fakeResolvConf})
}

func instancesFromResults(results ...*environs.StartInstanceResult) []instance.Instance {
	instances := make([]instance.Instance, len(results))
	for i := range results {
		instances[i] = results[i].Instance
	}
	return instances
}

func assertInstancesStarted(c *gc.C, broker environs.InstanceBroker, results ...*environs.StartInstanceResult) {
	allInstances, err := broker.AllInstances(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
	instancetest.MatchInstances(c, allInstances, instancesFromResults(results...)...)
}

func makeInstanceConfig(c *gc.C, s patcher, machineId string) *instancecfg.InstanceConfig {
	machineNonce := "fake-nonce"
	// To isolate the tests from the host's architecture, we override it here.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(coretesting.ControllerTag, machineId, machineNonce, "released", "quantal", apiInfo)
	c.Assert(err, jc.ErrorIsNil)
	return instanceConfig
}

func makePossibleTools() coretools.List {
	return coretools.List{&coretools.Tools{
		Version: version.MustParseBinary("2.3.4-quantal-amd64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-amd64.tgz",
	}, {
		// non-host-arch tools should be filtered out by StartInstance
		Version: version.MustParseBinary("2.3.4-quantal-arm64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-arm64.tgz",
	}}
}

func makeNoOpStatusCallback() func(settableStatus status.Status, info string, data map[string]interface{}) error {
	return func(_ status.Status, _ string, _ map[string]interface{}) error {
		return nil
	}
}

func callStartInstance(c *gc.C, s patcher, broker environs.InstanceBroker, machineId string) (*environs.StartInstanceResult, error) {
	return broker.StartInstance(context.NewCloudCallContext(), environs.StartInstanceParams{
		Constraints:    constraints.Value{},
		Tools:          makePossibleTools(),
		InstanceConfig: makeInstanceConfig(c, s, machineId),
		StatusCallback: makeNoOpStatusCallback(),
	})
}

func callMaintainInstance(c *gc.C, s patcher, broker environs.InstanceBroker, machineId string) {
	err := broker.MaintainInstance(context.NewCloudCallContext(), environs.StartInstanceParams{
		Constraints:    constraints.Value{},
		Tools:          makePossibleTools(),
		InstanceConfig: makeInstanceConfig(c, s, machineId),
		StatusCallback: makeNoOpStatusCallback(),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func assertCloudInitUserData(obtained, expected map[string]interface{}, c *gc.C) {
	c.Assert(obtained, gc.HasLen, len(expected))
	for obtainedK, obtainedV := range obtained {
		expectedV, ok := expected[obtainedK]
		c.Assert(ok, jc.IsTrue)
		switch obtainedK {
		case "package_upgrade":
			c.Assert(obtainedV, gc.Equals, expectedV)
		case "apt", "ca-certs":
			c.Assert(obtainedV, jc.DeepEquals, expectedV)
		default:
			c.Assert(obtainedV, jc.SameContents, expectedV)
		}
	}
}
