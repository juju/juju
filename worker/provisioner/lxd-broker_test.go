// Copyright 2013-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"runtime"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/provisioner"
)

type lxdBrokerSuite struct {
	coretesting.BaseSuite
	broker        environs.InstanceBroker
	agentConfig   agent.ConfigSetterWriter
	api           *fakeAPI
	manager       *fakeContainerManager
	possibleTools coretools.List
}

var _ = gc.Suite(&lxdBrokerSuite{})

func (s *lxdBrokerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	if runtime.GOOS == "windows" {
		c.Skip("Skipping lxd tests on windows")
	}

	// To isolate the tests from the host's architecture, we override it here.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	s.possibleTools = coretools.List{&coretools.Tools{
		Version: version.MustParseBinary("2.3.4-quantal-amd64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-amd64.tgz",
	}, {
		// non-host-arch tools should be filtered out by StartInstance
		Version: version.MustParseBinary("2.3.4-quantal-arm64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-arm64.tgz",
	}}

	var err error
	s.agentConfig, err = agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             agent.NewPathsWithDefaults(agent.Paths{DataDir: "/not/used/here"}),
			Tag:               names.NewMachineTag("1"),
			UpgradedToVersion: jujuversion.Current,
			Password:          "dummy-secret",
			Nonce:             "nonce",
			APIAddresses:      []string{"10.0.0.1:1234"},
			CACert:            coretesting.CACert,
			Model:             coretesting.ModelTag,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.api = NewFakeAPI()
	s.manager = &fakeContainerManager{}
	s.broker, err = provisioner.NewLxdBroker(s.api, s.manager, s.agentConfig, true)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *lxdBrokerSuite) instanceConfig(c *gc.C, machineId string) *instancecfg.InstanceConfig {
	machineNonce := "fake-nonce"
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(machineId, machineNonce, "released", "quantal", true, apiInfo)
	c.Assert(err, jc.ErrorIsNil)
	return instanceConfig
}

func (s *lxdBrokerSuite) startInstance(c *gc.C, machineId string) instance.Instance {
	instanceConfig := s.instanceConfig(c, machineId)
	cons := constraints.Value{}
	result, err := s.broker.StartInstance(environs.StartInstanceParams{
		Constraints:    cons,
		Tools:          s.possibleTools,
		InstanceConfig: instanceConfig,
	})
	c.Assert(err, jc.ErrorIsNil)
	return result.Instance
}

func (s *lxdBrokerSuite) TestStartInstance(c *gc.C) {
	machineId := "1/lxd/0"
	s.startInstance(c, machineId)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "ContainerConfig",
	}, {
		FuncName: "PrepareContainerInterfaceInfo",
		Args:     []interface{}{names.NewMachineTag("1-lxd-0")},
	}})
	s.manager.CheckCallNames(c, "CreateContainer")
	call := s.manager.Calls()[0]
	c.Assert(call.Args[0], gc.FitsTypeOf, &instancecfg.InstanceConfig{})
	instanceConfig := call.Args[0].(*instancecfg.InstanceConfig)
	c.Assert(instanceConfig.ToolsList(), gc.HasLen, 1)
	c.Assert(instanceConfig.ToolsList().Arches(), jc.DeepEquals, []string{"amd64"})
}

func (s *lxdBrokerSuite) TestStartInstanceNoHostArchTools(c *gc.C) {
	_, err := s.broker.StartInstance(environs.StartInstanceParams{
		Tools: coretools.List{{
			// non-host-arch tools should be filtered out by StartInstance
			Version: version.MustParseBinary("2.3.4-quantal-arm64"),
			URL:     "http://tools.testing.invalid/2.3.4-quantal-arm64.tgz",
		}},
		InstanceConfig: s.instanceConfig(c, "1/lxd/0"),
	})
	c.Assert(err, gc.ErrorMatches, `need tools for arch amd64, only found \[arm64\]`)
}

type fakeContainerManager struct {
	gitjujutesting.Stub
}

func (m *fakeContainerManager) CreateContainer(instanceConfig *instancecfg.InstanceConfig,
	cons constraints.Value,
	series string,
	network *container.NetworkConfig,
	storage *container.StorageConfig,
	callback container.StatusCallback,
) (instance.Instance, *instance.HardwareCharacteristics, error) {
	m.MethodCall(m, "CreateContainer", instanceConfig, cons, series, network, storage, callback)
	return nil, nil, m.NextErr()
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
