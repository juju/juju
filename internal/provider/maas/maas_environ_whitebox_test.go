// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net/http"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/tags"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

const maasVersionResponse = `{"version": "unknown", "subversion": "", "capabilities": ["networks-management", "static-ipaddresses", "ipv6-deployment-ubuntu", "devices-management", "storage-deployment-ubuntu", "network-deployment-ubuntu"]}`

const maasDomainsResponse = `
[
    {
        "authoritative": "true",
        "resource_uri": "/MAAS/api/2.0/domains/0/",
        "name": "maas",
        "id": 0,
        "ttl": null,
        "resource_record_count": 3
    }
]
`

type maasEnvironSuite struct {
	maasSuite
}

var _ = gc.Suite(&maasEnvironSuite{})

func (suite *maasEnvironSuite) getEnvWithServer(c *gc.C) (*maasEnviron, error) {
	testServer := gomaasapi.NewSimpleServer()
	testServer.AddGetResponse("/api/2.0/version/", http.StatusOK, maasVersionResponse)
	testServer.AddGetResponse("/api/2.0/users/?op=whoami", http.StatusOK, "{}")
	testServer.AddGetResponse("/api/2.0/domains", http.StatusOK, maasDomainsResponse)
	// Weirdly, rather than returning a 404 when the version is
	// unknown, MAAS2 returns some HTML (the login page).
	testServer.AddGetResponse("/api/1.0/version/", http.StatusOK, "<html></html>")
	testServer.Start()
	suite.AddCleanup(func(*gc.C) { testServer.Close() })
	cred := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{
		"maas-oauth": "a:b:c",
	})
	cloud := environscloudspec.CloudSpec{
		Type:       "maas",
		Name:       "maas",
		Endpoint:   testServer.Server.URL,
		Credential: &cred,
	}
	attrs := coretesting.FakeConfig().Merge(maasEnvAttrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return NewEnviron(context.Background(), cloud, cfg, suite.credentialInvalidator, nil)
}

func (suite *maasEnvironSuite) TestNewEnvironWithController(c *gc.C) {
	env, err := suite.getEnvWithServer(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
}

func (suite *maasEnvironSuite) TestNewEnvironWithControllerSkipTLSVerify(c *gc.C) {
	env, err := suite.getEnvWithServer(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
}

func (suite *maasEnvironSuite) injectControllerWithSpacesAndCheck(c *gc.C, spaces []gomaasapi.Space, expected gomaasapi.AllocateMachineArgs) (*maasEnviron, *fakeController) {
	machine := newFakeMachine("Bruce Sterling", arch.HostArch(), "")
	return suite.injectControllerWithMachine(c, machine, spaces, expected)
}

func (suite *maasEnvironSuite) injectControllerWithMachine(c *gc.C, machine *fakeMachine, spaces []gomaasapi.Space, expected gomaasapi.AllocateMachineArgs) (*maasEnviron, *fakeController) {
	var env *maasEnviron
	check := func(args gomaasapi.AllocateMachineArgs) {
		expected.AgentName = env.Config().UUID()
		c.Assert(args, gc.DeepEquals, expected)
	}

	controller := &fakeController{
		allocateMachineArgsCheck: check,
		allocateMachine:          machine,
		allocateMachineMatches: gomaasapi.ConstraintMatches{
			Storage: map[string][]gomaasapi.StorageDevice{},
		},
		spaces: spaces,
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)
	return env, controller
}

func (suite *maasEnvironSuite) makeEnvironWithMachines(c *gc.C, expectedSystemIDs []string, returnSystemIDs []string) *maasEnviron {
	var env *maasEnviron
	checkArgs := func(args gomaasapi.MachinesArgs) {
		c.Check(args.SystemIDs, gc.DeepEquals, expectedSystemIDs)
		c.Check(args.AgentName, gc.Equals, env.Config().UUID())
	}
	machines := make([]gomaasapi.Machine, len(returnSystemIDs))
	for index, id := range returnSystemIDs {
		machines[index] = &fakeMachine{systemID: id}
	}
	controller := &fakeController{
		machines:          machines,
		machinesArgsCheck: checkArgs,
	}
	env = suite.makeEnviron(c, controller)
	return env
}

func (suite *maasEnvironSuite) TestAllRunningInstances(c *gc.C) {
	env := suite.makeEnvironWithMachines(
		c, []string{}, []string{"tuco", "tio", "gus"},
	)
	result, err := env.AllRunningInstances(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	expectedMachines := set.NewStrings("tuco", "tio", "gus")
	actualMachines := set.NewStrings()
	for _, instance := range result {
		actualMachines.Add(string(instance.Id()))
	}
	c.Assert(actualMachines, gc.DeepEquals, expectedMachines)
}

func (suite *maasEnvironSuite) TestAllRunningInstancesError(c *gc.C) {
	controller := &fakeController{machinesError: errors.New("Something terrible!")}
	env := suite.makeEnviron(c, controller)
	_, err := env.AllRunningInstances(suite.callCtx)
	c.Assert(err, gc.ErrorMatches, "Something terrible!")
}

func (suite *maasEnvironSuite) TestInstances(c *gc.C) {
	env := suite.makeEnvironWithMachines(
		c, []string{"jake", "bonnibel"}, []string{"jake", "bonnibel"},
	)
	result, err := env.Instances(suite.callCtx, []instance.Id{"jake", "bonnibel"})
	c.Assert(err, jc.ErrorIsNil)
	expectedMachines := set.NewStrings("jake", "bonnibel")
	actualMachines := set.NewStrings()
	for _, machine := range result {
		actualMachines.Add(string(machine.Id()))
	}
	c.Assert(actualMachines, gc.DeepEquals, expectedMachines)
}

func (suite *maasEnvironSuite) TestInstancesInvalidCredential(c *gc.C) {
	controller := &fakeController{
		machinesError: gomaasapi.NewPermissionError("fail auth here"),
	}
	env := suite.makeEnviron(c, controller)
	c.Assert(suite.invalidCredential, jc.IsFalse)
	_, err := env.Instances(suite.callCtx, []instance.Id{"jake", "bonnibel"})
	c.Assert(err, gc.NotNil)
	c.Assert(suite.invalidCredential, jc.IsTrue)
}

func (suite *maasEnvironSuite) TestInstancesPartialResult(c *gc.C) {
	env := suite.makeEnvironWithMachines(
		c, []string{"jake", "bonnibel"}, []string{"tuco", "bonnibel"},
	)
	result, err := env.Instances(suite.callCtx, []instance.Id{"jake", "bonnibel"})
	c.Check(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(result, gc.HasLen, 2)
	c.Assert(result[0], gc.IsNil)
	c.Assert(result[1].Id(), gc.Equals, instance.Id("bonnibel"))
}

func (suite *maasEnvironSuite) TestAvailabilityZones(c *gc.C) {
	env := suite.makeEnviron(c, newFakeController())
	result, err := env.AvailabilityZones(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	expectedZones := set.NewStrings("mossack", "fonseca")
	actualZones := set.NewStrings()
	for _, zone := range result {
		actualZones.Add(zone.Name())
	}
	c.Assert(actualZones, gc.DeepEquals, expectedZones)
}

func (suite *maasEnvironSuite) TestAvailabilityZonesError(c *gc.C) {
	controller := &fakeController{
		zonesError: errors.New("a bad thing"),
	}
	env := suite.makeEnviron(c, controller)
	_, err := env.AvailabilityZones(suite.callCtx)
	c.Assert(err, gc.ErrorMatches, "a bad thing")
}

func (suite *maasEnvironSuite) TestAvailabilityZonesInvalidCredential(c *gc.C) {
	controller := &fakeController{
		zonesError: gomaasapi.NewPermissionError("fail auth here"),
	}
	env := suite.makeEnviron(c, controller)
	c.Assert(suite.invalidCredential, jc.IsFalse)
	_, err := env.AvailabilityZones(suite.callCtx)
	c.Assert(err, gc.NotNil)
	c.Assert(suite.invalidCredential, jc.IsTrue)
}

func (suite *maasEnvironSuite) TestSpaces(c *gc.C) {
	controller := newFakeController()
	controller.spaces = []gomaasapi.Space{
		fakeSpace{
			name: "pepper",
			id:   1234,
		},
		fakeSpace{
			name: "freckles",
			id:   4567,
			subnets: []gomaasapi.Subnet{
				fakeSubnet{id: 99, vlan: fakeVLAN{vid: 66}, cidr: "192.168.10.0/24"},
				fakeSubnet{id: 98, vlan: fakeVLAN{vid: 67}, cidr: "192.168.11.0/24"},
			},
		},
	}

	env := suite.makeEnviron(c, controller)
	result, err := env.Spaces(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Check(result[0].Name, gc.Equals, network.SpaceName("freckles"))
	c.Check(result[0].ProviderId, gc.Equals, network.Id("4567"))

	subnets := result[0].Subnets
	c.Assert(subnets, gc.HasLen, 2)

	s0 := subnets[0]
	c.Check(s0.ProviderId, gc.Equals, network.Id("99"))
	c.Check(s0.VLANTag, gc.Equals, 66)
	c.Check(s0.CIDR, gc.Equals, "192.168.10.0/24")
	c.Check(s0.ProviderSpaceId, gc.Equals, network.Id("4567"))
	c.Check(s0.AvailabilityZones, jc.SameContents, []string{"mossack", "fonseca"})

	s1 := subnets[1]
	c.Check(s1.ProviderId, gc.Equals, network.Id("98"))
	c.Check(s1.VLANTag, gc.Equals, 67)
	c.Check(s1.CIDR, gc.Equals, "192.168.11.0/24")
	c.Check(s1.ProviderSpaceId, gc.Equals, network.Id("4567"))
	c.Check(s1.AvailabilityZones, jc.SameContents, []string{"mossack", "fonseca"})
}

func (suite *maasEnvironSuite) TestSpacesError(c *gc.C) {
	controller := &fakeController{
		spacesError: errors.New("Joe Manginiello"),
	}
	env := suite.makeEnviron(c, controller)
	_, err := env.Spaces(suite.callCtx)
	c.Assert(err, gc.ErrorMatches, "Joe Manginiello")
}

func (suite *maasEnvironSuite) TestSpacesInvalidCredential(c *gc.C) {
	controller := &fakeController{
		spacesError: gomaasapi.NewPermissionError("fail auth here"),
	}
	env := suite.makeEnviron(c, controller)
	c.Assert(suite.invalidCredential, jc.IsFalse)
	_, err := env.Spaces(suite.callCtx)
	c.Assert(err, gc.NotNil)
	c.Assert(suite.invalidCredential, jc.IsTrue)
}

func collectReleaseArgs(controller *fakeController) []gomaasapi.ReleaseMachinesArgs {
	args := []gomaasapi.ReleaseMachinesArgs{}
	for _, call := range controller.Stub.Calls() {
		if call.FuncName == "ReleaseMachines" {
			args = append(args, call.Args[0].(gomaasapi.ReleaseMachinesArgs))
		}
	}
	return args
}

func (suite *maasEnvironSuite) TestStopInstancesReturnsIfParameterEmpty(c *gc.C) {
	controller := newFakeController()
	err := suite.makeEnviron(c, controller).StopInstances(suite.callCtx)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(collectReleaseArgs(controller), gc.HasLen, 0)
}

func (suite *maasEnvironSuite) TestStopInstancesStopsAndReleasesInstances(c *gc.C) {
	// Return a cannot complete indicating that test1 is in the wrong state.
	// The release operation will still release the others and succeed.
	controller := newFakeControllerWithFiles(&fakeFile{name: coretesting.ModelTag.Id() + "-provider-state"})
	err := suite.makeEnviron(c, controller).StopInstances(suite.callCtx, "test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)
	args := collectReleaseArgs(controller)
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0].SystemIDs, gc.DeepEquals, []string{"test1", "test2", "test3"})
}

func (suite *maasEnvironSuite) TestStopInstancesIgnoresConflict(c *gc.C) {
	// Return a cannot complete indicating that test1 is in the wrong state.
	// The release operation will still release the others and succeed.
	controller := newFakeControllerWithFiles(&fakeFile{name: coretesting.ModelTag.Id() + "-provider-state"})
	controller.SetErrors(gomaasapi.NewCannotCompleteError("test1 not allocated"))
	err := suite.makeEnviron(c, controller).StopInstances(suite.callCtx, "test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)

	args := collectReleaseArgs(controller)
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0].SystemIDs, gc.DeepEquals, []string{"test1", "test2", "test3"})
}

func (suite *maasEnvironSuite) TestStopInstancesIgnoresMissingNodeAndRecurses(c *gc.C) {
	controller := newFakeControllerWithFiles(&fakeFile{name: coretesting.ModelTag.Id() + "-provider-state"})
	controller.SetErrors(
		gomaasapi.NewBadRequestError("no such machine: test1"),
		gomaasapi.NewBadRequestError("no such machine: test1"),
	)
	err := suite.makeEnviron(c, controller).StopInstances(suite.callCtx, "test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)
	args := collectReleaseArgs(controller)
	c.Assert(args, gc.HasLen, 4)
	c.Assert(args[0].SystemIDs, gc.DeepEquals, []string{"test1", "test2", "test3"})
	c.Assert(args[1].SystemIDs, gc.DeepEquals, []string{"test1"})
	c.Assert(args[2].SystemIDs, gc.DeepEquals, []string{"test2"})
	c.Assert(args[3].SystemIDs, gc.DeepEquals, []string{"test3"})
}

func (suite *maasEnvironSuite) checkStopInstancesFails(c *gc.C, withError error) {
	controller := newFakeControllerWithFiles(&fakeFile{name: coretesting.ModelTag.Id() + "-provider-state"})
	controller.SetErrors(withError)
	err := suite.makeEnviron(c, controller).StopInstances(suite.callCtx, "test1", "test2", "test3")
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("cannot release nodes: %s", withError))
	// Only tries once.
	c.Assert(collectReleaseArgs(controller), gc.HasLen, 1)
}

func (suite *maasEnvironSuite) TestStopInstancesReturnsUnexpectedMAASError(c *gc.C) {
	suite.checkStopInstancesFails(c, gomaasapi.NewNoMatchError("Something else bad!"))
}

func (suite *maasEnvironSuite) TestStopInstancesReturnsUnexpectedError(c *gc.C) {
	suite.checkStopInstancesFails(c, errors.New("Something completely unexpected!"))
}

func (suite *maasEnvironSuite) TestStartInstanceError(c *gc.C) {
	suite.injectController(&fakeController{
		allocateMachineError: errors.New("Charles Babbage"),
	})
	env := suite.makeEnviron(c, nil)
	_, err := env.StartInstance(suite.callCtx, environs.StartInstanceParams{})
	c.Assert(err, gc.ErrorMatches, "failed to acquire node: Charles Babbage")
}

func (suite *maasEnvironSuite) TestStartInstance(c *gc.C) {
	env, _ := suite.injectControllerWithSpacesAndCheck(c, nil, gomaasapi.AllocateMachineArgs{})

	params := environs.StartInstanceParams{ControllerUUID: suite.controllerUUID}
	result, err := jujutesting.StartInstanceWithParams(env, suite.callCtx, "1", params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Instance.Id(), gc.Equals, instance.Id("Bruce Sterling"))
	c.Assert(result.DisplayName, gc.Equals, "example.com.")
}

func (suite *maasEnvironSuite) TestStartInstanceReturnsHostnameAsDisplayName(c *gc.C) {
	machine := &fakeMachine{
		systemID:     "Bruce Sterling",
		architecture: arch.HostArch(),
		hostname:     "mirrorshades.author",
		Stub:         &testing.Stub{},
		statusName:   "",
	}
	env, _ := suite.injectControllerWithMachine(c, machine, nil, gomaasapi.AllocateMachineArgs{})
	params := environs.StartInstanceParams{ControllerUUID: suite.controllerUUID}
	result, err := jujutesting.StartInstanceWithParams(env, suite.callCtx, "0", params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Instance.Id(), gc.Equals, instance.Id("Bruce Sterling"))
	c.Assert(result.DisplayName, gc.Equals, machine.Hostname())
}

func (suite *maasEnvironSuite) TestStartInstanceReturnsFQDNAsDisplayNameWhenHostnameUnavailable(c *gc.C) {
	machine := &fakeMachine{
		systemID:     "Bruce Sterling",
		architecture: arch.HostArch(),
		hostname:     "",
		Stub:         &testing.Stub{},
		statusName:   "",
	}
	env, _ := suite.injectControllerWithMachine(c, machine, nil, gomaasapi.AllocateMachineArgs{})
	params := environs.StartInstanceParams{ControllerUUID: suite.controllerUUID}
	result, err := jujutesting.StartInstanceWithParams(env, suite.callCtx, "0", params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Instance.Id(), gc.Equals, instance.Id("Bruce Sterling"))
	c.Assert(result.DisplayName, gc.Equals, machine.FQDN())
}

func (suite *maasEnvironSuite) TestStartInstanceAppliesResourceTags(c *gc.C) {
	env, controller := suite.injectControllerWithSpacesAndCheck(c, nil, gomaasapi.AllocateMachineArgs{})
	config := env.Config()
	_, ok := config.ResourceTags()
	c.Assert(ok, jc.IsTrue)
	params := environs.StartInstanceParams{ControllerUUID: suite.controllerUUID}
	_, err := jujutesting.StartInstanceWithParams(env, suite.callCtx, "1", params)
	c.Assert(err, jc.ErrorIsNil)

	machine := controller.allocateMachine.(*fakeMachine)
	machine.CheckCallNames(c, "Start", "SetOwnerData")
	c.Assert(machine.Calls()[1].Args[0], gc.DeepEquals, map[string]string{
		"claude":            "rains",
		tags.JujuController: suite.controllerUUID,
		tags.JujuModel:      config.UUID(),
	})
}

func (suite *maasEnvironSuite) TestStartInstanceParams(c *gc.C) {
	var env *maasEnviron
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, gc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.Config().UUID(),
				Zone:      "foo",
				MinMemory: 8192,
			})
		},
		allocateMachine: newFakeMachine("Bruce Sterling", arch.HostArch(), ""),
		allocateMachineMatches: gomaasapi.ConstraintMatches{
			Storage: map[string][]gomaasapi.StorageDevice{},
		},
		zones: []gomaasapi.Zone{&fakeZone{name: "foo"}},
	})
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)
	params := environs.StartInstanceParams{
		ControllerUUID:   suite.controllerUUID,
		AvailabilityZone: "foo",
		Constraints:      constraints.MustParse("mem=8G"),
	}
	result, err := jujutesting.StartInstanceWithParams(env, suite.callCtx, "1", params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Instance.Id(), gc.Equals, instance.Id("Bruce Sterling"))
}

