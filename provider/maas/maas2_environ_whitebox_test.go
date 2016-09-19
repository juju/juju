// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	envjujutesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
)

type maas2EnvironSuite struct {
	maas2Suite
}

var _ = gc.Suite(&maas2EnvironSuite{})

func (suite *maas2EnvironSuite) getEnvWithServer(c *gc.C) (*maasEnviron, error) {
	testServer := gomaasapi.NewSimpleServer()
	testServer.AddGetResponse("/api/2.0/version/", http.StatusOK, maas2VersionResponse)
	testServer.AddGetResponse("/api/2.0/users/?op=whoami", http.StatusOK, "{}")
	// Weirdly, rather than returning a 404 when the version is
	// unknown, MAAS2 returns some HTML (the login page).
	testServer.AddGetResponse("/api/1.0/version/", http.StatusOK, "<html></html>")
	testServer.Start()
	suite.AddCleanup(func(*gc.C) { testServer.Close() })
	cred := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{
		"maas-oauth": "a:b:c",
	})
	cloud := environs.CloudSpec{
		Type:       "maas",
		Name:       "maas",
		Endpoint:   testServer.Server.URL,
		Credential: &cred,
	}
	attrs := coretesting.FakeConfig().Merge(maasEnvAttrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return NewEnviron(cloud, cfg)
}

