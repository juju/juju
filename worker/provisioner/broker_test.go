// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"io/ioutil"
	"net"
	"path/filepath"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	instancetest "github.com/juju/juju/instance/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/worker/provisioner"
)

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

var fakeContainerConfig = params.ContainerConfig{
	UpdateBehavior:          &params.UpdateBehavior{true, true},
	ProviderType:            "fake",
	AuthorizedKeys:          coretesting.FakeAuthKeys,
	SSLHostnameVerification: true,
}

func NewFakeAPI() *fakeAPI {
	return &fakeAPI{
		Stub:                &gitjujutesting.Stub{},
		fakeContainerConfig: fakeContainerConfig,
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

func (f *fakeAPI) PrepareContainerInterfaceInfo(tag names.MachineTag) ([]network.InterfaceInfo, []network.DeviceToBridge, error) {
	f.MethodCall(f, "PrepareContainerInterfaceInfo", tag)
	if err := f.NextErr(); err != nil {
		return nil, nil, err
	}
	return []network.InterfaceInfo{f.fakeInterfaceInfo}, nil, nil
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
	s.PatchValue(provisioner.ResolvConf, fakeResolvConf)
}

func instancesFromResults(results ...*environs.StartInstanceResult) []instance.Instance {
	instances := make([]instance.Instance, len(results))
	for i := range results {
		instances[i] = results[i].Instance
	}
	return instances
}

func assertInstancesStarted(c *gc.C, broker environs.InstanceBroker, results ...*environs.StartInstanceResult) {
	allInstances, err := broker.AllInstances()
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

func callStartInstance(c *gc.C, s patcher, broker environs.InstanceBroker, machineId string) *environs.StartInstanceResult {
	result, err := broker.StartInstance(environs.StartInstanceParams{
		Constraints:    constraints.Value{},
		Tools:          makePossibleTools(),
		InstanceConfig: makeInstanceConfig(c, s, machineId),
		StatusCallback: makeNoOpStatusCallback(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return result
}

func callMaintainInstance(c *gc.C, s patcher, broker environs.InstanceBroker, machineId string) {
	err := broker.MaintainInstance(environs.StartInstanceParams{
		Constraints:    constraints.Value{},
		Tools:          makePossibleTools(),
		InstanceConfig: makeInstanceConfig(c, s, machineId),
		StatusCallback: makeNoOpStatusCallback(),
	})
	c.Assert(err, jc.ErrorIsNil)
}