func (suite *maasEnvironSuite) TestAcquireNodePassedAgentName(c *gc.C) {
	var env *maasEnviron
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, gc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.Config().UUID(),
			})
		},
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
	})
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)

	_, err := env.acquireNode(suite.callCtx, "", "", "", constraints.Value{}, nil, nil, nil)

	c.Check(err, jc.ErrorIsNil)
}

func (suite *maasEnvironSuite) TestAcquireNodePassesPositiveAndNegativeTags(c *gc.C) {
	var env *maasEnviron
	expected := gomaasapi.AllocateMachineArgs{
		Tags:    []string{"tag1", "tag3"},
		NotTags: []string{"tag2", "tag4"},
	}
	env, _ = suite.injectControllerWithSpacesAndCheck(c, nil, expected)
	_, err := env.acquireNode(suite.callCtx,
		"", "", "",
		constraints.Value{Tags: stringslicep("tag1", "^tag2", "tag3", "^tag4")},
		nil, nil, nil,
	)
	c.Check(err, jc.ErrorIsNil)
}

func getFourSpaces() []gomaasapi.Space {
	return []gomaasapi.Space{
		fakeSpace{
			name:    "space-1",
			subnets: []gomaasapi.Subnet{fakeSubnet{id: 99, vlan: fakeVLAN{vid: 66}, cidr: "192.168.10.0/24"}},
			id:      5,
		},
		fakeSpace{
			name:    "space-2",
			subnets: []gomaasapi.Subnet{fakeSubnet{id: 100, vlan: fakeVLAN{vid: 66}, cidr: "192.168.11.0/24"}},
			id:      6,
		},
		fakeSpace{
			name:    "space-3",
			subnets: []gomaasapi.Subnet{fakeSubnet{id: 101, vlan: fakeVLAN{vid: 66}, cidr: "192.168.12.0/24"}},
			id:      7,
		},
		fakeSpace{
			name:    "space-4",
			subnets: []gomaasapi.Subnet{fakeSubnet{id: 102, vlan: fakeVLAN{vid: 66}, cidr: "192.168.13.0/24"}},
			id:      8,
		},
	}
}

func (suite *maasEnvironSuite) TestAcquireNodePassesPositiveAndNegativeSpaces(c *gc.C) {
	expected := gomaasapi.AllocateMachineArgs{
		NotSpace: []string{"6", "8"},
		Interfaces: []gomaasapi.InterfaceSpec{
			{Label: "5", Space: "5"},
			{Label: "7", Space: "7"},
		},
	}
	env, _ := suite.injectControllerWithSpacesAndCheck(c, getFourSpaces(), expected)

	cons := constraints.Value{Spaces: stringslicep("space-1", "^space-2", "space-3", "^space-4")}
	positiveSpaceIDs, negativeSpaceIDs, err := env.networkSpaceRequirements(suite.callCtx, nil, cons)
	c.Check(err, jc.ErrorIsNil)

	_, err = env.acquireNode(suite.callCtx,
		"", "", "",
		cons,
		positiveSpaceIDs, negativeSpaceIDs,
		nil,
	)
	c.Check(err, jc.ErrorIsNil)
}

func (suite *maasEnvironSuite) TestAcquireNodeStorage(c *gc.C) {
	var env *maasEnviron
	var getStorage func() []gomaasapi.StorageSpec
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, jc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.Config().UUID(),
				Storage:   getStorage(),
			})
		},
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
	})
	suite.setupFakeTools(c)
	for i, test := range []struct {
		volumes  []volumeInfo
		expected []gomaasapi.StorageSpec
	}{{
		volumes:  nil,
		expected: []gomaasapi.StorageSpec{},
	}, {
		volumes:  []volumeInfo{{"volume-1", 1234, nil}},
		expected: []gomaasapi.StorageSpec{{"volume-1", 1234, nil}},
	}, {
		volumes:  []volumeInfo{{"", 1234, []string{"tag1", "tag2"}}},
		expected: []gomaasapi.StorageSpec{{"", 1234, []string{"tag1", "tag2"}}},
	}, {
		volumes:  []volumeInfo{{"volume-1", 1234, []string{"tag1", "tag2"}}},
		expected: []gomaasapi.StorageSpec{{"volume-1", 1234, []string{"tag1", "tag2"}}},
	}, {
		volumes: []volumeInfo{
			{"volume-1", 1234, []string{"tag1", "tag2"}},
			{"volume-2", 4567, []string{"tag1", "tag3"}},
		},
		expected: []gomaasapi.StorageSpec{
			{"volume-1", 1234, []string{"tag1", "tag2"}},
			{"volume-2", 4567, []string{"tag1", "tag3"}},
		},
	}} {
		c.Logf("test #%d: volumes=%v", i, test.volumes)
		getStorage = func() []gomaasapi.StorageSpec {
			return test.expected
		}
		env = suite.makeEnviron(c, nil)
		_, err := env.acquireNode(suite.callCtx, "", "", "", constraints.Value{}, nil, nil, test.volumes)
		c.Check(err, jc.ErrorIsNil)
	}
}