func (suite *maas2EnvironSuite) TestNewEnvironWithController(c *gc.C) {
	env, err := suite.getEnvWithServer(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
}

func (suite *maas2EnvironSuite) injectControllerWithSpacesAndCheck(c *gc.C, spaces []gomaasapi.Space, expected gomaasapi.AllocateMachineArgs) (*maasEnviron, *fakeController) {
	var env *maasEnviron
	check := func(args gomaasapi.AllocateMachineArgs) {
		expected.AgentName = env.Config().UUID()
		c.Assert(args, gc.DeepEquals, expected)
	}
	controller := &fakeController{
		allocateMachineArgsCheck: check,
		allocateMachine:          newFakeMachine("Bruce Sterling", arch.HostArch(), ""),
		allocateMachineMatches: gomaasapi.ConstraintMatches{
			Storage: map[string][]gomaasapi.BlockDevice{},
		},
		spaces: spaces,
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)
	return env, controller
}

func (suite *maas2EnvironSuite) makeEnvironWithMachines(c *gc.C, expectedSystemIDs []string, returnSystemIDs []string) *maasEnviron {
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

func (suite *maas2EnvironSuite) TestAllInstances(c *gc.C) {
	env := suite.makeEnvironWithMachines(
		c, []string{}, []string{"tuco", "tio", "gus"},
	)
	result, err := env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	expectedMachines := set.NewStrings("tuco", "tio", "gus")
	actualMachines := set.NewStrings()
	for _, instance := range result {
		actualMachines.Add(string(instance.Id()))
	}
	c.Assert(actualMachines, gc.DeepEquals, expectedMachines)
}

func (suite *maas2EnvironSuite) TestAllInstancesError(c *gc.C) {
	controller := &fakeController{machinesError: errors.New("Something terrible!")}
	env := suite.makeEnviron(c, controller)
	_, err := env.AllInstances()
	c.Assert(err, gc.ErrorMatches, "Something terrible!")
}

func (suite *maas2EnvironSuite) TestInstances(c *gc.C) {
	env := suite.makeEnvironWithMachines(
		c, []string{"jake", "bonnibel"}, []string{"jake", "bonnibel"},
	)
	result, err := env.Instances([]instance.Id{"jake", "bonnibel"})
	c.Assert(err, jc.ErrorIsNil)
	expectedMachines := set.NewStrings("jake", "bonnibel")
	actualMachines := set.NewStrings()
	for _, machine := range result {
		actualMachines.Add(string(machine.Id()))
	}
	c.Assert(actualMachines, gc.DeepEquals, expectedMachines)
}

func (suite *maas2EnvironSuite) TestInstancesPartialResult(c *gc.C) {
	env := suite.makeEnvironWithMachines(
		c, []string{"jake", "bonnibel"}, []string{"tuco", "bonnibel"},
	)
	result, err := env.Instances([]instance.Id{"jake", "bonnibel"})
	c.Check(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(result, gc.HasLen, 2)
	c.Assert(result[0], gc.IsNil)
	c.Assert(result[1].Id(), gc.Equals, instance.Id("bonnibel"))
}

func (suite *maas2EnvironSuite) TestAvailabilityZones(c *gc.C) {
	controller := &fakeController{
		zones: []gomaasapi.Zone{
			&fakeZone{name: "mossack"},
			&fakeZone{name: "fonseca"},
		},
	}
	env := suite.makeEnviron(c, controller)
	result, err := env.AvailabilityZones()
	c.Assert(err, jc.ErrorIsNil)
	expectedZones := set.NewStrings("mossack", "fonseca")
	actualZones := set.NewStrings()
	for _, zone := range result {
		actualZones.Add(zone.Name())
	}
	c.Assert(actualZones, gc.DeepEquals, expectedZones)
}

func (suite *maas2EnvironSuite) TestAvailabilityZonesError(c *gc.C) {
	controller := &fakeController{
		zonesError: errors.New("a bad thing"),
	}
	env := suite.makeEnviron(c, controller)
	_, err := env.AvailabilityZones()
	c.Assert(err, gc.ErrorMatches, "a bad thing")
}

func (suite *maas2EnvironSuite) TestSpaces(c *gc.C) {
	controller := &fakeController{
		spaces: []gomaasapi.Space{
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
		},
	}
	env := suite.makeEnviron(c, controller)
	result, err := env.Spaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(result[0].Name, gc.Equals, "freckles")
	c.Assert(result[0].ProviderId, gc.Equals, network.Id("4567"))
	subnets := result[0].Subnets
	c.Assert(subnets, gc.HasLen, 2)
	c.Assert(subnets[0].ProviderId, gc.Equals, network.Id("99"))
	c.Assert(subnets[0].VLANTag, gc.Equals, 66)
	c.Assert(subnets[0].CIDR, gc.Equals, "192.168.10.0/24")
	c.Assert(subnets[0].SpaceProviderId, gc.Equals, network.Id("4567"))
	c.Assert(subnets[1].ProviderId, gc.Equals, network.Id("98"))
	c.Assert(subnets[1].VLANTag, gc.Equals, 67)
	c.Assert(subnets[1].CIDR, gc.Equals, "192.168.11.0/24")
	c.Assert(subnets[1].SpaceProviderId, gc.Equals, network.Id("4567"))
}

func (suite *maas2EnvironSuite) TestSpacesError(c *gc.C) {
	controller := &fakeController{
		spacesError: errors.New("Joe Manginiello"),
	}
	env := suite.makeEnviron(c, controller)
	_, err := env.Spaces()
	c.Assert(err, gc.ErrorMatches, "Joe Manginiello")
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

func (suite *maas2EnvironSuite) TestStopInstancesReturnsIfParameterEmpty(c *gc.C) {
	controller := newFakeController()
	err := suite.makeEnviron(c, controller).StopInstances()
	c.Check(err, jc.ErrorIsNil)
	c.Assert(collectReleaseArgs(controller), gc.HasLen, 0)
}

func (suite *maas2EnvironSuite) TestStopInstancesStopsAndReleasesInstances(c *gc.C) {
	// Return a cannot complete indicating that test1 is in the wrong state.
	// The release operation will still release the others and succeed.
	controller := newFakeControllerWithFiles(&fakeFile{name: coretesting.ModelTag.Id() + "-provider-state"})
	err := suite.makeEnviron(c, controller).StopInstances("test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)
	args := collectReleaseArgs(controller)
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0].SystemIDs, gc.DeepEquals, []string{"test1", "test2", "test3"})
}

func (suite *maas2EnvironSuite) TestStopInstancesIgnoresConflict(c *gc.C) {
	// Return a cannot complete indicating that test1 is in the wrong state.
	// The release operation will still release the others and succeed.
	controller := newFakeControllerWithFiles(&fakeFile{name: coretesting.ModelTag.Id() + "-provider-state"})
	controller.SetErrors(gomaasapi.NewCannotCompleteError("test1 not allocated"))
	err := suite.makeEnviron(c, controller).StopInstances("test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)

	args := collectReleaseArgs(controller)
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0].SystemIDs, gc.DeepEquals, []string{"test1", "test2", "test3"})
}

func (suite *maas2EnvironSuite) TestStopInstancesIgnoresMissingNodeAndRecurses(c *gc.C) {
	controller := newFakeControllerWithFiles(&fakeFile{name: coretesting.ModelTag.Id() + "-provider-state"})
	controller.SetErrors(
		gomaasapi.NewBadRequestError("no such machine: test1"),
		gomaasapi.NewBadRequestError("no such machine: test1"),
	)
	err := suite.makeEnviron(c, controller).StopInstances("test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)
	args := collectReleaseArgs(controller)
	c.Assert(args, gc.HasLen, 4)
	c.Assert(args[0].SystemIDs, gc.DeepEquals, []string{"test1", "test2", "test3"})
	c.Assert(args[1].SystemIDs, gc.DeepEquals, []string{"test1"})
	c.Assert(args[2].SystemIDs, gc.DeepEquals, []string{"test2"})
	c.Assert(args[3].SystemIDs, gc.DeepEquals, []string{"test3"})
}

func (suite *maas2EnvironSuite) checkStopInstancesFails(c *gc.C, withError error) {
	controller := newFakeControllerWithFiles(&fakeFile{name: coretesting.ModelTag.Id() + "-provider-state"})
	controller.SetErrors(withError)
	err := suite.makeEnviron(c, controller).StopInstances("test1", "test2", "test3")
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("cannot release nodes: %s", withError))
	// Only tries once.
	c.Assert(collectReleaseArgs(controller), gc.HasLen, 1)
}

func (suite *maas2EnvironSuite) TestStopInstancesReturnsUnexpectedMAASError(c *gc.C) {
	suite.checkStopInstancesFails(c, gomaasapi.NewNoMatchError("Something else bad!"))
}

func (suite *maas2EnvironSuite) TestStopInstancesReturnsUnexpectedError(c *gc.C) {
	suite.checkStopInstancesFails(c, errors.New("Something completely unexpected!"))
}

func (suite *maas2EnvironSuite) TestStartInstanceError(c *gc.C) {
	suite.injectController(&fakeController{
		allocateMachineError: errors.New("Charles Babbage"),
	})
	env := suite.makeEnviron(c, nil)
	_, err := env.StartInstance(environs.StartInstanceParams{})
	c.Assert(err, gc.ErrorMatches, ".* cannot run instance: Charles Babbage")
}

func (suite *maas2EnvironSuite) TestStartInstance(c *gc.C) {
	env, _ := suite.injectControllerWithSpacesAndCheck(c, nil, gomaasapi.AllocateMachineArgs{})

	params := environs.StartInstanceParams{ControllerUUID: suite.controllerUUID}
	result, err := jujutesting.StartInstanceWithParams(env, "1", params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Instance.Id(), gc.Equals, instance.Id("Bruce Sterling"))
}

func (suite *maas2EnvironSuite) TestStartInstanceAppliesResourceTags(c *gc.C) {
	env, controller := suite.injectControllerWithSpacesAndCheck(c, nil, gomaasapi.AllocateMachineArgs{})
	config := env.Config()
	_, ok := config.ResourceTags()
	c.Assert(ok, jc.IsTrue)
	params := environs.StartInstanceParams{ControllerUUID: suite.controllerUUID}
	_, err := jujutesting.StartInstanceWithParams(env, "1", params)
	c.Assert(err, jc.ErrorIsNil)

	machine := controller.allocateMachine.(*fakeMachine)
	machine.CheckCallNames(c, "Start", "SetOwnerData")
	c.Assert(machine.Calls()[1].Args[0], gc.DeepEquals, map[string]string{
		"claude":            "rains",
		tags.JujuController: suite.controllerUUID,
		tags.JujuModel:      config.UUID(),
	})
}

func (suite *maas2EnvironSuite) TestStartInstanceParams(c *gc.C) {
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
			Storage: map[string][]gomaasapi.BlockDevice{},
		},
		zones: []gomaasapi.Zone{&fakeZone{name: "foo"}},
	})
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)
	params := environs.StartInstanceParams{
		ControllerUUID: suite.controllerUUID,
		Placement:      "zone=foo",
		Constraints:    constraints.MustParse("mem=8G"),
	}
	result, err := jujutesting.StartInstanceWithParams(env, "1", params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Instance.Id(), gc.Equals, instance.Id("Bruce Sterling"))
}

