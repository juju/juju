// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
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
	testAttrs := coretesting.Attrs{}
	for k, v := range maasEnvAttrs {
		testAttrs[k] = v
	}
	testAttrs["maas-server"] = testServer.Server.URL
	attrs := coretesting.FakeConfig().Merge(testAttrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return NewEnviron(cfg)
}

func (suite *maas2EnvironSuite) TestNewEnvironWithoutFeatureFlag(c *gc.C) {
	suite.SetFeatureFlags()
	_, err := suite.getEnvWithServer(c)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (suite *maas2EnvironSuite) TestNewEnvironWithController(c *gc.C) {
	env, err := suite.getEnvWithServer(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
}

func (suite *maas2EnvironSuite) TestSupportedArchitectures(c *gc.C) {
	controller := &fakeController{
		bootResources: []gomaasapi.BootResource{
			&fakeBootResource{name: "wily", architecture: "amd64/blah"},
			&fakeBootResource{name: "wily", architecture: "amd64/something"},
			&fakeBootResource{name: "xenial", architecture: "arm/somethingelse"},
		},
	}
	env := suite.makeEnviron(c, controller)
	result, err := env.SupportedArchitectures()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []string{"amd64", "arm"})
}

func (suite *maas2EnvironSuite) TestSupportedArchitecturesError(c *gc.C) {
	env := suite.makeEnviron(c, &fakeController{bootResourcesError: errors.New("Something terrible!")})
	_, err := env.SupportedArchitectures()
	c.Assert(err, gc.ErrorMatches, "Something terrible!")
}

func (suite *maas2EnvironSuite) injectControllerWithSpacesAndCheck(c *gc.C, spaces []gomaasapi.Space, expected gomaasapi.AllocateMachineArgs) *maasEnviron {
	var env *maasEnviron
	check := func(args gomaasapi.AllocateMachineArgs) {
		expected.AgentName = env.ecfg().maasAgentName()
		c.Assert(args, gc.DeepEquals, expected)
	}
	controller := &fakeController{
		allocateMachineArgsCheck: check,
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
		allocateMachineMatches: gomaasapi.ConstraintMatches{
			Storage: map[string]gomaasapi.BlockDevice{},
		},
		spaces: spaces,
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)
	return env
}

func (suite *maas2EnvironSuite) makeEnvironWithMachines(c *gc.C, expectedSystemIDs []string, returnSystemIDs []string) *maasEnviron {
	var env *maasEnviron
	checkArgs := func(args gomaasapi.MachinesArgs) {
		c.Check(args.SystemIDs, gc.DeepEquals, expectedSystemIDs)
		c.Check(args.AgentName, gc.Equals, env.ecfg().maasAgentName())
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
	controller := newFakeControllerWithFiles(&fakeFile{name: "agent-prefix-provider-state"})
	err := suite.makeEnviron(c, controller).StopInstances("test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)
	args := collectReleaseArgs(controller)
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0].SystemIDs, gc.DeepEquals, []string{"test1", "test2", "test3"})
}

func (suite *maas2EnvironSuite) TestStopInstancesIgnoresConflict(c *gc.C) {
	// Return a cannot complete indicating that test1 is in the wrong state.
	// The release operation will still release the others and succeed.
	controller := newFakeControllerWithFiles(&fakeFile{name: "agent-prefix-provider-state"})
	controller.SetErrors(gomaasapi.NewCannotCompleteError("test1 not allocated"))
	err := suite.makeEnviron(c, controller).StopInstances("test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)

	args := collectReleaseArgs(controller)
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0].SystemIDs, gc.DeepEquals, []string{"test1", "test2", "test3"})
}