func (suite *maasEnvironSuite) TestAcquireNodeInterfaces(c *gc.C) {
	var env *maasEnviron
	var getNegatives func() []string
	var getPositives func() []gomaasapi.InterfaceSpec
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, gc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName:  env.Config().UUID(),
				Interfaces: getPositives(),
				NotSpace:   getNegatives(),
			})
		},
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
		spaces: getTwoSpaces(),
	})
	suite.setupFakeTools(c)

	// Add some constraints, including spaces to verify specified bindings
	// always override any spaces constraints.
	cons := constraints.Value{
		Spaces: stringslicep("foo", "^bar"),
	}
	// In the tests below Space 2 means foo, Space 3 means bar.
	for i, test := range []struct {
		descr             string
		endpointBindings  map[string]network.Id
		expectedPositives []gomaasapi.InterfaceSpec
		expectedNegatives []string
		expectedError     string
	}{{
		descr:             "no bindings and space constraints",
		expectedPositives: []gomaasapi.InterfaceSpec{{Label: "2", Space: "2"}},
		expectedNegatives: []string{"3"},
		expectedError:     "",
	}, {
		descr:            "bindings and no space constraints",
		endpointBindings: map[string]network.Id{"name-1": "space-1"},
		expectedPositives: []gomaasapi.InterfaceSpec{
			{Label: "2", Space: "2"},
			{Label: "space-1", Space: "space-1"},
		},
		expectedNegatives: []string{"3"},
	}, {
		descr: "bindings (to the same provider space ID) and space constraints",
		endpointBindings: map[string]network.Id{
			"":         "999", // we should get a NIC in this space even if none of the endpoints are bound to it
			"name-1":   "1",
			"name-2":   "1",
			"name-3":   "2",
			"name-4":   "42",
			"to-alpha": network.AlphaSpaceName, // alpha space is not present on maas and is skipped
		},
		expectedPositives: []gomaasapi.InterfaceSpec{
			{Label: "1", Space: "1"},
			{Label: "2", Space: "2"},
			{Label: "42", Space: "42"},
			{Label: "999", Space: "999"},
		},
		expectedNegatives: []string{"3"},
	}} {
		c.Logf("test #%d: %s", i, test.descr)

		env = suite.makeEnviron(c, nil)
		getNegatives = func() []string {
			return test.expectedNegatives
		}
		getPositives = func() []gomaasapi.InterfaceSpec {
			return test.expectedPositives
		}

		positiveSpaceIDs, negativeSpaceIDs, err := env.networkSpaceRequirements(suite.callCtx, test.endpointBindings, cons)
		c.Check(err, jc.ErrorIsNil)

		_, err = env.acquireNode(suite.callCtx, "", "", "", cons, positiveSpaceIDs, negativeSpaceIDs, nil)
		if test.expectedError != "" {
			c.Check(err, gc.ErrorMatches, test.expectedError)
			c.Check(err, jc.ErrorIs, errors.NotValid)
			continue
		}
		c.Check(err, jc.ErrorIsNil)
	}
}

func getTwoSpaces() []gomaasapi.Space {
	return []gomaasapi.Space{
		fakeSpace{
			name:    "foo",
			subnets: []gomaasapi.Subnet{fakeSubnet{id: 99, vlan: fakeVLAN{vid: 66}, cidr: "192.168.10.0/24"}},
			id:      2,
		},
		fakeSpace{
			name:    "bar",
			subnets: []gomaasapi.Subnet{fakeSubnet{id: 100, vlan: fakeVLAN{vid: 66}, cidr: "192.168.11.0/24"}},
			id:      3,
		},
	}
}

func (suite *maasEnvironSuite) TestWaitForNodeDeploymentError(c *gc.C) {
	machine := newFakeMachine("Bruce Sterling", arch.HostArch(), "")
	controller := newFakeController()
	controller.allocateMachine = machine
	controller.allocateMachineMatches = gomaasapi.ConstraintMatches{
		Storage: map[string][]gomaasapi.StorageDevice{},
	}
	controller.machines = []gomaasapi.Machine{machine}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env := suite.makeEnviron(c, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		suite.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             jujutesting.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
			DialOpts: environs.BootstrapDialOpts{
				Timeout: coretesting.LongWait,
			},
		})
	c.Assert(err, gc.ErrorMatches, "bootstrap instance started but did not change to Deployed state.*")
}

func (suite *maasEnvironSuite) TestWaitForNodeDeploymentRetry(c *gc.C) {
	machine := newFakeMachine("Inaccessible machine", arch.HostArch(), "")
	controller := newFakeController()
	controller.allocateMachine = machine
	controller.allocateMachineMatches = gomaasapi.ConstraintMatches{
		Storage: map[string][]gomaasapi.StorageDevice{},
	}
	controller.machines = []gomaasapi.Machine{}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env := suite.makeEnviron(c, nil)
	bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		suite.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             jujutesting.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
			DialOpts: environs.BootstrapDialOpts{
				Timeout: coretesting.LongWait,
			},
		})
	c.Check(c.GetTestLog(), jc.Contains, "WARNING juju.provider.maas failed to get instance from provider attempt")
}

func (suite *maasEnvironSuite) TestWaitForNodeDeploymentSucceeds(c *gc.C) {
	machine := newFakeMachine("Bruce Sterling", arch.HostArch(), "Deployed")
	controller := newFakeController()
	controller.allocateMachine = machine
	controller.allocateMachineMatches = gomaasapi.ConstraintMatches{
		Storage: map[string][]gomaasapi.StorageDevice{},
	}
	controller.machines = []gomaasapi.Machine{machine}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env := suite.makeEnviron(c, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		suite.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             jujutesting.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
			DialOpts: environs.BootstrapDialOpts{
				Timeout: coretesting.LongWait,
			},
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *maasEnvironSuite) TestSubnetsNoFilters(c *gc.C) {
	suite.injectController(&fakeController{
		spaces: getFourSpaces(),
	})
	env := suite.makeEnviron(c, nil)
	subnets, err := env.Subnets(suite.callCtx, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	expected := []network.SubnetInfo{
		{CIDR: "192.168.10.0/24", ProviderId: "99", VLANTag: 66, ProviderSpaceId: "5"},
		{CIDR: "192.168.11.0/24", ProviderId: "100", VLANTag: 66, ProviderSpaceId: "6"},
		{CIDR: "192.168.12.0/24", ProviderId: "101", VLANTag: 66, ProviderSpaceId: "7"},
		{CIDR: "192.168.13.0/24", ProviderId: "102", VLANTag: 66, ProviderSpaceId: "8"},
	}
	c.Assert(subnets, jc.DeepEquals, expected)
}

func (suite *maasEnvironSuite) TestSubnetsNoFiltersError(c *gc.C) {
	suite.injectController(&fakeController{
		spacesError: errors.New("bang"),
	})
	env := suite.makeEnviron(c, nil)
	_, err := env.Subnets(suite.callCtx, "", nil)
	c.Assert(err, gc.ErrorMatches, "bang")
}

func (suite *maasEnvironSuite) TestSubnetsSubnetIds(c *gc.C) {
	suite.injectController(&fakeController{
		spaces: getFourSpaces(),
	})
	env := suite.makeEnviron(c, nil)
	subnets, err := env.Subnets(suite.callCtx, "", []network.Id{"99", "100"})
	c.Assert(err, jc.ErrorIsNil)
	expected := []network.SubnetInfo{
		{CIDR: "192.168.10.0/24", ProviderId: "99", VLANTag: 66, ProviderSpaceId: "5"},
		{CIDR: "192.168.11.0/24", ProviderId: "100", VLANTag: 66, ProviderSpaceId: "6"},
	}
	c.Assert(subnets, jc.DeepEquals, expected)
}

func (suite *maasEnvironSuite) TestSubnetsSubnetIdsMissing(c *gc.C) {
	suite.injectController(&fakeController{
		spaces: getFourSpaces(),
	})
	env := suite.makeEnviron(c, nil)
	_, err := env.Subnets(suite.callCtx, "", []network.Id{"99", "missing"})
	msg := "failed to find the following subnets: missing"
	c.Assert(err, gc.ErrorMatches, msg)
}

func (suite *maasEnvironSuite) TestSubnetsInstIdNotFound(c *gc.C) {
	suite.injectController(&fakeController{})
	env := suite.makeEnviron(c, nil)
	_, err := env.Subnets(suite.callCtx, "foo", nil)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (suite *maasEnvironSuite) TestSubnetsInstId(c *gc.C) {
	interfaces := []gomaasapi.Interface{
		&fakeInterface{
			links: []gomaasapi.Link{
				&fakeLink{subnet: fakeSubnet{id: 99, vlan: fakeVLAN{vid: 66}, cidr: "192.168.10.0/24", space: "space-1"}},
				&fakeLink{subnet: fakeSubnet{id: 100, vlan: fakeVLAN{vid: 0}, cidr: "192.168.11.0/24", space: "space-2"}},
			},
		},
		&fakeInterface{
			links: []gomaasapi.Link{
				&fakeLink{subnet: fakeSubnet{id: 101, vlan: fakeVLAN{vid: 2}, cidr: "192.168.12.0/24", space: "space-3"}},
			},
		},
	}
	machine := &fakeMachine{
		systemID:     "William Gibson",
		interfaceSet: interfaces,
	}
	machine2 := &fakeMachine{systemID: "Bruce Sterling"}
	suite.injectController(&fakeController{
		machines: []gomaasapi.Machine{machine, machine2},
		spaces:   getFourSpaces(),
	})
	env := suite.makeEnviron(c, nil)
	subnets, err := env.Subnets(suite.callCtx, "William Gibson", nil)
	c.Assert(err, jc.ErrorIsNil)
	expected := []network.SubnetInfo{
		{CIDR: "192.168.10.0/24", ProviderId: "99", VLANTag: 66, ProviderSpaceId: "5"},
		{CIDR: "192.168.11.0/24", ProviderId: "100", VLANTag: 0, ProviderSpaceId: "6"},
		{CIDR: "192.168.12.0/24", ProviderId: "101", VLANTag: 2, ProviderSpaceId: "7"},
	}
	c.Assert(subnets, jc.DeepEquals, expected)
}

func (suite *maasEnvironSuite) TestStartInstanceNetworkInterfaces(c *gc.C) {
	vlan0 := fakeVLAN{
		id:  5001,
		vid: 0,
		mtu: 1500,
	}

	vlan50 := fakeVLAN{
		id:  5004,
		vid: 50,
		mtu: 1500,
	}

	subnetPXE := fakeSubnet{
		id:         3,
		space:      "default",
		vlan:       vlan0,
		gateway:    "10.20.19.2",
		cidr:       "10.20.19.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}

	exampleInterfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         91,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan0,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        436,
					subnet:    &subnetPXE,
					ipAddress: "10.20.19.103",
					mode:      "static",
				},
				&fakeLink{
					id:        437,
					subnet:    &subnetPXE,
					ipAddress: "10.20.19.104",
					mode:      "static",
				},
			},
			parents:  []string{},
			children: []string{"eth0.100", "eth0.250", "eth0.50"},
		},
		&fakeInterface{
			id:         150,
			name:       "eth0.50",
			type_:      "vlan",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan50,
			links: []gomaasapi.Link{
				&fakeLink{
					id: 517,
					subnet: &fakeSubnet{
						id:         5,
						space:      "admin",
						vlan:       vlan50,
						gateway:    "10.50.19.2",
						cidr:       "10.50.19.0/24",
						dnsServers: []string{},
					},
					ipAddress: "10.50.19.103",
					mode:      "static",
				},
			},
			parents:  []string{"eth0"},
			children: []string{},
		},
	}
	var env *maasEnviron
	machine := newFakeMachine("Bruce Sterling", arch.HostArch(), "")
	machine.interfaceSet = exampleInterfaces
	controller := &fakeController{
		allocateMachine: machine,
		allocateMachineMatches: gomaasapi.ConstraintMatches{
			Storage: map[string][]gomaasapi.StorageDevice{},
		},
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)

	params := environs.StartInstanceParams{ControllerUUID: suite.controllerUUID}
	result, err := jujutesting.StartInstanceWithParams(env, suite.callCtx, "1", params)
	c.Assert(err, jc.ErrorIsNil)
	expected := network.InterfaceInfos{{
		DeviceIndex:       0,
		MACAddress:        "52:54:00:70:9b:fe",
		ProviderId:        "91",
		ProviderSubnetId:  "3",
		AvailabilityZones: nil,
		VLANTag:           0,
		ProviderVLANId:    "5001",
		ProviderAddressId: "436",
		InterfaceName:     "eth0",
		InterfaceType:     "ethernet",
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.20.19.103", network.WithCIDR("10.20.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("default")),
		},
		DNSServers: network.NewMachineAddresses([]string{
			"10.20.19.2",
			"10.20.19.3",
		}).AsProviderAddresses(network.WithSpaceName("default")),
		DNSSearchDomains: nil,
		MTU:              1500,
		GatewayAddress:   network.NewMachineAddress("10.20.19.2").AsProviderAddress(network.WithSpaceName("default")),
		Origin:           network.OriginProvider,
	}, {
		DeviceIndex:       0,
		MACAddress:        "52:54:00:70:9b:fe",
		ProviderId:        "91",
		ProviderSubnetId:  "3",
		AvailabilityZones: nil,
		VLANTag:           0,
		ProviderVLANId:    "5001",
		ProviderAddressId: "437",
		InterfaceName:     "eth0",
		InterfaceType:     "ethernet",
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.20.19.104", network.WithCIDR("10.20.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("default")),
		},
		DNSServers: network.NewMachineAddresses([]string{
			"10.20.19.2",
			"10.20.19.3",
		}).AsProviderAddresses(network.WithSpaceName("default")),
		DNSSearchDomains: nil,
		MTU:              1500,
		GatewayAddress:   network.NewMachineAddress("10.20.19.2").AsProviderAddress(network.WithSpaceName("default")),
		Origin:           network.OriginProvider,
	}, {
		DeviceIndex:         1,
		MACAddress:          "52:54:00:70:9b:fe",
		ProviderId:          "150",
		ProviderSubnetId:    "5",
		AvailabilityZones:   nil,
		VLANTag:             50,
		ProviderVLANId:      "5004",
		ProviderAddressId:   "517",
		InterfaceName:       "eth0.50",
		ParentInterfaceName: "eth0",
		InterfaceType:       "802.1q",
		Disabled:            false,
		NoAutoStart:         false,
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.50.19.103", network.WithCIDR("10.50.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("admin")),
		},
		DNSServers:       nil,
		DNSSearchDomains: nil,
		MTU:              1500,
		GatewayAddress:   network.NewMachineAddress("10.50.19.2").AsProviderAddress(network.WithSpaceName("admin")),
		Origin:           network.OriginProvider,
	}}
	c.Assert(result.NetworkInfo, jc.DeepEquals, expected)
}