func (suite *maas2EnvironSuite) TestAcquireNodePassedAgentName(c *gc.C) {
	var env *maasEnviron
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, gc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.Config().UUID()})
		},
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
	})
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)

	_, err := env.acquireNode2("", "", constraints.Value{}, nil, nil)

	c.Check(err, jc.ErrorIsNil)
}

func (suite *maas2EnvironSuite) TestAcquireNodePassesPositiveAndNegativeTags(c *gc.C) {
	var env *maasEnviron
	expected := gomaasapi.AllocateMachineArgs{
		Tags:    []string{"tag1", "tag3"},
		NotTags: []string{"tag2", "tag4"},
	}
	env, _ = suite.injectControllerWithSpacesAndCheck(c, nil, expected)
	_, err := env.acquireNode2(
		"", "",
		constraints.Value{Tags: stringslicep("tag1", "^tag2", "tag3", "^tag4")},
		nil, nil,
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

func (suite *maas2EnvironSuite) TestAcquireNodePassesPositiveAndNegativeSpaces(c *gc.C) {
	expected := gomaasapi.AllocateMachineArgs{
		NotSpace: []string{"6", "8"},
		Interfaces: []gomaasapi.InterfaceSpec{
			{Label: "0", Space: "5"},
			{Label: "1", Space: "7"},
		},
	}
	env, _ := suite.injectControllerWithSpacesAndCheck(c, getFourSpaces(), expected)

	_, err := env.acquireNode2(
		"", "",
		constraints.Value{Spaces: stringslicep("space-1", "^space-2", "space-3", "^space-4")},
		nil, nil,
	)
	c.Check(err, jc.ErrorIsNil)
}

func (suite *maas2EnvironSuite) TestAcquireNodeDisambiguatesNamedLabelsFromIndexedUpToALimit(c *gc.C) {
	env, _ := suite.injectControllerWithSpacesAndCheck(c, getFourSpaces(), gomaasapi.AllocateMachineArgs{})
	var shortLimit uint = 0
	suite.PatchValue(&numericLabelLimit, shortLimit)

	_, err := env.acquireNode2(
		"", "",
		constraints.Value{Spaces: stringslicep("space-1", "^space-2", "space-3", "^space-4")},
		[]interfaceBinding{{"0", "first-clash"}, {"1", "final-clash"}},
		nil,
	)
	c.Assert(err, gc.ErrorMatches, `too many conflicting numeric labels, giving up.`)
}

func (suite *maas2EnvironSuite) TestAcquireNodeStorage(c *gc.C) {
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
		_, err := env.acquireNode2("", "", constraints.Value{}, nil, test.volumes)
		c.Check(err, jc.ErrorIsNil)
	}
}

func (suite *maas2EnvironSuite) TestAcquireNodeInterfaces(c *gc.C) {
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
	// In the tests below "space:5" means foo, "space:6" means bar.
	for i, test := range []struct {
		interfaces        []interfaceBinding
		expectedPositives []gomaasapi.InterfaceSpec
		expectedNegatives []string
		expectedError     string
	}{{ // without specified bindings, spaces constraints are used instead.
		interfaces:        nil,
		expectedPositives: []gomaasapi.InterfaceSpec{{"0", "2"}},
		expectedNegatives: []string{"3"},
		expectedError:     "",
	}, {
		interfaces:        []interfaceBinding{{"name-1", "space-1"}},
		expectedPositives: []gomaasapi.InterfaceSpec{{"name-1", "space-1"}, {"0", "2"}},
		expectedNegatives: []string{"3"},
	}, {
		interfaces: []interfaceBinding{
			{"name-1", "7"},
			{"name-2", "8"},
			{"name-3", "9"},
		},
		expectedPositives: []gomaasapi.InterfaceSpec{{"name-1", "7"}, {"name-2", "8"}, {"name-3", "9"}, {"0", "2"}},
		expectedNegatives: []string{"3"},
	}, {
		interfaces:    []interfaceBinding{{"", "anything"}},
		expectedError: "interface bindings cannot have empty names",
	}, {
		interfaces:    []interfaceBinding{{"shared-db", "3"}},
		expectedError: `negative space "bar" from constraints clashes with interface bindings`,
	}, {
		interfaces: []interfaceBinding{
			{"shared-db", "1"},
			{"db", "1"},
		},
		expectedPositives: []gomaasapi.InterfaceSpec{{"shared-db", "1"}, {"db", "1"}, {"0", "2"}},
		expectedNegatives: []string{"3"},
	}, {
		interfaces:    []interfaceBinding{{"", ""}},
		expectedError: "interface bindings cannot have empty names",
	}, {
		interfaces: []interfaceBinding{
			{"valid", "ok"},
			{"", "valid-but-ignored-space"},
			{"valid-name-empty-space", ""},
			{"", ""},
		},
		expectedError: "interface bindings cannot have empty names",
	}, {
		interfaces:    []interfaceBinding{{"foo", ""}},
		expectedError: `invalid interface binding "foo": space provider ID is required`,
	}, {
		interfaces: []interfaceBinding{
			{"bar", ""},
			{"valid", "ok"},
			{"", "valid-but-ignored-space"},
			{"", ""},
		},
		expectedError: `invalid interface binding "bar": space provider ID is required`,
	}, {
		interfaces: []interfaceBinding{
			{"dup-name", "1"},
			{"dup-name", "2"},
		},
		expectedError: `duplicated interface binding "dup-name"`,
	}, {
		interfaces: []interfaceBinding{
			{"valid-1", "0"},
			{"dup-name", "1"},
			{"dup-name", "2"},
			{"valid-2", "3"},
		},
		expectedError: `duplicated interface binding "dup-name"`,
	}} {
		c.Logf("test #%d: interfaces=%v", i, test.interfaces)
		env = suite.makeEnviron(c, nil)
		getNegatives = func() []string {
			return test.expectedNegatives
		}
		getPositives = func() []gomaasapi.InterfaceSpec {
			return test.expectedPositives
		}
		_, err := env.acquireNode2("", "", cons, test.interfaces, nil)
		if test.expectedError != "" {
			c.Check(err, gc.ErrorMatches, test.expectedError)
			c.Check(err, jc.Satisfies, errors.IsNotValid)
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

func (suite *maas2EnvironSuite) TestAcquireNodeConvertsSpaceNames(c *gc.C) {
	expected := gomaasapi.AllocateMachineArgs{
		NotSpace:   []string{"3"},
		Interfaces: []gomaasapi.InterfaceSpec{{Label: "0", Space: "2"}},
	}
	env, _ := suite.injectControllerWithSpacesAndCheck(c, getTwoSpaces(), expected)
	cons := constraints.Value{
		Spaces: stringslicep("foo", "^bar"),
	}
	_, err := env.acquireNode2("", "", cons, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *maas2EnvironSuite) TestAcquireNodeTranslatesSpaceNames(c *gc.C) {
	expected := gomaasapi.AllocateMachineArgs{
		NotSpace:   []string{"3"},
		Interfaces: []gomaasapi.InterfaceSpec{{Label: "0", Space: "2"}},
	}
	env, _ := suite.injectControllerWithSpacesAndCheck(c, getTwoSpaces(), expected)
	cons := constraints.Value{
		Spaces: stringslicep("foo-1", "^bar-3"),
	}
	_, err := env.acquireNode2("", "", cons, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *maas2EnvironSuite) TestAcquireNodeUnrecognisedSpace(c *gc.C) {
	suite.injectController(&fakeController{})
	env := suite.makeEnviron(c, nil)
	cons := constraints.Value{
		Spaces: stringslicep("baz"),
	}
	_, err := env.acquireNode2("", "", cons, nil, nil)
	c.Assert(err, gc.ErrorMatches, `unrecognised space in constraint "baz"`)
}

func (suite *maas2EnvironSuite) TestWaitForNodeDeploymentError(c *gc.C) {
	machine := newFakeMachine("Bruce Sterling", arch.HostArch(), "")
	controller := newFakeController()
	controller.allocateMachine = machine
	controller.allocateMachineMatches = gomaasapi.ConstraintMatches{
		Storage: map[string][]gomaasapi.BlockDevice{},
	}
	controller.machines = []gomaasapi.Machine{machine}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env := suite.makeEnviron(c, nil)
	err := bootstrap.Bootstrap(envjujutesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		AdminSecret:      jujutesting.AdminSecret,
		CAPrivateKey:     coretesting.CAKey,
	})
	c.Assert(err, gc.ErrorMatches, "bootstrap instance started but did not change to Deployed state.*")
}

func (suite *maas2EnvironSuite) TestWaitForNodeDeploymentSucceeds(c *gc.C) {
	machine := newFakeMachine("Bruce Sterling", arch.HostArch(), "Deployed")
	controller := newFakeController()
	controller.allocateMachine = machine
	controller.allocateMachineMatches = gomaasapi.ConstraintMatches{
		Storage: map[string][]gomaasapi.BlockDevice{},
	}
	controller.machines = []gomaasapi.Machine{machine}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env := suite.makeEnviron(c, nil)
	err := bootstrap.Bootstrap(envjujutesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		AdminSecret:      jujutesting.AdminSecret,
		CAPrivateKey:     coretesting.CAKey,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *maas2EnvironSuite) TestSubnetsNoFilters(c *gc.C) {
	suite.injectController(&fakeController{
		spaces: getFourSpaces(),
	})
	env := suite.makeEnviron(c, nil)
	subnets, err := env.Subnets("", nil)
	c.Assert(err, jc.ErrorIsNil)
	expected := []network.SubnetInfo{
		{CIDR: "192.168.10.0/24", ProviderId: "99", VLANTag: 66, SpaceProviderId: "5"},
		{CIDR: "192.168.11.0/24", ProviderId: "100", VLANTag: 66, SpaceProviderId: "6"},
		{CIDR: "192.168.12.0/24", ProviderId: "101", VLANTag: 66, SpaceProviderId: "7"},
		{CIDR: "192.168.13.0/24", ProviderId: "102", VLANTag: 66, SpaceProviderId: "8"},
	}
	c.Assert(subnets, jc.DeepEquals, expected)
}

func (suite *maas2EnvironSuite) TestSubnetsNoFiltersError(c *gc.C) {
	suite.injectController(&fakeController{
		spacesError: errors.New("bang"),
	})
	env := suite.makeEnviron(c, nil)
	_, err := env.Subnets("", nil)
	c.Assert(err, gc.ErrorMatches, "bang")
}

func (suite *maas2EnvironSuite) TestSubnetsSubnetIds(c *gc.C) {
	suite.injectController(&fakeController{
		spaces: getFourSpaces(),
	})
	env := suite.makeEnviron(c, nil)
	subnets, err := env.Subnets("", []network.Id{"99", "100"})
	c.Assert(err, jc.ErrorIsNil)
	expected := []network.SubnetInfo{
		{CIDR: "192.168.10.0/24", ProviderId: "99", VLANTag: 66, SpaceProviderId: "5"},
		{CIDR: "192.168.11.0/24", ProviderId: "100", VLANTag: 66, SpaceProviderId: "6"},
	}
	c.Assert(subnets, jc.DeepEquals, expected)
}

func (suite *maas2EnvironSuite) TestSubnetsSubnetIdsMissing(c *gc.C) {
	suite.injectController(&fakeController{
		spaces: getFourSpaces(),
	})
	env := suite.makeEnviron(c, nil)
	_, err := env.Subnets("", []network.Id{"99", "missing"})
	msg := "failed to find the following subnets: missing"
	c.Assert(err, gc.ErrorMatches, msg)
}

func (suite *maas2EnvironSuite) TestSubnetsInstIdNotFound(c *gc.C) {
	suite.injectController(&fakeController{})
	env := suite.makeEnviron(c, nil)
	_, err := env.Subnets("foo", nil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (suite *maas2EnvironSuite) TestSubnetsInstId(c *gc.C) {
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
	subnets, err := env.Subnets("William Gibson", nil)
	c.Assert(err, jc.ErrorIsNil)
	expected := []network.SubnetInfo{
		{CIDR: "192.168.10.0/24", ProviderId: "99", VLANTag: 66, SpaceProviderId: "5"},
		{CIDR: "192.168.11.0/24", ProviderId: "100", VLANTag: 0, SpaceProviderId: "6"},
		{CIDR: "192.168.12.0/24", ProviderId: "101", VLANTag: 2, SpaceProviderId: "7"},
	}
	c.Assert(subnets, jc.DeepEquals, expected)
}

func (suite *maas2EnvironSuite) TestStartInstanceNetworkInterfaces(c *gc.C) {
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
			Storage: map[string][]gomaasapi.BlockDevice{},
		},
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)

	params := environs.StartInstanceParams{ControllerUUID: suite.controllerUUID}
	result, err := jujutesting.StartInstanceWithParams(env, "1", params)
	c.Assert(err, jc.ErrorIsNil)
	expected := []network.InterfaceInfo{{
		DeviceIndex:       0,
		MACAddress:        "52:54:00:70:9b:fe",
		CIDR:              "10.20.19.0/24",
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
		ConfigType:        "static",
		Address:           network.NewAddressOnSpace("default", "10.20.19.103"),
		DNSServers:        network.NewAddressesOnSpace("default", "10.20.19.2", "10.20.19.3"),
		DNSSearchDomains:  nil,
		MTU:               1500,
		GatewayAddress:    network.NewAddressOnSpace("default", "10.20.19.2"),
	}, {
		DeviceIndex:       0,
		MACAddress:        "52:54:00:70:9b:fe",
		CIDR:              "10.20.19.0/24",
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
		ConfigType:        "static",
		Address:           network.NewAddressOnSpace("default", "10.20.19.104"),
		DNSServers:        network.NewAddressesOnSpace("default", "10.20.19.2", "10.20.19.3"),
		DNSSearchDomains:  nil,
		MTU:               1500,
		GatewayAddress:    network.NewAddressOnSpace("default", "10.20.19.2"),
	}, {
		DeviceIndex:         1,
		MACAddress:          "52:54:00:70:9b:fe",
		CIDR:                "10.50.19.0/24",
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
		ConfigType:          "static",
		Address:             network.NewAddressOnSpace("admin", "10.50.19.103"),
		DNSServers:          nil,
		DNSSearchDomains:    nil,
		MTU:                 1500,
		GatewayAddress:      network.NewAddressOnSpace("admin", "10.50.19.2"),
	},
	}
	c.Assert(result.NetworkInfo, jc.DeepEquals, expected)
}

func (suite *maas2EnvironSuite) TestAllocateContainerAddressesSingleNic(c *gc.C) {
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
		devices: []gomaasapi.Device{device},
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)

	prepared := []network.InterfaceInfo{{
		MACAddress:    "52:54:00:70:9b:fe",
		CIDR:          "10.20.19.0/24",
		InterfaceName: "eth0",
	}}
	ignored := names.NewMachineTag("1/lxd/0")
	result, err := env.AllocateContainerAddresses(instance.Id("1"), ignored, prepared)
	c.Assert(err, jc.ErrorIsNil)
	expected := []network.InterfaceInfo{{
		DeviceIndex:       0,
		MACAddress:        "53:54:00:70:9b:ff",
		CIDR:              "192.168.1.0/24",
		ProviderId:        "93",
		ProviderSubnetId:  "4",
		VLANTag:           0,
		ProviderVLANId:    "5002",
		ProviderAddressId: "480",
		InterfaceName:     "eth1",
		InterfaceType:     "ethernet",
		ConfigType:        "static",
		Address:           network.NewAddressOnSpace("freckles", "192.168.1.127"),
		DNSServers:        network.NewAddressesOnSpace("freckles", "10.20.19.2", "10.20.19.3"),
		MTU:               1500,
		GatewayAddress:    network.NewAddressOnSpace("freckles", "192.168.1.1"),
	}}
	c.Assert(result, jc.DeepEquals, expected)
}

func (suite *maas2EnvironSuite) TestAllocateContainerAddressesDualNic(c *gc.C) {
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
		devices: []gomaasapi.Device{device},
	}
	suite.injectController(controller)
	env := suite.makeEnviron(c, nil)

	prepared := []network.InterfaceInfo{{
		MACAddress:    "53:54:00:70:9b:ff",
		CIDR:          "10.20.19.0/24",
		InterfaceName: "eth0",
	}, {
		MACAddress:    "52:54:00:70:9b:f4",
		CIDR:          "192.168.1.0/24",
		InterfaceName: "eth1",
	}}
	expected := []network.InterfaceInfo{{
		DeviceIndex:       0,
		MACAddress:        "53:54:00:70:9b:ff",
		CIDR:              "10.20.19.0/24",
		ProviderId:        "93",
		ProviderSubnetId:  "3",
		ProviderVLANId:    "5001",
		ProviderAddressId: "480",
		InterfaceName:     "eth0",
		InterfaceType:     "ethernet",
		ConfigType:        "static",
		Address:           network.NewAddressOnSpace("freckles", "10.20.19.127"),
		DNSServers:        network.NewAddressesOnSpace("freckles", "10.20.19.2", "10.20.19.3"),
		MTU:               1500,
		GatewayAddress:    network.NewAddressOnSpace("freckles", "10.20.19.2"),
	}, {
		DeviceIndex:       0,
		MACAddress:        "52:54:00:70:9b:f4",
		CIDR:              "192.168.1.0/24",
		ProviderId:        "94",
		ProviderSubnetId:  "4",
		ProviderVLANId:    "5002",
		ProviderAddressId: "481",
		InterfaceName:     "eth1",
		InterfaceType:     "ethernet",
		ConfigType:        "static",
		Address:           network.NewAddressOnSpace("freckles", "192.168.1.127"),
		DNSServers:        network.NewAddressesOnSpace("freckles", "10.20.19.2", "10.20.19.3"),
		MTU:               1500,
		GatewayAddress:    network.NewAddressOnSpace("freckles", "192.168.1.1"),
	}}
	ignored := names.NewMachineTag("1/lxd/0")
	result, err := env.AllocateContainerAddresses(instance.Id("1"), ignored, prepared)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expected)
}

func (suite *maas2EnvironSuite) assertAllocateContainerAddressesFails(c *gc.C, controller *fakeController, prepared []network.InterfaceInfo, errorMatches string) {
	if prepared == nil {
		prepared = []network.InterfaceInfo{{}}
	}
	suite.injectController(controller)
	env := suite.makeEnviron(c, nil)
	ignored := names.NewMachineTag("1/lxd/0")
	_, err := env.AllocateContainerAddresses(instance.Id("1"), ignored, prepared)
	c.Assert(err, gc.ErrorMatches, errorMatches)
}

func (suite *maas2EnvironSuite) TestAllocateContainerAddressesSpacesError(c *gc.C) {
	controller := &fakeController{spacesError: errors.New("boom")}
	suite.assertAllocateContainerAddressesFails(c, controller, nil, "boom")
}

func (suite *maas2EnvironSuite) TestAllocateContainerAddressesPrimaryInterfaceMissing(c *gc.C) {
	controller := &fakeController{}
	suite.assertAllocateContainerAddressesFails(c, controller, nil, "cannot find primary interface for container")
}

func (suite *maas2EnvironSuite) TestAllocateContainerAddressesPrimaryInterfaceSubnetMissing(c *gc.C) {
	controller := &fakeController{}
	prepared := []network.InterfaceInfo{{InterfaceName: "eth0"}}
	errorMatches := "primary NIC subnet  not found"
	suite.assertAllocateContainerAddressesFails(c, controller, prepared, errorMatches)
}

func makeFakeSubnet(id int) fakeSubnet {
	return fakeSubnet{
		id:      id,
		space:   "freckles",
		gateway: fmt.Sprintf("10.20.%d.2", 16+id),
		cidr:    fmt.Sprintf("10.20.%d.0/24", 16+id),
	}
}
func (suite *maas2EnvironSuite) TestAllocateContainerAddressesMachinesError(c *gc.C) {
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
	prepared := []network.InterfaceInfo{
		{InterfaceName: "eth0", CIDR: "10.20.19.0/24"},
	}
	ignored := names.NewMachineTag("1/lxd/0")
	_, err := env.AllocateContainerAddresses(instance.Id("1"), ignored, prepared)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func getArgs(c *gc.C, calls []testing.StubCall) interface{} {
	c.Assert(calls, gc.HasLen, 1)
	args := calls[0].Args
	c.Assert(args, gc.HasLen, 1)
	return args[0]
}

func (suite *maas2EnvironSuite) TestAllocateContainerAddressesCreateDevicerror(c *gc.C) {
	subnet := makeFakeSubnet(3)
	var env *maasEnviron
	machine := &fakeMachine{
		Stub:     &testing.Stub{},
		systemID: "1",
	}
	machine.SetErrors(errors.New("boom"))
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
	prepared := []network.InterfaceInfo{
		{InterfaceName: "eth0", CIDR: "10.20.19.0/24", MACAddress: "DEADBEEF"},
	}
	ignored := names.NewMachineTag("1/lxd/0")
	_, err := env.AllocateContainerAddresses(instance.Id("1"), ignored, prepared)
	c.Assert(err, gc.ErrorMatches, "boom")
	args := getArgs(c, machine.Calls())
	maasArgs, ok := args.(gomaasapi.CreateMachineDeviceArgs)
	c.Assert(ok, jc.IsTrue)
	expected := gomaasapi.CreateMachineDeviceArgs{
		Hostname:      "juju-06f00d-1-lxd-0",
		Subnet:        subnet,
		MACAddress:    "DEADBEEF",
		InterfaceName: "eth0",
	}
	c.Assert(maasArgs, jc.DeepEquals, expected)
}

func (suite *maas2EnvironSuite) TestAllocateContainerAddressesSecondNICSubnetMissing(c *gc.C) {
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
						id:        480,
						subnet:    &subnet,
						ipAddress: "10.20.19.127",
						mode:      "static",
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
	prepared := []network.InterfaceInfo{
		{InterfaceName: "eth0", CIDR: "10.20.19.0/24", MACAddress: "DEADBEEF"},
		{InterfaceName: "eth1", CIDR: "10.20.20.0/24", MACAddress: "DEADBEEE"},
	}
	ignored := names.NewMachineTag("1/lxd/0")
	allocated, err := env.AllocateContainerAddresses(instance.Id("1"), ignored, prepared)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allocated, jc.DeepEquals, []network.InterfaceInfo{{
		MACAddress:        "53:54:00:70:9b:ff",
		CIDR:              "10.20.19.0/24",
		ProviderId:        "93",
		ProviderSubnetId:  "3",
		ProviderVLANId:    "0",
		ProviderAddressId: "480",
		VLANTag:           0,
		InterfaceName:     "eth0",
		InterfaceType:     "ethernet",
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        "static",
		Address:           network.NewAddressOnSpace("freckles", "10.20.19.127"),
		MTU:               1500,
		GatewayAddress:    network.NewAddressOnSpace("freckles", "10.20.19.2"),
	}, {
		MACAddress:       "53:54:00:70:9b:f1",
		CIDR:             "",
		ProviderId:       "94",
		ProviderSubnetId: "",
		ProviderVLANId:   "0",
		VLANTag:          0,
		InterfaceName:    "eth1",
		InterfaceType:    "ethernet",
		Disabled:         false,
		NoAutoStart:      false,
		ConfigType:       "manual",
		MTU:              1500,
	}})
}

func (suite *maas2EnvironSuite) TestAllocateContainerAddressesCreateInterfaceError(c *gc.C) {
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
	prepared := []network.InterfaceInfo{
		{InterfaceName: "eth0", CIDR: "10.20.19.0/24", MACAddress: "DEADBEEF"},
		{InterfaceName: "eth1", CIDR: "10.20.20.0/24", MACAddress: "DEADBEEE"},
	}
	ignored := names.NewMachineTag("1/lxd/0")
	_, err := env.AllocateContainerAddresses(instance.Id("1"), ignored, prepared)
	c.Assert(err, gc.ErrorMatches, "creating device interface: boom")
	args := getArgs(c, device.Calls())
	maasArgs, ok := args.(gomaasapi.CreateInterfaceArgs)
	c.Assert(ok, jc.IsTrue)
	expected := gomaasapi.CreateInterfaceArgs{
		MACAddress: "DEADBEEE",
		Name:       "eth1",
		VLAN:       subnet2.VLAN(),
	}
	c.Assert(maasArgs, jc.DeepEquals, expected)
}

func (suite *maas2EnvironSuite) TestAllocateContainerAddressesLinkSubnetError(c *gc.C) {
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
	prepared := []network.InterfaceInfo{
		{InterfaceName: "eth0", CIDR: "10.20.19.0/24", MACAddress: "DEADBEEF"},
		{InterfaceName: "eth1", CIDR: "10.20.20.0/24", MACAddress: "DEADBEEE"},
	}
	ignored := names.NewMachineTag("1/lxd/0")
	allocated, err := env.AllocateContainerAddresses(instance.Id("1"), ignored, prepared)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allocated, jc.DeepEquals, []network.InterfaceInfo{{
		CIDR:             "",
		ProviderId:       "0",
		ProviderSubnetId: "",
		ProviderVLANId:   "0",
		VLANTag:          0,
		InterfaceName:    "",
		InterfaceType:    "ethernet",
		ConfigType:       "",
		MTU:              1500,
		Disabled:         true,
		NoAutoStart:      true,
	}, {
		CIDR:             "",
		ProviderId:       "0",
		ProviderSubnetId: "",
		ProviderVLANId:   "0",
		VLANTag:          0,
		InterfaceName:    "",
		InterfaceType:    "ethernet",
		ConfigType:       "",
		MTU:              1500,
		Disabled:         true,
		NoAutoStart:      true,
	}})

	args := getArgs(c, interface_.Calls())
	maasArgs, ok := args.(gomaasapi.LinkSubnetArgs)
	c.Assert(ok, jc.IsTrue)
	expected := gomaasapi.LinkSubnetArgs{
		Mode:   gomaasapi.LinkModeStatic,
		Subnet: subnet2,
	}
	c.Assert(maasArgs, jc.DeepEquals, expected)
}
func (suite *maas2EnvironSuite) TestStorageReturnsStorage(c *gc.C) {
	controller := newFakeController()
	env := suite.makeEnviron(c, controller)
	stor := env.Storage()
	c.Check(stor, gc.NotNil)

	// The Storage object is really a maas2Storage.
	specificStorage := stor.(*maas2Storage)

	// Its environment pointer refers back to its environment.
	c.Check(specificStorage.environ, gc.Equals, env)
	c.Check(specificStorage.maasController, gc.Equals, controller)
}

func (suite *maas2EnvironSuite) TestStartInstanceEndToEnd(c *gc.C) {
	suite.setupFakeTools(c)
	machine := newFakeMachine("gus", arch.HostArch(), "Deployed")
	file := &fakeFile{name: coretesting.ModelTag.Id() + "-provider-state"}
	controller := newFakeControllerWithFiles(file)
	controller.machines = []gomaasapi.Machine{machine}
	controller.allocateMachine = machine
	controller.allocateMachineMatches = gomaasapi.ConstraintMatches{
		Storage: make(map[string][]gomaasapi.BlockDevice),
	}

	env := suite.makeEnviron(c, controller)
	err := bootstrap.Bootstrap(envjujutesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		AdminSecret:      jujutesting.AdminSecret,
		CAPrivateKey:     coretesting.CAKey,
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
	instanceIds, err := env.ControllerInstances(suite.controllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceIds, gc.HasLen, 1)
	insts, err := env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(insts[0].Id(), gc.Equals, instanceIds[0])

	node1 := newFakeMachine("victor", arch.HostArch(), "Deployed")
	node1.hostname = "host1"
	node1.cpuCount = 1
	node1.memory = 1024
	node1.zoneName = "test_zone"
	controller.allocateMachine = node1

	instance, hc := jujutesting.AssertStartInstance(c, env, suite.controllerUUID, "1")
	c.Check(instance, gc.NotNil)
	c.Assert(hc, gc.NotNil)
	c.Check(hc.String(), gc.Equals, fmt.Sprintf("arch=%s cores=1 mem=1024M availability-zone=test_zone", arch.HostArch()))

	node1.Stub.CheckCallNames(c, "Start", "SetOwnerData")
	startArgs, ok := node1.Stub.Calls()[0].Args[0].(gomaasapi.StartArgs)
	c.Assert(ok, jc.IsTrue)

	decodedUserData, err := decodeUserData(startArgs.UserData)
	c.Assert(err, jc.ErrorIsNil)
	info := machineInfo{"host1"}
	cloudcfg, err := cloudinit.New("precise")
	c.Assert(err, jc.ErrorIsNil)
	cloudinitRunCmd, err := info.cloudinitRunCmd(cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	data, err := goyaml.Marshal(cloudinitRunCmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(decodedUserData), jc.Contains, string(data))

	// Trash the tools and try to start another instance.
	suite.PatchValue(&envtools.DefaultBaseURL, "")
	instance, _, _, err = jujutesting.StartInstance(env, suite.controllerUUID, "2")
	c.Check(instance, gc.IsNil)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (suite *maas2EnvironSuite) TestControllerInstances(c *gc.C) {
	controller := newFakeControllerWithErrors(gomaasapi.NewNoMatchError("state"))
	env := suite.makeEnviron(c, controller)
	_, err := env.ControllerInstances(suite.controllerUUID)
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
		controllerInstances, err := env.ControllerInstances(suite.controllerUUID)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(controllerInstances, jc.SameContents, expected)
	}
}

func (suite *maas2EnvironSuite) TestDestroy(c *gc.C) {
	file1 := &fakeFile{name: coretesting.ModelTag.Id() + "-provider-state"}
	file2 := &fakeFile{name: coretesting.ModelTag.Id() + "-horace"}
	controller := newFakeControllerWithFiles(file1, file2)
	controller.machines = []gomaasapi.Machine{&fakeMachine{systemID: "pete"}}
	env := suite.makeEnviron(c, controller)
	err := env.Destroy()
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

func (suite *maas2EnvironSuite) TestBootstrapFailsIfNoTools(c *gc.C) {
	env := suite.makeEnviron(c, newFakeController())
	vers := version.MustParse("1.2.3")
	err := bootstrap.Bootstrap(envjujutesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		AdminSecret:      jujutesting.AdminSecret,
		CAPrivateKey:     coretesting.CAKey,
		// Disable auto-uploading by setting the agent version
		// to something that's not the current version.
		AgentVersion: &vers,
	})
	c.Check(err, gc.ErrorMatches, "Juju cannot bootstrap because no agent binaries are available for your model(.|\n)*")
}

func (suite *maas2EnvironSuite) TestBootstrapFailsIfNoNodes(c *gc.C) {
	suite.setupFakeTools(c)
	controller := newFakeController()
	controller.allocateMachineError = gomaasapi.NewNoMatchError("oops")
	env := suite.makeEnviron(c, controller)
	err := bootstrap.Bootstrap(envjujutesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		AdminSecret:      jujutesting.AdminSecret,
		CAPrivateKey:     coretesting.CAKey,
	})
	// Since there are no nodes, the attempt to allocate one returns a
	// 409: Conflict.
	c.Check(err, gc.ErrorMatches, ".*cannot run instances.*")
}

func (suite *maas2EnvironSuite) TestGetToolsMetadataSources(c *gc.C) {
	// Add a dummy file to storage so we can use that to check the
	// obtained source later.
	env := suite.makeEnviron(c, newFakeControllerWithFiles(
		&fakeFile{name: coretesting.ModelTag.Id() + "-tools/filename", contents: makeRandomBytes(10)},
	))
	sources, err := envtools.GetMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 0)
}

func (suite *maas2EnvironSuite) TestConstraintsValidator(c *gc.C) {
	controller := newFakeController()
	controller.bootResources = []gomaasapi.BootResource{&fakeBootResource{name: "trusty", architecture: "amd64"}}
	env := suite.makeEnviron(c, controller)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 cpu-power=10 instance-type=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power", "instance-type", "virt-type"})
}

func (suite *maas2EnvironSuite) TestConstraintsValidatorVocab(c *gc.C) {
	controller := newFakeController()
	controller.bootResources = []gomaasapi.BootResource{
		&fakeBootResource{name: "trusty", architecture: "amd64"},
		&fakeBootResource{name: "precise", architecture: "armhf"},
	}
	env := suite.makeEnviron(c, controller)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are: \\[amd64 armhf\\]")
}

func (suite *maas2EnvironSuite) TestReleaseContainerAddresses(c *gc.C) {
	dev1 := newFakeDevice("a", "eleven")
	dev2 := newFakeDevice("b", "will")
	controller := newFakeController()
	controller.devices = []gomaasapi.Device{dev1, dev2}

	env := suite.makeEnviron(c, controller)
	err := env.ReleaseContainerAddresses([]network.ProviderInterfaceInfo{
		{MACAddress: "will"},
		{MACAddress: "dustin"},
		{MACAddress: "eleven"},
	})
	c.Assert(err, jc.ErrorIsNil)

	args, ok := getArgs(c, controller.Calls()).(gomaasapi.DevicesArgs)
	c.Assert(ok, jc.IsTrue)
	expected := gomaasapi.DevicesArgs{MACAddresses: []string{"will", "dustin", "eleven"}}
	c.Assert(args, gc.DeepEquals, expected)

	dev1.CheckCallNames(c, "Delete")
	dev2.CheckCallNames(c, "Delete")
}

func (suite *maas2EnvironSuite) TestReleaseContainerAddresses_HandlesDupes(c *gc.C) {
	dev1 := newFakeDevice("a", "eleven")
	controller := newFakeController()
	controller.devices = []gomaasapi.Device{dev1, dev1}

	env := suite.makeEnviron(c, controller)
	err := env.ReleaseContainerAddresses([]network.ProviderInterfaceInfo{
		{MACAddress: "will"},
		{MACAddress: "eleven"},
	})
	c.Assert(err, jc.ErrorIsNil)

	args, ok := getArgs(c, controller.Calls()).(gomaasapi.DevicesArgs)
	c.Assert(ok, jc.IsTrue)
	expected := gomaasapi.DevicesArgs{MACAddresses: []string{"will", "eleven"}}
	c.Assert(args, gc.DeepEquals, expected)

	dev1.CheckCallNames(c, "Delete")
}

func (suite *maas2EnvironSuite) TestReleaseContainerAddressesErrorGettingDevices(c *gc.C) {
	controller := newFakeControllerWithErrors(errors.New("Everything done broke"))
	env := suite.makeEnviron(c, controller)
	err := env.ReleaseContainerAddresses([]network.ProviderInterfaceInfo{{MACAddress: "anything"}})
	c.Assert(err, gc.ErrorMatches, "Everything done broke")
}

func (suite *maas2EnvironSuite) TestReleaseContainerAddressesErrorDeletingDevice(c *gc.C) {
	dev1 := newFakeDevice("a", "eleven")
	dev1.systemID = "hopper"
	dev1.SetErrors(errors.New("don't delete me"))
	controller := newFakeController()
	controller.devices = []gomaasapi.Device{dev1}

	env := suite.makeEnviron(c, controller)
	err := env.ReleaseContainerAddresses([]network.ProviderInterfaceInfo{
		{MACAddress: "eleven"},
	})
	c.Assert(err, gc.ErrorMatches, "deleting device hopper: don't delete me")

	_, ok := getArgs(c, controller.Calls()).(gomaasapi.DevicesArgs)
	c.Assert(ok, jc.IsTrue)

	dev1.CheckCallNames(c, "Delete")
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