func (suite *maas2EnvironSuite) TestStopInstancesIgnoresMissingNodeAndRecurses(c *gc.C) {
	controller := newFakeControllerWithFiles(&fakeFile{name: "agent-prefix-provider-state"})
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
	controller := newFakeControllerWithFiles(&fakeFile{name: "agent-prefix-provider-state"})
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
	env := suite.injectControllerWithSpacesAndCheck(c, nil, gomaasapi.AllocateMachineArgs{})

	params := environs.StartInstanceParams{}
	result, err := testing.StartInstanceWithParams(env, "1", params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Instance.Id(), gc.Equals, instance.Id("Bruce Sterling"))
}

func (suite *maas2EnvironSuite) TestStartInstanceParams(c *gc.C) {
	var env *maasEnviron
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, gc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.ecfg().maasAgentName(),
				Zone:      "foo",
				MinMemory: 8192,
			})
		},
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
		allocateMachineMatches: gomaasapi.ConstraintMatches{
			Storage: map[string]gomaasapi.BlockDevice{},
		},
		zones: []gomaasapi.Zone{&fakeZone{name: "foo"}},
	})
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)
	params := environs.StartInstanceParams{
		Placement:   "zone=foo",
		Constraints: constraints.MustParse("mem=8G"),
	}
	result, err := testing.StartInstanceWithParams(env, "1", params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Instance.Id(), gc.Equals, instance.Id("Bruce Sterling"))
}

func (suite *maas2EnvironSuite) TestAcquireNodePassedAgentName(c *gc.C) {
	var env *maasEnviron
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, gc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.ecfg().maasAgentName()})
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
	env = suite.injectControllerWithSpacesAndCheck(c, nil, expected)
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
	env := suite.injectControllerWithSpacesAndCheck(c, getFourSpaces(), expected)

	_, err := env.acquireNode2(
		"", "",
		constraints.Value{Spaces: stringslicep("space-1", "^space-2", "space-3", "^space-4")},
		nil, nil,
	)
	c.Check(err, jc.ErrorIsNil)
}

func (suite *maas2EnvironSuite) TestAcquireNodeDisambiguatesNamedLabelsFromIndexedUpToALimit(c *gc.C) {
	env := suite.injectControllerWithSpacesAndCheck(c, getFourSpaces(), gomaasapi.AllocateMachineArgs{})
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
				AgentName: env.ecfg().maasAgentName(),
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
				AgentName:  env.ecfg().maasAgentName(),
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
	env := suite.injectControllerWithSpacesAndCheck(c, getTwoSpaces(), expected)
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
	env := suite.injectControllerWithSpacesAndCheck(c, getTwoSpaces(), expected)
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
	machine := &fakeMachine{
		systemID:     "Bruce Sterling",
		architecture: arch.HostArch(),
	}
	controller := newFakeController()
	controller.allocateMachine = machine
	controller.allocateMachineMatches = gomaasapi.ConstraintMatches{
		Storage: map[string]gomaasapi.BlockDevice{},
	}
	controller.machines = []gomaasapi.Machine{machine}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env := suite.makeEnviron(c, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "bootstrap instance started but did not change to Deployed state.*")
}

func (suite *maas2EnvironSuite) TestWaitForNodeDeploymentSucceeds(c *gc.C) {
	machine := &fakeMachine{
		systemID:     "Bruce Sterling",
		architecture: arch.HostArch(),
		statusName:   "Deployed",
	}

	controller := newFakeController()
	controller.allocateMachine = machine
	controller.allocateMachineMatches = gomaasapi.ConstraintMatches{
		Storage: map[string]gomaasapi.BlockDevice{},
	}
	controller.machines = []gomaasapi.Machine{machine}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env := suite.makeEnviron(c, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
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
	controller := &fakeController{
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
			interfaceSet: exampleInterfaces,
		},
		allocateMachineMatches: gomaasapi.ConstraintMatches{
			Storage: map[string]gomaasapi.BlockDevice{},
		},
	}
	suite.injectController(controller)
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)

	params := environs.StartInstanceParams{}
	result, err := testing.StartInstanceWithParams(env, "1", params)
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