func (suite *maasEnvironSuite) TestAllocateContainerAddressesSingleNic(c *gc.C) {
	vlan1 := fakeVLAN{
		id:  5001,
		mtu: 1500,
	}
	vlan2 := fakeVLAN{
		id:  5002,
		mtu: 1500,
	}
	subnet1 := fakeSubnet{
		id:         3,
		space:      "default",
		vlan:       vlan1,
		gateway:    "10.20.19.2",
		cidr:       "10.20.19.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}
	subnet2 := fakeSubnet{
		id:         4,
		space:      "freckles",
		vlan:       vlan2,
		gateway:    "192.168.1.1",
		cidr:       "192.168.1.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}
	staticRoute2to1 := fakeStaticRoute{
		id:          1,
		source:      subnet2,
		destination: subnet1,
		gatewayIP:   "192.168.1.1",
		metric:      100,
	}

	interfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         91,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan1,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        436,
					subnet:    &subnet1,
					ipAddress: "10.20.19.103",
					mode:      "static",
				},
			},
			parents:  []string{},
			children: []string{"eth0.100", "eth0.250", "eth0.50"},
		},
	}
	deviceInterfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         93,
			name:       "eth1",
			type_:      "physical",
			enabled:    true,
			macAddress: "53:54:00:70:9b:ff",
			vlan:       vlan2,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        480,
					subnet:    &subnet2,
					ipAddress: "192.168.1.127",
					mode:      "static",
				},
			},
			parents:  []string{},
			children: []string{"eth0.100", "eth0.250", "eth0.50"},
		},
	}
	var env *maasEnviron
	device := &fakeDevice{
		interfaceSet: deviceInterfaces,
		systemID:     "foo",
	}
	controller := &fakeController{
		Stub: &testing.Stub{},
		machines: []gomaasapi.Machine{&fakeMachine{
			Stub:         &testing.Stub{},
			systemID:     "1",
			architecture: arch.HostArch(),
			interfaceSet: interfaces,
			createDevice: device,
		}},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "freckles",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet1, subnet2},
			},
		},
		devices:      []gomaasapi.Device{device},
		staticRoutes: []gomaasapi.StaticRoute{staticRoute2to1},
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)

	prepared := network.InterfaceInfos{{
		MACAddress:    "52:54:00:70:9b:fe",
		Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.19.0/24")).AsProviderAddress()},
		InterfaceName: "eth0",
	}}
	ignored := names.NewMachineTag("1/lxd/0")
	result, err := env.AllocateContainerAddresses(suite.callCtx, "1", ignored, prepared)
	c.Assert(err, jc.ErrorIsNil)
	expected := network.InterfaceInfos{{
		DeviceIndex:       0,
		MACAddress:        "53:54:00:70:9b:ff",
		ProviderId:        "93",
		ProviderSubnetId:  "4",
		VLANTag:           0,
		ProviderVLANId:    "5002",
		ProviderAddressId: "480",
		InterfaceName:     "eth1",
		InterfaceType:     "ethernet",
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"192.168.1.127", network.WithCIDR("192.168.1.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("freckles")),
		},
		DNSServers: network.NewMachineAddresses([]string{
			"10.20.19.2",
			"10.20.19.3",
		}).AsProviderAddresses(network.WithSpaceName("freckles")),
		MTU:            1500,
		GatewayAddress: network.NewMachineAddress("192.168.1.1").AsProviderAddress(network.WithSpaceName("freckles")),
		Routes: []network.Route{{
			DestinationCIDR: subnet1.CIDR(),
			GatewayIP:       "192.168.1.1",
			Metric:          100,
		}},
		Origin: network.OriginProvider,
	}}
	c.Assert(result, jc.DeepEquals, expected)
}

func (suite *maasEnvironSuite) TestAllocateContainerAddressesSingleNicWithNoVLAN(c *gc.C) {
	vlan1 := fakeVLAN{
		id:  5001,
		mtu: 1500,
	}
	vlan2 := fakeVLAN{
		id:  5002,
		mtu: 1500,
	}
	subnet1 := fakeSubnet{
		id:         3,
		space:      "default",
		vlan:       vlan1,
		gateway:    "10.20.19.2",
		cidr:       "10.20.19.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}
	subnet2 := fakeSubnet{
		id:         4,
		space:      "freckles",
		vlan:       vlan2,
		gateway:    "192.168.1.1",
		cidr:       "192.168.1.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}
	staticRoute2to1 := fakeStaticRoute{
		id:          1,
		source:      subnet2,
		destination: subnet1,
		gatewayIP:   "192.168.1.1",
		metric:      100,
	}

	interfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         91,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan1,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        436,
					subnet:    &subnet1,
					ipAddress: "10.20.19.103",
					mode:      "static",
				},
			},
			parents:  []string{},
			children: []string{"eth0.100", "eth0.250", "eth0.50"},
		},
	}
	deviceInterfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         93,
			name:       "eth1",
			type_:      "physical",
			enabled:    true,
			macAddress: "53:54:00:70:9b:ff",
			links: []gomaasapi.Link{
				&fakeLink{
					id:        480,
					subnet:    &subnet2,
					ipAddress: "192.168.1.127",
					mode:      "static",
				},
			},
			parents:  []string{},
			children: []string{"eth0.100", "eth0.250", "eth0.50"},
		},
	}
	var env *maasEnviron
	device := &fakeDevice{
		interfaceSet: deviceInterfaces,
		systemID:     "foo",
	}
	controller := &fakeController{
		Stub: &testing.Stub{},
		machines: []gomaasapi.Machine{&fakeMachine{
			Stub:         &testing.Stub{},
			systemID:     "1",
			architecture: arch.HostArch(),
			interfaceSet: interfaces,
			createDevice: device,
		}},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "freckles",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet1, subnet2},
			},
		},
		devices:      []gomaasapi.Device{device},
		staticRoutes: []gomaasapi.StaticRoute{staticRoute2to1},
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)

	prepared := network.InterfaceInfos{{
		MACAddress:    "52:54:00:70:9b:fe",
		Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.19.0/24")).AsProviderAddress()},
		InterfaceName: "eth0",
	}}
	ignored := names.NewMachineTag("1/lxd/0")
	result, err := env.AllocateContainerAddresses(suite.callCtx, "1", ignored, prepared)
	c.Assert(err, jc.ErrorIsNil)
	expected := network.InterfaceInfos{{
		DeviceIndex:       0,
		MACAddress:        "53:54:00:70:9b:ff",
		ProviderId:        "93",
		ProviderSubnetId:  "4",
		VLANTag:           0,
		ProviderVLANId:    "0",
		ProviderAddressId: "480",
		InterfaceName:     "eth1",
		InterfaceType:     "ethernet",
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"192.168.1.127", network.WithCIDR("192.168.1.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("freckles")),
		},
		DNSServers: network.NewMachineAddresses([]string{
			"10.20.19.2",
			"10.20.19.3",
		}).AsProviderAddresses(network.WithSpaceName("freckles")),
		MTU:            1500,
		GatewayAddress: network.NewMachineAddress("192.168.1.1").AsProviderAddress(network.WithSpaceName("freckles")),
		Routes: []network.Route{{
			DestinationCIDR: subnet1.CIDR(),
			GatewayIP:       "192.168.1.1",
			Metric:          100,
		}},
		Origin: network.OriginProvider,
	}}
	c.Assert(result, jc.DeepEquals, expected)
}

