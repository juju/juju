// Copyright 2013-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/container/broker"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
)

type blankMachineInitReader struct {
	cloudconfig.InitReader
}

func (r *blankMachineInitReader) GetInitConfig() (map[string]any, error) {
	return nil, nil
}

var newBlankMachineInitReader = func(base corebase.Base) (cloudconfig.InitReader, error) {
	r, err := cloudconfig.NewMachineInitReader(base)
	return &blankMachineInitReader{r}, err
}

type lxdBrokerSuite struct {
	coretesting.BaseSuite
	agentConfig agent.ConfigSetterWriter
	api         *fakeAPI
	manager     *fakeContainerManager
}

func TestLxdBrokerSuite(t *stdtesting.T) {
	tc.Run(t, &lxdBrokerSuite{})
}

func (s *lxdBrokerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	// To isolate the tests from the host's architecture, we override it here.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	broker.PatchNewMachineInitReader(s, newBlankMachineInitReader)

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
			Controller:        coretesting.ControllerTag,
			Model:             coretesting.ModelTag,
		})
	c.Assert(err, tc.ErrorIsNil)
	s.api = NewFakeAPI()
	s.manager = &fakeContainerManager{}
}

func (s *lxdBrokerSuite) startInstance(c *tc.C, broker environs.InstanceBroker, machineId string) (*environs.StartInstanceResult, error) {
	return callStartInstance(c, s, broker, machineId)
}

func (s *lxdBrokerSuite) newLXDBroker(c *tc.C) (environs.InstanceBroker, error) {
	return broker.NewLXDBroker(s.api.PrepareHost, s.api, s.manager, s.agentConfig)
}

func (s *lxdBrokerSuite) TestStartInstanceWithoutHostNetworkChanges(c *tc.C) {
	broker, brokerErr := s.newLXDBroker(c)
	c.Assert(brokerErr, tc.ErrorIsNil)
	machineId := "1/lxd/0"
	containerTag := names.NewMachineTag("1-lxd-0")
	s.startInstance(c, broker, machineId)
	s.api.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "ContainerConfig",
	}, {
		FuncName: "PrepareHost",
		Args:     []any{containerTag},
	}, {
		FuncName: "PrepareContainerInterfaceInfo",
		Args:     []any{names.NewMachineTag("1-lxd-0")},
	}})
	s.manager.CheckCallNames(c, "CreateContainer")
	call := s.manager.Calls()[0]
	c.Assert(call.Args[0], tc.FitsTypeOf, &instancecfg.InstanceConfig{})
	instanceConfig := call.Args[0].(*instancecfg.InstanceConfig)
	c.Assert(instanceConfig.ToolsList(), tc.HasLen, 1)
	arch, err := instanceConfig.ToolsList().OneArch()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(arch, tc.Equals, "amd64")
}

func (s *lxdBrokerSuite) TestStartInstancePopulatesFallbackNetworkInfo(c *tc.C) {
	broker, brokerErr := s.newLXDBroker(c)
	c.Assert(brokerErr, tc.ErrorIsNil)

	patchResolvConf(s, c)

	s.api.SetErrors(
		nil, // ContainerConfig succeeds
		nil, // HostChangesForContainer succeeds
		errors.NotSupportedf("container address allocation"),
	)
	_, err := s.startInstance(c, broker, "1/lxd/0")
	c.Assert(err, tc.ErrorMatches, "container address allocation not supported")
}

func (s *lxdBrokerSuite) TestStartInstanceNoHostArchTools(c *tc.C) {
	broker, brokerErr := s.newLXDBroker(c)
	c.Assert(brokerErr, tc.ErrorIsNil)

	_, err := broker.StartInstance(c.Context(), environs.StartInstanceParams{
		Tools: coretools.List{{
			// non-host-arch tools should be filtered out by StartInstance
			Version: semversion.MustParseBinary("2.3.4-ubuntu-arm64"),
			URL:     "http://tools.testing.invalid/2.3.4-ubuntu-arm64.tgz",
		}},
		InstanceConfig: makeInstanceConfig(c, s, "1/lxd/0"),
	})
	c.Assert(err, tc.ErrorMatches, `need agent binaries for arch amd64, only found arm64`)
}

func (s *lxdBrokerSuite) TestStartInstanceWithCloudInitUserData(c *tc.C) {
	broker, brokerErr := s.newLXDBroker(c)
	c.Assert(brokerErr, tc.ErrorIsNil)

	_, err := s.startInstance(c, broker, "1/lxd/0")
	c.Assert(err, tc.ErrorIsNil)

	s.manager.CheckCallNames(c, "CreateContainer")
	call := s.manager.Calls()[0]
	c.Assert(call.Args[0], tc.FitsTypeOf, &instancecfg.InstanceConfig{})
	instanceConfig := call.Args[0].(*instancecfg.InstanceConfig)
	assertCloudInitUserData(instanceConfig.CloudInitUserData, map[string]any{
		"packages":        []any{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []any{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []any{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false,
	}, c)
}

func (s *lxdBrokerSuite) TestStartInstanceWithContainerInheritProperties(c *tc.C) {
	broker.PatchNewMachineInitReader(s, newFakeMachineInitReader)
	s.api.fakeContainerConfig.ContainerInheritProperties = "ca-certs,apt-security"

	broker, brokerErr := s.newLXDBroker(c)
	c.Assert(brokerErr, tc.ErrorIsNil)
	_, err := s.startInstance(c, broker, "1/lxd/0")
	c.Assert(err, tc.ErrorIsNil)

	s.manager.CheckCallNames(c, "CreateContainer")
	call := s.manager.Calls()[0]
	c.Assert(call.Args[0], tc.FitsTypeOf, &instancecfg.InstanceConfig{})
	instanceConfig := call.Args[0].(*instancecfg.InstanceConfig)
	assertCloudInitUserData(instanceConfig.CloudInitUserData, map[string]any{
		"packages":        []any{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []any{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []any{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false,
		"apt": map[string]any{
			"security": []any{
				map[any]any{
					"arches": []any{"default"},
					"uri":    "http://archive.ubuntu.com/ubuntu",
				},
			},
		},
		"ca-certs": map[any]any{
			"remove-defaults": true,
			"trusted": []any{
				"-----BEGIN CERTIFICATE-----\nYOUR-ORGS-TRUSTED-CA-CERT-HERE\n-----END CERTIFICATE-----\n"},
		},
	}, c)
}