func (suite *maasEnvironSuite) TestAllocateContainerAddressesNoStaticRoutesAPI(c *gc.C) {
	// MAAS 2.0 doesn't have support for static routes, and generates an Error
	vlan1 := fakeVLAN{
		id:  5001,
		mtu: 1500,
	}
	subnet1 := fakeSubnet{
		id:         3,
		space:      "freckles",
		vlan:       vlan1,
		gateway:    "10.20.19.2",
		cidr:       "10.20.19.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}

	interfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         91,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan1,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        436,
					subnet:    &subnet1,
					ipAddress: "10.20.19.103",
					mode:      "static",
				},
			},
			parents:  []string{},
			children: []string{"eth0.100", "eth0.250", "eth0.50"},
		},
	}
	// This will be returned by the fakeController when we call CreateDevice
	deviceInterfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         93,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "53:54:00:70:9b:ff",
			vlan:       vlan1,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        480,
					subnet:    &subnet1,
					ipAddress: "10.20.19.104",
					mode:      "static",
				},
			},
			parents:  []string{},
			children: []string{},
		},
	}
	stub := &testing.Stub{}
	device := &fakeDevice{
		Stub:         stub,
		interfaceSet: deviceInterfaces,
		systemID:     "foo",
	}
	// MAAS 2.0 gives us this kind of error back, I'm not sure of the conten of
	// the Headers or BodyMessage, but it is a 404 with a particular error
	// string that we've seen.
	body := "Unknown API endpoint: /MAAS/api/2.0/static-routes/."
	notFound := gomaasapi.ServerError{
		StatusCode:  http.StatusNotFound,
		BodyMessage: body,
	}
	wrap1 := errors.Annotatef(notFound, "ServerError: 404 NOT FOUND (%s)", body)
	staticRoutesErr := gomaasapi.NewUnexpectedError(wrap1)
	var env *maasEnviron
	controller := &fakeController{
		Stub: stub,
		machines: []gomaasapi.Machine{&fakeMachine{
			Stub:         stub,
			systemID:     "1",
			architecture: arch.HostArch(),
			interfaceSet: interfaces,
			createDevice: device,
		}},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "freckles",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet1},
			},
		},
		devices:           []gomaasapi.Device{device},
		staticRoutesError: staticRoutesErr,
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)

	prepared := network.InterfaceInfos{{
		MACAddress:    "52:54:00:70:9b:fe",
		Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.19.0/24")).AsProviderAddress()},
		InterfaceName: "eth0",
	}}
	ignored := names.NewMachineTag("1/lxd/0")
	result, err := env.AllocateContainerAddresses(suite.callCtx, instance.Id("1"), ignored, prepared)
	c.Assert(err, jc.ErrorIsNil)
	expected := network.InterfaceInfos{{
		DeviceIndex:       0,
		MACAddress:        "53:54:00:70:9b:ff",
		ProviderId:        "93",
		ProviderSubnetId:  "3",
		VLANTag:           0,
		ProviderVLANId:    "5001",
		ProviderAddressId: "480",
		InterfaceName:     "eth0",
		InterfaceType:     "ethernet",
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.20.19.104", network.WithCIDR("10.20.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("freckles")),
		},
		DNSServers: network.NewMachineAddresses([]string{
			"10.20.19.2",
			"10.20.19.3",
		}).AsProviderAddresses(network.WithSpaceName("freckles")),
		MTU:            1500,
		GatewayAddress: network.NewMachineAddress("10.20.19.2").AsProviderAddress(network.WithSpaceName("freckles")),
		Routes:         []network.Route{},
		Origin:         network.OriginProvider,
	}}
	c.Assert(result, jc.DeepEquals, expected)
}

func (suite *maasEnvironSuite) TestAllocateContainerAddressesStaticRoutesDenied(c *gc.C) {
	// I don't have a specific error that we've triggered, but we want to make
	// sure that we don't suppress all error responses from MAAS just because
	// we know we want to skip 404
	vlan1 := fakeVLAN{
		id:  5001,
		mtu: 1500,
	}
	subnet1 := fakeSubnet{
		id:         3,
		space:      "freckles",
		vlan:       vlan1,
		gateway:    "10.20.19.2",
		cidr:       "10.20.19.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}

	interfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         91,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan1,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        436,
					subnet:    &subnet1,
					ipAddress: "10.20.19.103",
					mode:      "static",
				},
			},
			parents:  []string{},
			children: []string{"eth0.100", "eth0.250", "eth0.50"},
		},
	}
	body := "I have failed you"
	internalError := gomaasapi.ServerError{
		StatusCode:  http.StatusInternalServerError,
		BodyMessage: body,
	}
	staticRoutesErr := errors.Annotatef(internalError, "ServerError: %v (%s)", http.StatusInternalServerError, body)
	var env *maasEnviron
	controller := &fakeController{
		Stub: &testing.Stub{},
		machines: []gomaasapi.Machine{&fakeMachine{
			Stub:         &testing.Stub{},
			systemID:     "1",
			architecture: arch.HostArch(),
			interfaceSet: interfaces,
		}},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "freckles",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet1},
			},
		},
		staticRoutesError: staticRoutesErr,
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)

	prepared := network.InterfaceInfos{{
		MACAddress:    "52:54:00:70:9b:fe",
		Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.19.0/24")).AsProviderAddress()},
		InterfaceName: "eth0",
	}}
	ignored := names.NewMachineTag("1/lxd/0")
	_, err := env.AllocateContainerAddresses(suite.callCtx, "1", ignored, prepared)
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, ".*ServerError: 500 \\(I have failed you\\).*")
}

func (suite *maasEnvironSuite) TestAllocateContainerAddressesDualNic(c *gc.C) {
	vlan1 := fakeVLAN{
		id:  5001,
		mtu: 1500,
	}
	vlan2 := fakeVLAN{
		id:  5002,
		mtu: 1500,
	}
	subnet1 := fakeSubnet{
		id:         3,
		space:      "freckles",
		vlan:       vlan1,
		gateway:    "10.20.19.2",
		cidr:       "10.20.19.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}
	subnet2 := fakeSubnet{
		id:         4,
		space:      "freckles",
		vlan:       vlan2,
		gateway:    "192.168.1.1",
		cidr:       "192.168.1.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}
	staticRoute2to1 := fakeStaticRoute{
		id:          1,
		source:      subnet2,
		destination: subnet1,
		gatewayIP:   "192.168.1.1",
		metric:      100,
	}

	interfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         91,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan1,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        436,
					subnet:    &subnet1,
					ipAddress: "10.20.19.103",
					mode:      "static",
				},
			},
			parents:  []string{},
			children: []string{"eth0.100", "eth0.250", "eth0.50"},
		},
		&fakeInterface{
			id:         92,
			name:       "eth1",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:ff",
			vlan:       vlan2,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        437,
					subnet:    &subnet2,
					ipAddress: "192.168.1.4",
					mode:      "static",
				},
			},
		},
	}
	deviceInterfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         93,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "53:54:00:70:9b:ff",
			vlan:       vlan1,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        480,
					subnet:    &subnet1,
					ipAddress: "10.20.19.127",
					mode:      "static",
				},
			},
			parents:  []string{},
			children: []string{"eth0.100", "eth0.250", "eth0.50"},
			Stub:     &testing.Stub{},
		},
	}
	newInterface := &fakeInterface{
		id:         94,
		name:       "eth1",
		type_:      "physical",
		enabled:    true,
		macAddress: "52:54:00:70:9b:f4",
		vlan:       vlan2,
		links: []gomaasapi.Link{
			&fakeLink{
				id:        481,
				subnet:    &subnet2,
				ipAddress: "192.168.1.127",
				mode:      "static",
			},
		},
		Stub: &testing.Stub{},
	}
	device := &fakeDevice{
		interfaceSet: deviceInterfaces,
		systemID:     "foo",
		interface_:   newInterface,
		Stub:         &testing.Stub{},
	}
	controller := &fakeController{
		Stub: &testing.Stub{},
		machines: []gomaasapi.Machine{&fakeMachine{
			Stub:         &testing.Stub{},
			systemID:     "1",
			architecture: arch.HostArch(),
			interfaceSet: interfaces,
			createDevice: device,
		}},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "freckles",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet1, subnet2},
			},
		},
		devices:      []gomaasapi.Device{device},
		staticRoutes: []gomaasapi.StaticRoute{staticRoute2to1},
	}
	suite.injectController(controller)
	env := suite.makeEnviron(c, nil)

	prepared := network.InterfaceInfos{{
		MACAddress:    "53:54:00:70:9b:ff",
		Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.19.0/24")).AsProviderAddress()},
		InterfaceName: "eth0",
	}, {
		MACAddress:    "52:54:00:70:9b:f4",
		Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("192.168.1.0/24")).AsProviderAddress()},
		InterfaceName: "eth1",
	}}
	expected := network.InterfaceInfos{{
		DeviceIndex:       0,
		MACAddress:        "53:54:00:70:9b:ff",
		ProviderId:        "93",
		ProviderSubnetId:  "3",
		ProviderVLANId:    "5001",
		ProviderAddressId: "480",
		InterfaceName:     "eth0",
		InterfaceType:     "ethernet",
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.20.19.127", network.WithCIDR("10.20.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("freckles")),
		},
		DNSServers: network.NewMachineAddresses([]string{
			"10.20.19.2",
			"10.20.19.3",
		}).AsProviderAddresses(network.WithSpaceName("freckles")),
		MTU:            1500,
		GatewayAddress: network.NewMachineAddress("10.20.19.2").AsProviderAddress(network.WithSpaceName("freckles")),
		Origin:         network.OriginProvider,
	}, {
		DeviceIndex:       1,
		MACAddress:        "52:54:00:70:9b:f4",
		ProviderId:        "94",
		ProviderSubnetId:  "4",
		ProviderVLANId:    "5002",
		ProviderAddressId: "481",
		InterfaceName:     "eth1",
		InterfaceType:     "ethernet",
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"192.168.1.127", network.WithCIDR("192.168.1.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("freckles")),
		},
		DNSServers: network.NewMachineAddresses([]string{
			"10.20.19.2",
			"10.20.19.3",
		}).AsProviderAddresses(network.WithSpaceName("freckles")),
		MTU:            1500,
		GatewayAddress: network.NewMachineAddress("192.168.1.1").AsProviderAddress(network.WithSpaceName("freckles")),
		Routes: []network.Route{{
			DestinationCIDR: "10.20.19.0/24",
			GatewayIP:       "192.168.1.1",
			Metric:          100,
		}},
		Origin: network.OriginProvider,
	}}
	ignored := names.NewMachineTag("1/lxd/0")
	result, err := env.AllocateContainerAddresses(suite.callCtx, instance.Id("1"), ignored, prepared)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expected)
}

func (suite *maasEnvironSuite) assertAllocateContainerAddressesFails(c *gc.C, controller *fakeController, prepared network.InterfaceInfos, errorMatches string) {
	if prepared == nil {
		prepared = network.InterfaceInfos{{}}
	}
	suite.injectController(controller)
	env := suite.makeEnviron(c, nil)
	ignored := names.NewMachineTag("1/lxd/0")
	_, err := env.AllocateContainerAddresses(suite.callCtx, instance.Id("1"), ignored, prepared)
	c.Assert(err, gc.ErrorMatches, errorMatches)
}

func (suite *maasEnvironSuite) TestAllocateContainerAddressesSpacesError(c *gc.C) {
	machine := &fakeMachine{
		Stub:     &testing.Stub{},
		systemID: "1",
	}
	controller := &fakeController{
		machines:    []gomaasapi.Machine{machine},
		spacesError: errors.New("boom"),
	}
	suite.assertAllocateContainerAddressesFails(c, controller, nil, "boom")
}

func (suite *maasEnvironSuite) TestAllocateContainerAddressesPrimaryInterfaceMissing(c *gc.C) {
	machine := &fakeMachine{
		Stub:     &testing.Stub{},
		systemID: "1",
	}
	controller := &fakeController{
		machines: []gomaasapi.Machine{machine},
	}
	suite.assertAllocateContainerAddressesFails(c, controller, nil, "cannot find primary interface for container")
}

func makeFakeSubnet(id int) fakeSubnet {
	return fakeSubnet{
		id:      id,
		space:   "freckles",
		gateway: fmt.Sprintf("10.20.%d.2", 16+id),
		cidr:    fmt.Sprintf("10.20.%d.0/24", 16+id),
	}
}

func (suite *maasEnvironSuite) TestAllocateContainerAddressesMachinesError(c *gc.C) {
	var env *maasEnviron
	subnet := makeFakeSubnet(3)
	checkMachinesArgs := func(args gomaasapi.MachinesArgs) {
		expected := gomaasapi.MachinesArgs{
			AgentName: env.Config().UUID(),
			SystemIDs: []string{"1"},
		}
		c.Assert(args, jc.DeepEquals, expected)
	}
	controller := &fakeController{
		machinesError:     errors.New("boom"),
		machinesArgsCheck: checkMachinesArgs,
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "freckles",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet},
			},
		},
	}
	suite.injectController(controller)
	env = suite.makeEnviron(c, nil)
	prepared := network.InterfaceInfos{{
		InterfaceName: "eth0",
		Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.19.0/24")).AsProviderAddress()},
	}}
	ignored := names.NewMachineTag("1/lxd/0")
	_, err := env.AllocateContainerAddresses(suite.callCtx, instance.Id("1"), ignored, prepared)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func getArgs(c *gc.C, calls []testing.StubCall, callNum, argNum int) interface{} {
	c.Assert(len(calls), gc.Not(jc.LessThan), callNum)
	args := calls[callNum].Args
	c.Assert(len(args), gc.Not(jc.LessThan), argNum)
	return args[argNum]
}

func (suite *maasEnvironSuite) TestAllocateContainerAddressesCreateDeviceError(c *gc.C) {
	subnet := makeFakeSubnet(3)
	var env *maasEnviron
	machine := &fakeMachine{
		Stub:     &testing.Stub{},
		systemID: "1",
	}
	machine.SetErrors(nil, errors.New("bad device call"))
	controller := &fakeController{
		machines: []gomaasapi.Machine{machine},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "freckles",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet},
			},
		},
	}
	suite.injectController(controller)
	env = suite.makeEnviron(c, nil)
	prepared := network.InterfaceInfos{{
		InterfaceName: "eth0",
		Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.19.0/24")).AsProviderAddress()},
		MACAddress:    "DEADBEEF",
	}}
	ignored := names.NewMachineTag("1/lxd/0")
	_, err := env.AllocateContainerAddresses(suite.callCtx, instance.Id("1"), ignored, prepared)
	c.Assert(err, gc.ErrorMatches, `failed to create MAAS device for "juju-06f00d-1-lxd-0": bad device call`)
	machine.CheckCall(c, 0, "Devices", gomaasapi.DevicesArgs{
		Hostname: []string{"juju-06f00d-1-lxd-0"},
	})
	machine.CheckCall(c, 1, "CreateDevice", gomaasapi.CreateMachineDeviceArgs{
		Hostname:      "juju-06f00d-1-lxd-0",
		Subnet:        subnet,
		MACAddress:    "DEADBEEF",
		InterfaceName: "eth0",
	})
}

func (suite *maasEnvironSuite) TestAllocateContainerAddressesSubnetMissing(c *gc.C) {
	subnet := makeFakeSubnet(3)
	var env *maasEnviron
	device := &fakeDevice{
		Stub: &testing.Stub{},
		interfaceSet: []gomaasapi.Interface{
			&fakeInterface{
				id:         93,
				name:       "eth0",
				type_:      "physical",
				enabled:    true,
				macAddress: "53:54:00:70:9b:ff",
				vlan:       &fakeVLAN{vid: 0},
				links: []gomaasapi.Link{
					&fakeLink{
						id:   480,
						mode: "link_up",
					},
				},
				parents:  []string{},
				children: []string{},
				Stub:     &testing.Stub{},
			},
		},
		interface_: &fakeInterface{
			id:         94,
			name:       "eth1",
			type_:      "physical",
			enabled:    true,
			macAddress: "53:54:00:70:9b:f1",
			vlan:       &fakeVLAN{vid: 0},
			links: []gomaasapi.Link{
				&fakeLink{
					id:   481,
					mode: "link_up",
				},
			},
			parents:  []string{},
			children: []string{},
			Stub:     &testing.Stub{},
		},
		systemID: "foo",
	}
	machine := &fakeMachine{
		Stub:         &testing.Stub{},
		systemID:     "1",
		createDevice: device,
	}
	controller := &fakeController{
		Stub:     &testing.Stub{},
		machines: []gomaasapi.Machine{machine},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "freckles",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet},
			},
		},
		devices: []gomaasapi.Device{device},
	}
	suite.injectController(controller)
	env = suite.makeEnviron(c, nil)
	prepared := network.InterfaceInfos{
		{InterfaceName: "eth0", MACAddress: "DEADBEEF"},
		{InterfaceName: "eth1", MACAddress: "DEADBEEE"},
	}
	ignored := names.NewMachineTag("1/lxd/0")
	allocated, err := env.AllocateContainerAddresses(suite.callCtx, "1", ignored, prepared)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allocated, jc.DeepEquals, network.InterfaceInfos{{
		DeviceIndex:    0,
		MACAddress:     "53:54:00:70:9b:ff",
		ProviderId:     "93",
		ProviderVLANId: "0",
		VLANTag:        0,
		InterfaceName:  "eth0",
		InterfaceType:  "ethernet",
		Disabled:       false,
		NoAutoStart:    false,
		MTU:            1500,
		Origin:         network.OriginProvider,
	}, {
		DeviceIndex:    1,
		MACAddress:     "53:54:00:70:9b:f1",
		ProviderId:     "94",
		ProviderVLANId: "0",
		VLANTag:        0,
		InterfaceName:  "eth1",
		InterfaceType:  "ethernet",
		Disabled:       false,
		NoAutoStart:    false,
		MTU:            1500,
		Origin:         network.OriginProvider,
	}})
}

func (suite *maasEnvironSuite) TestAllocateContainerAddressesCreateInterfaceError(c *gc.C) {
	subnet := makeFakeSubnet(3)
	subnet2 := makeFakeSubnet(4)
	subnet2.vlan = fakeVLAN{vid: 66}
	var env *maasEnviron
	device := &fakeDevice{
		Stub:         &testing.Stub{},
		interfaceSet: []gomaasapi.Interface{&fakeInterface{}},
		systemID:     "foo",
	}
	device.SetErrors(errors.New("boom"))
	machine := &fakeMachine{
		Stub:         &testing.Stub{},
		systemID:     "1",
		createDevice: device,
	}
	controller := &fakeController{
		machines: []gomaasapi.Machine{machine},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "freckles",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet, subnet2},
			},
		},
	}
	suite.injectController(controller)
	env = suite.makeEnviron(c, nil)
	prepared := network.InterfaceInfos{
		{
			InterfaceName: "eth0",
			Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.19.0/24")).AsProviderAddress()},
			MACAddress:    "DEADBEEF",
		},
		{
			InterfaceName: "eth1",
			Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.20.0/24")).AsProviderAddress()},
			MACAddress:    "DEADBEEE",
		},
	}
	ignored := names.NewMachineTag("1/lxd/0")
	_, err := env.AllocateContainerAddresses(suite.callCtx, instance.Id("1"), ignored, prepared)
	c.Assert(err, gc.ErrorMatches, `failed to create MAAS device for "juju-06f00d-1-lxd-0": creating device interface: boom`)
	args := getArgs(c, device.Calls(), 0, 0)
	maasArgs, ok := args.(gomaasapi.CreateInterfaceArgs)
	c.Assert(ok, jc.IsTrue)
	expected := gomaasapi.CreateInterfaceArgs{
		MACAddress: "DEADBEEE",
		Name:       "eth1",
		VLAN:       subnet2.VLAN(),
	}
	c.Assert(maasArgs, jc.DeepEquals, expected)
}

func (suite *maasEnvironSuite) TestAllocateContainerAddressesLinkSubnetError(c *gc.C) {
	subnet := makeFakeSubnet(3)
	subnet2 := makeFakeSubnet(4)
	subnet2.vlan = fakeVLAN{vid: 66}
	var env *maasEnviron
	interface_ := &fakeInterface{Stub: &testing.Stub{}}
	interface_.SetErrors(errors.New("boom"))
	device := &fakeDevice{
		Stub:         &testing.Stub{},
		interfaceSet: []gomaasapi.Interface{&fakeInterface{}},
		interface_:   interface_,
		systemID:     "foo",
	}
	machine := &fakeMachine{
		Stub:         &testing.Stub{},
		systemID:     "1",
		createDevice: device,
	}
	controller := &fakeController{
		Stub:     &testing.Stub{},
		machines: []gomaasapi.Machine{machine},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "freckles",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet, subnet2},
			},
		},
		devices: []gomaasapi.Device{device},
	}
	suite.injectController(controller)
	env = suite.makeEnviron(c, nil)
	prepared := network.InterfaceInfos{
		{
			InterfaceName: "eth0",
			Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.19.0/24")).AsProviderAddress()},
			MACAddress:    "DEADBEEF",
		},
		{
			InterfaceName: "eth1",
			Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.20.0/24")).AsProviderAddress()},
			MACAddress:    "DEADBEEE",
		},
	}
	ignored := names.NewMachineTag("1/lxd/0")
	_, err := env.AllocateContainerAddresses(suite.callCtx, "1", ignored, prepared)
	c.Assert(err, gc.ErrorMatches, "failed to create MAAS device.*boom")
	args := getArgs(c, interface_.Calls(), 0, 0)
	maasArgs, ok := args.(gomaasapi.LinkSubnetArgs)
	c.Assert(ok, jc.IsTrue)
	expected := gomaasapi.LinkSubnetArgs{
		Mode:   gomaasapi.LinkModeStatic,
		Subnet: subnet2,
	}
	c.Assert(maasArgs, jc.DeepEquals, expected)
}

func (suite *maasEnvironSuite) TestStorageReturnsStorage(c *gc.C) {
	controller := newFakeController()
	env := suite.makeEnviron(c, controller)
	stor := env.Storage()
	c.Check(stor, gc.NotNil)

	// The Storage object is really a maasStorage.
	specificStorage := stor.(*maasStorage)

	// Its environment pointer refers back to its environment.
	c.Check(specificStorage.environ, gc.Equals, env)
	c.Check(specificStorage.maasController, gc.Equals, controller)
}

func (suite *maasEnvironSuite) TestAllocateContainerReuseExistingDevice(c *gc.C) {
	stub := &testing.Stub{}
	vlan1 := fakeVLAN{
		id:  5001,
		mtu: 1500,
	}
	subnet1 := fakeSubnet{
		id:         3,
		space:      "space-1",
		vlan:       vlan1,
		gateway:    "10.20.19.2",
		cidr:       "10.20.19.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}
	interfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         91,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan1,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        436,
					subnet:    &subnet1,
					ipAddress: "10.20.19.103",
					mode:      "static",
				},
			},
			parents: []string{},
		},
	}
	deviceInterfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         93,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "53:54:00:70:9b:ff",
			vlan:       vlan1,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        480,
					subnet:    &subnet1,
					ipAddress: "10.20.19.105",
					mode:      "static",
				},
			},
			parents: []string{},
		},
	}
	var env *maasEnviron
	device := &fakeDevice{
		Stub:         stub,
		interfaceSet: deviceInterfaces,
		systemID:     "foo",
	}
	controller := &fakeController{
		Stub: stub,
		machines: []gomaasapi.Machine{&fakeMachine{
			Stub:         stub,
			systemID:     "1",
			architecture: arch.HostArch(),
			interfaceSet: interfaces,
			// Instead of having createDevice return it, Devices()
			// returns it from the beginning
			createDevice: nil,
			devices:      []gomaasapi.Device{device},
		}},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "space-1",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet1},
			},
		},
		devices: []gomaasapi.Device{device},
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)

	prepared := network.InterfaceInfos{{
		MACAddress:    "53:54:00:70:9b:ff",
		Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.19.0/24")).AsProviderAddress()},
		InterfaceName: "eth0",
	}}
	containerTag := names.NewMachineTag("1/lxd/0")
	result, err := env.AllocateContainerAddresses(suite.callCtx, "1", containerTag, prepared)
	c.Assert(err, jc.ErrorIsNil)
	expected := network.InterfaceInfos{{
		DeviceIndex:       0,
		MACAddress:        "53:54:00:70:9b:ff",
		ProviderId:        "93",
		ProviderSubnetId:  "3",
		VLANTag:           0,
		ProviderVLANId:    "5001",
		ProviderAddressId: "480",
		InterfaceName:     "eth0",
		InterfaceType:     "ethernet",
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.20.19.105", network.WithCIDR("10.20.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("space-1")),
		},
		DNSServers: network.NewMachineAddresses([]string{
			"10.20.19.2",
			"10.20.19.3",
		}).AsProviderAddresses(network.WithSpaceName("space-1")),
		MTU:            1500,
		GatewayAddress: network.NewMachineAddress("10.20.19.2").AsProviderAddress(network.WithSpaceName("space-1")),
		Routes:         []network.Route{},
		Origin:         network.OriginProvider,
	}}
	c.Assert(result, jc.DeepEquals, expected)
}

func (suite *maasEnvironSuite) TestAllocateContainerRefusesReuseInvalidNIC(c *gc.C) {
	vlan1 := fakeVLAN{
		id:  5001,
		mtu: 1500,
	}
	vlan2 := fakeVLAN{
		id:  5002,
		mtu: 1500,
	}
	subnet1 := fakeSubnet{
		id:         3,
		space:      "freckles",
		vlan:       vlan1,
		gateway:    "10.20.19.2",
		cidr:       "10.20.19.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}
	subnet2 := fakeSubnet{
		id:         4,
		space:      "freckles",
		vlan:       vlan2,
		gateway:    "192.168.1.1",
		cidr:       "192.168.1.0/24",
		dnsServers: []string{"192.168.1.2"},
	}
	subnet3 := fakeSubnet{
		id:         5,
		space:      "freckles",
		vlan:       vlan2,
		gateway:    "192.168.1.1",
		cidr:       "192.168.2.0/24",
		dnsServers: []string{"192.168.1.2"},
	}
	interfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         91,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan1,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        436,
					subnet:    &subnet1,
					ipAddress: "10.20.19.103",
					mode:      "static",
				},
			},
			parents: []string{},
		},
		&fakeInterface{
			id:         92,
			name:       "eth1",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:ff",
			vlan:       vlan2,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        437,
					subnet:    &subnet2,
					ipAddress: "192.168.1.100",
					mode:      "static",
				},
			},
			parents: []string{},
		},
	}
	badDeviceInterfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         93,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "53:54:00:70:88:aa",
			vlan:       vlan1,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        480,
					subnet:    &subnet1,
					ipAddress: "10.20.19.105",
					mode:      "static",
				},
			},
			parents: []string{},
		},
		// This interface is linked to the wrong subnet
		&fakeInterface{
			id:         94,
			name:       "eth1",
			type_:      "physical",
			enabled:    true,
			macAddress: "53:54:00:70:88:bb",
			vlan:       vlan1,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        481,
					subnet:    &subnet3,
					ipAddress: "192.168.2.100",
					mode:      "static",
				},
			},
			parents: []string{},
		},
	}
	goodSecondInterface := &fakeInterface{
		id:         94,
		name:       "eth1",
		type_:      "physical",
		enabled:    true,
		macAddress: "53:54:00:70:88:bb",
		vlan:       vlan2,
		links: []gomaasapi.Link{
			&fakeLink{
				id:        481,
				subnet:    &subnet2,
				ipAddress: "192.168.1.101",
				mode:      "static",
			},
		},
		parents: []string{},
	}
	goodDeviceInterfaces := []gomaasapi.Interface{
		badDeviceInterfaces[0],
	}
	var env *maasEnviron
	stub := &testing.Stub{}
	badDevice := &fakeDevice{
		Stub:         stub,
		interfaceSet: badDeviceInterfaces,
		systemID:     "foo",
	}
	goodDevice := &fakeDevice{
		Stub:         stub,
		interfaceSet: goodDeviceInterfaces,
		systemID:     "foo",
		interface_:   goodSecondInterface,
	}
	machine := &fakeMachine{
		Stub:         stub,
		systemID:     "1",
		architecture: arch.HostArch(),
		interfaceSet: interfaces,
		createDevice: goodDevice,
		// Devices will first list the bad device, and then
		// createDevice will create the right one
		devices: []gomaasapi.Device{badDevice},
	}
	badDevice.deleteCB = func() { machine.devices = machine.devices[:0] }
	controller := &fakeController{
		Stub:     stub,
		machines: []gomaasapi.Machine{machine},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "space-1",
				id:      4567,
				subnets: []gomaasapi.Subnet{subnet1},
			},
		},
		devices: []gomaasapi.Device{goodDevice},
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)

	prepared := network.InterfaceInfos{{
		MACAddress:    "53:54:00:70:88:aa",
		Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("10.20.19.0/24")).AsProviderAddress()},
		InterfaceName: "eth0",
	}, {
		MACAddress:    "53:54:00:70:88:bb",
		Addresses:     network.ProviderAddresses{network.NewMachineAddress("", network.WithCIDR("192.168.1.0/24")).AsProviderAddress()},
		InterfaceName: "eth1",
	}}
	containerTag := names.NewMachineTag("1/lxd/0")
	result, err := env.AllocateContainerAddresses(suite.callCtx, instance.Id("1"), containerTag, prepared)
	c.Assert(err, jc.ErrorIsNil)
	expected := network.InterfaceInfos{{
		DeviceIndex:       0,
		MACAddress:        "53:54:00:70:88:aa",
		ProviderId:        "93",
		ProviderSubnetId:  "3",
		VLANTag:           0,
		ProviderVLANId:    "5001",
		ProviderAddressId: "480",
		InterfaceName:     "eth0",
		InterfaceType:     "ethernet",
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.20.19.105", network.WithCIDR("10.20.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("freckles")),
		},
		DNSServers: network.NewMachineAddresses([]string{
			"10.20.19.2",
			"10.20.19.3",
		}).AsProviderAddresses(network.WithSpaceName("freckles")),
		MTU:            1500,
		GatewayAddress: network.NewMachineAddress("10.20.19.2").AsProviderAddress(network.WithSpaceName("freckles")),
		Routes:         []network.Route{},
		Origin:         network.OriginProvider,
	}, {
		DeviceIndex:       1,
		MACAddress:        "53:54:00:70:88:bb",
		ProviderId:        "94",
		ProviderSubnetId:  "4",
		VLANTag:           0,
		ProviderVLANId:    "5002",
		ProviderAddressId: "481",
		InterfaceName:     "eth1",
		InterfaceType:     "ethernet",
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"192.168.1.101", network.WithCIDR("192.168.1.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("freckles")),
		},
		DNSServers: network.NewMachineAddresses([]string{
			"192.168.1.2",
		}).AsProviderAddresses(network.WithSpaceName("freckles")),
		MTU:            1500,
		GatewayAddress: network.NewMachineAddress("192.168.1.1").AsProviderAddress(network.WithSpaceName("freckles")),
		Routes:         []network.Route{},
		Origin:         network.OriginProvider,
	}}
	c.Assert(result, jc.DeepEquals, expected)
}

func (suite *maasEnvironSuite) TestStartInstanceEndToEnd(c *gc.C) {
	suite.setupFakeTools(c)
	machine := newFakeMachine("gus", arch.HostArch(), "Deployed")
	file := &fakeFile{name: coretesting.ModelTag.Id() + "-provider-state"}
	controller := newFakeControllerWithFiles(file)
	controller.machines = []gomaasapi.Machine{machine}
	controller.allocateMachine = machine
	controller.allocateMachineMatches = gomaasapi.ConstraintMatches{
		Storage: make(map[string][]gomaasapi.StorageDevice),
	}

	env := suite.makeEnviron(c, controller)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		suite.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             jujutesting.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
			DialOpts: environs.BootstrapDialOpts{
				Timeout: coretesting.LongWait,
			},
		})
	c.Assert(err, jc.ErrorIsNil)

	machine.Stub.CheckCallNames(c, "Start", "SetOwnerData")
	ownerData, ok := machine.Stub.Calls()[1].Args[0].(map[string]string)
	c.Assert(ok, jc.IsTrue)
	c.Assert(ownerData, gc.DeepEquals, map[string]string{
		"claude":              "rains",
		tags.JujuController:   suite.controllerUUID,
		tags.JujuIsController: "true",
		tags.JujuModel:        env.Config().UUID(),
	})

	// Test the instance id is correctly recorded for the bootstrap node.
	// Check that ControllerInstances returns the id of the bootstrap machine.
	instanceIds, err := env.ControllerInstances(suite.callCtx, suite.controllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceIds, gc.HasLen, 1)
	insts, err := env.AllRunningInstances(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(insts[0].Id(), gc.Equals, instanceIds[0])

	node1 := newFakeMachine("victor", arch.HostArch(), "Deployed")
	node1.hostname = "host1"
	node1.cpuCount = 1
	node1.memory = 1024
	node1.zoneName = "test_zone"
	controller.allocateMachine = node1

	instance, hc := jujutesting.AssertStartInstanceWithConstraints(
		c,
		env,
		suite.callCtx,
		suite.controllerUUID,
		"1",
		constraints.Value{
			ImageID: stringp("ubuntu-bf2"),
		})
	c.Check(instance, gc.NotNil)
	c.Assert(hc, gc.NotNil)
	c.Check(hc.String(), gc.Equals, fmt.Sprintf("arch=%s cores=1 mem=1024M availability-zone=test_zone", arch.HostArch()))

	node1.Stub.CheckCallNames(c, "Start", "SetOwnerData")
	startArgs, ok := node1.Stub.Calls()[0].Args[0].(gomaasapi.StartArgs)
	c.Assert(ok, jc.IsTrue)

	c.Assert(startArgs.DistroSeries, gc.Equals, "ubuntu-bf2")

	decodedUserData, err := decodeUserData(startArgs.UserData)
	c.Assert(err, jc.ErrorIsNil)
	info := machineInfo{"host1"}
	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	cloudinitRunCmd, err := info.cloudinitRunCmd(cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	data, err := goyaml.Marshal(cloudinitRunCmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(decodedUserData), jc.Contains, string(data))

	// Trash the tools and try to start another instance.
	suite.PatchValue(&envtools.DefaultBaseURL, "")
	instance, _, _, err = jujutesting.StartInstance(env, suite.callCtx, suite.controllerUUID, "2")
	c.Check(instance, gc.IsNil)
	c.Check(err, jc.ErrorIs, errors.NotFound)
}

func (suite *maasEnvironSuite) TestControllerInstances(c *gc.C) {
	controller := newFakeControllerWithErrors(gomaasapi.NewNoMatchError("state"))
	env := suite.makeEnviron(c, controller)
	_, err := env.ControllerInstances(suite.callCtx, suite.controllerUUID)
	c.Assert(err, gc.Equals, environs.ErrNotBootstrapped)

	controller.machinesArgsCheck = func(args gomaasapi.MachinesArgs) {
		c.Assert(args, gc.DeepEquals, gomaasapi.MachinesArgs{
			OwnerData: map[string]string{
				tags.JujuIsController: "true",
				tags.JujuController:   suite.controllerUUID,
			},
		})
	}

	tests := [][]instance.Id{{"inst-0"}, {"inst-0", "inst-1"}}
	for _, expected := range tests {
		controller.machines = make([]gomaasapi.Machine, len(expected))
		for i := range expected {
			controller.machines[i] = newFakeMachine(string(expected[i]), "", "")
		}
		controllerInstances, err := env.ControllerInstances(suite.callCtx, suite.controllerUUID)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(controllerInstances, jc.SameContents, expected)
	}
}

func (suite *maasEnvironSuite) TestControllerInstancesInvalidCredential(c *gc.C) {
	controller := &fakeController{
		machinesError: gomaasapi.NewPermissionError("fail auth here"),
	}
	env := suite.makeEnviron(c, controller)

	c.Assert(suite.invalidCredential, jc.IsFalse)
	_, err := env.ControllerInstances(suite.callCtx, suite.controllerUUID)
	c.Assert(err, gc.NotNil)
	c.Assert(suite.invalidCredential, jc.IsTrue)
}

func (suite *maasEnvironSuite) TestDestroy(c *gc.C) {
	file1 := &fakeFile{name: coretesting.ModelTag.Id() + "-provider-state"}
	file2 := &fakeFile{name: coretesting.ModelTag.Id() + "-horace"}
	controller := newFakeControllerWithFiles(file1, file2)
	controller.machines = []gomaasapi.Machine{&fakeMachine{systemID: "pete"}}
	env := suite.makeEnviron(c, controller)
	err := env.Destroy(suite.callCtx)
	c.Check(err, jc.ErrorIsNil)

	controller.Stub.CheckCallNames(c, "ReleaseMachines", "GetFile", "Files", "GetFile", "GetFile")
	// Instances have been stopped.
	controller.Stub.CheckCall(c, 0, "ReleaseMachines", gomaasapi.ReleaseMachinesArgs{
		SystemIDs: []string{"pete"},
		Comment:   "Released by Juju MAAS provider",
	})

	// Files have been cleaned up.
	c.Check(file1.deleted, jc.IsTrue)
	c.Check(file2.deleted, jc.IsTrue)
}

func (suite *maasEnvironSuite) TestBootstrapFailsIfNoTools(c *gc.C) {
	env := suite.makeEnviron(c, newFakeController())
	vers := version.MustParse("1.2.3")
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		suite.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      jujutesting.AdminSecret,
			CAPrivateKey:     coretesting.CAKey,
			// Disable auto-uploading by setting the agent version
			// to something that's not the current version.
			AgentVersion:            &vers,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
			DialOpts: environs.BootstrapDialOpts{
				Timeout: coretesting.LongWait,
			},
		})
	c.Check(err, gc.ErrorMatches, "Juju cannot bootstrap because no agent binaries are available for your model(.|\n)*")
}

func (suite *maasEnvironSuite) TestBootstrapFailsIfNoNodes(c *gc.C) {
	suite.setupFakeTools(c)
	controller := newFakeController()
	controller.allocateMachineError = gomaasapi.NewNoMatchError("oops")
	env := suite.makeEnviron(c, controller)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		suite.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             jujutesting.AdminSecret,
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
			DialOpts: environs.BootstrapDialOpts{
				Timeout: coretesting.LongWait,
			},
		})
	// Since there are no nodes, the attempt to allocate one returns a
	// 409: Conflict.
	c.Check(err, gc.ErrorMatches, "(?ms)cannot start bootstrap instance in any availability zone \\(mossack, fonseca\\).*")
}

func (suite *maasEnvironSuite) TestGetToolsMetadataSources(c *gc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	// Add a dummy file to storage so we can use that to check the
	// obtained source later.
	env := suite.makeEnviron(c, newFakeControllerWithFiles(
		&fakeFile{name: coretesting.ModelTag.Id() + "-tools/filename", contents: makeRandomBytes(10)},
	))
	sources, err := envtools.GetMetadataSources(env, ss)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 0)
}

func (suite *maasEnvironSuite) TestConstraintsValidator(c *gc.C) {
	controller := newFakeController()
	controller.bootResources = []gomaasapi.BootResource{&fakeBootResource{name: "jammy", architecture: "amd64"}}
	env := suite.makeEnviron(c, controller)
	validator, err := env.ConstraintsValidator(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 cpu-power=10 instance-type=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power", "instance-type", "virt-type"})
}

func (suite *maasEnvironSuite) TestConstraintsValidatorWithUnsupportedArch(c *gc.C) {
	controller := newFakeController()
	controller.bootResources = []gomaasapi.BootResource{
		&fakeBootResource{name: "jammy", architecture: "i386"},
		&fakeBootResource{name: "jammy", architecture: "amd64"},
	}
	env := suite.makeEnviron(c, controller)
	validator, err := env.ConstraintsValidator(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 cpu-power=10 instance-type=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power", "instance-type", "virt-type"})
}

func (suite *maasEnvironSuite) TestConstraintsValidatorInvalidCredential(c *gc.C) {
	controller := &fakeController{
		bootResources:      []gomaasapi.BootResource{&fakeBootResource{name: "jammy", architecture: "amd64"}},
		bootResourcesError: gomaasapi.NewPermissionError("fail auth here"),
	}
	env := suite.makeEnviron(c, controller)
	c.Assert(suite.invalidCredential, jc.IsFalse)
	_, err := env.ConstraintsValidator(suite.callCtx)
	c.Assert(err, gc.NotNil)
	c.Assert(suite.invalidCredential, jc.IsTrue)
}

func (suite *maasEnvironSuite) TestDomainsInvalidCredential(c *gc.C) {
	controller := &fakeController{
		domainsError: gomaasapi.NewPermissionError("fail auth here"),
	}
	env := suite.makeEnviron(c, controller)
	c.Assert(suite.invalidCredential, jc.IsFalse)
	_, err := env.Domains(suite.callCtx)
	c.Assert(err, gc.NotNil)
	c.Assert(suite.invalidCredential, jc.IsTrue)
}

func (suite *maasEnvironSuite) TestConstraintsValidatorVocab(c *gc.C) {
	controller := newFakeController()
	controller.bootResources = []gomaasapi.BootResource{
		&fakeBootResource{name: "jammy", architecture: "amd64"},
		&fakeBootResource{name: "focal", architecture: "arm64"},
	}
	env := suite.makeEnviron(c, controller)
	validator, err := env.ConstraintsValidator(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are: amd64 arm64")
}

func (suite *maasEnvironSuite) TestReleaseContainerAddresses(c *gc.C) {
	dev1 := newFakeDevice("a", "eleven")
	dev2 := newFakeDevice("b", "will")
	controller := newFakeController()
	controller.devices = []gomaasapi.Device{dev1, dev2}

	env := suite.makeEnviron(c, controller)
	err := env.ReleaseContainerAddresses(suite.callCtx, []network.ProviderInterfaceInfo{
		{HardwareAddress: "will"},
		{HardwareAddress: "dustin"},
		{HardwareAddress: "eleven"},
	})
	c.Assert(err, jc.ErrorIsNil)

	args, ok := getArgs(c, controller.Calls(), 0, 0).(gomaasapi.DevicesArgs)
	c.Assert(ok, jc.IsTrue)
	expected := gomaasapi.DevicesArgs{MACAddresses: []string{"will", "dustin", "eleven"}}
	c.Assert(args, gc.DeepEquals, expected)

	dev1.CheckCallNames(c, "Delete")
	dev2.CheckCallNames(c, "Delete")
}

func (suite *maasEnvironSuite) TestReleaseContainerAddresses_HandlesDupes(c *gc.C) {
	dev1 := newFakeDevice("a", "eleven")
	controller := newFakeController()
	controller.devices = []gomaasapi.Device{dev1, dev1}

	env := suite.makeEnviron(c, controller)
	err := env.ReleaseContainerAddresses(suite.callCtx, []network.ProviderInterfaceInfo{
		{HardwareAddress: "will"},
		{HardwareAddress: "eleven"},
	})
	c.Assert(err, jc.ErrorIsNil)

	args, ok := getArgs(c, controller.Calls(), 0, 0).(gomaasapi.DevicesArgs)
	c.Assert(ok, jc.IsTrue)
	expected := gomaasapi.DevicesArgs{MACAddresses: []string{"will", "eleven"}}
	c.Assert(args, gc.DeepEquals, expected)

	dev1.CheckCallNames(c, "Delete")
}

func (suite *maasEnvironSuite) TestReleaseContainerAddressesErrorGettingDevices(c *gc.C) {
	controller := newFakeControllerWithErrors(errors.New("Everything done broke"))
	env := suite.makeEnviron(c, controller)
	err := env.ReleaseContainerAddresses(suite.callCtx, []network.ProviderInterfaceInfo{{HardwareAddress: "anything"}})
	c.Assert(err, gc.ErrorMatches, "Everything done broke")
}

func (suite *maasEnvironSuite) TestReleaseContainerAddressesErrorDeletingDevice(c *gc.C) {
	dev1 := newFakeDevice("a", "eleven")
	dev1.systemID = "hopper"
	dev1.SetErrors(errors.New("don't delete me"))
	controller := newFakeController()
	controller.devices = []gomaasapi.Device{dev1}

	env := suite.makeEnviron(c, controller)
	err := env.ReleaseContainerAddresses(suite.callCtx, []network.ProviderInterfaceInfo{
		{HardwareAddress: "eleven"},
	})
	c.Assert(err, gc.ErrorMatches, "deleting device hopper: don't delete me")

	_, ok := getArgs(c, controller.Calls(), 0, 0).(gomaasapi.DevicesArgs)
	c.Assert(ok, jc.IsTrue)

	dev1.CheckCallNames(c, "Delete")
}

func (suite *maasEnvironSuite) TestAdoptResources(c *gc.C) {
	machine1 := newFakeMachine("big-fig-wasp", "gaudi", "good")
	machine2 := newFakeMachine("robot-stop", "hundertwasser", "fine")
	machine3 := newFakeMachine("gamma-knife", "von-neumann", "acceptable")
	controller := newFakeController()
	controller.machines = append(controller.machines, machine1, machine3)
	env := suite.makeEnviron(c, controller)

	err := env.AdoptResources(suite.callCtx, "some-other-controller", version.MustParse("1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	machine1.CheckCallNames(c, "SetOwnerData")
	c.Assert(machine1.Calls()[0].Args[0], gc.DeepEquals, map[string]string{
		tags.JujuController: "some-other-controller",
	})
	machine2.CheckCallNames(c)
	machine3.CheckCallNames(c, "SetOwnerData")
	c.Assert(machine3.Calls()[0].Args[0], gc.DeepEquals, map[string]string{
		tags.JujuController: "some-other-controller",
	})
}

func (suite *maasEnvironSuite) TestAdoptResourcesError(c *gc.C) {
	machine1 := newFakeMachine("evil-death-roll", "frank-lloyd-wright", "ok")
	machine2 := newFakeMachine("people-vultures", "gehry", "adequate")
	controller := newFakeController()
	controller.machines = append(controller.machines, machine1, machine2)
	env := suite.makeEnviron(c, controller)

	machine1.SetErrors(errors.New("blorp"))

	err := env.AdoptResources(suite.callCtx, "some-other-controller", version.MustParse("3.2.1"))
	c.Assert(err, gc.ErrorMatches, `failed to update controller for some instances: \[evil-death-roll\]`)

	machine1.CheckCallNames(c, "SetOwnerData")
	c.Assert(machine1.Calls()[0].Args[0], gc.DeepEquals, map[string]string{
		tags.JujuController: "some-other-controller",
	})
	machine2.CheckCallNames(c, "SetOwnerData")
	c.Assert(machine2.Calls()[0].Args[0], gc.DeepEquals, map[string]string{
		tags.JujuController: "some-other-controller",
	})
}

func newFakeDevice(systemID, macAddress string) *fakeDevice {
	return &fakeDevice{
		Stub:     &testing.Stub{},
		systemID: systemID,
		interface_: &fakeInterface{
			Stub:       &testing.Stub{},
			macAddress: macAddress,
		},
	}
}

// makeRandomBytes returns an array of arbitrary byte values.
func makeRandomBytes(length int) []byte {
	data := make([]byte, length)
	for index := range data {
		data[index] = byte(rand.Intn(256))
	}
	return data
}

func decodeUserData(userData string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(userData)
	if err != nil {
		return []byte(""), err
	}
	return utils.Gunzip(data)
}
