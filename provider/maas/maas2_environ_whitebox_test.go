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

func (suite *maas2EnvironSuite) TestStopInstancesReturnsIfParameterEmpty(c *gc.C) {
	controller := &fakeController{}
	err := suite.makeEnviron(c, controller).StopInstances()
	c.Check(err, jc.ErrorIsNil)
	c.Assert(controller.releaseMachinesArgs, gc.IsNil)
}

func (suite *maas2EnvironSuite) TestStopInstancesStopsAndReleasesInstances(c *gc.C) {
	// Return a cannot complete indicating that test1 is in the wrong state.
	// The release operation will still release the others and succeed.
	controller := &fakeController{
		files: []gomaasapi.File{&fakeFile{name: "agent-prefix-provider-state"}},
	}
	err := suite.makeEnviron(c, controller).StopInstances("test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)
	args := controller.releaseMachinesArgs
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0].SystemIDs, gc.DeepEquals, []string{"test1", "test2", "test3"})
}

func (suite *maas2EnvironSuite) TestStopInstancesIgnoresConflict(c *gc.C) {
	// Return a cannot complete indicating that test1 is in the wrong state.
	// The release operation will still release the others and succeed.
	controller := &fakeController{
		releaseMachinesErrors: []error{gomaasapi.NewCannotCompleteError("test1 not allocated")},
		files: []gomaasapi.File{&fakeFile{name: "agent-prefix-provider-state"}},
	}
	err := suite.makeEnviron(c, controller).StopInstances("test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)
	args := controller.releaseMachinesArgs
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0].SystemIDs, gc.DeepEquals, []string{"test1", "test2", "test3"})
}

func (suite *maas2EnvironSuite) TestStopInstancesIgnoresMissingNodeAndRecurses(c *gc.C) {
	controller := &fakeController{
		releaseMachinesErrors: []error{
			gomaasapi.NewBadRequestError("no such machine: test1"),
			gomaasapi.NewBadRequestError("no such machine: test1"),
		},
		files: []gomaasapi.File{&fakeFile{name: "agent-prefix-provider-state"}},
	}
	err := suite.makeEnviron(c, controller).StopInstances("test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)
	args := controller.releaseMachinesArgs
	c.Assert(args, gc.HasLen, 4)
	c.Assert(args[0].SystemIDs, gc.DeepEquals, []string{"test1", "test2", "test3"})
	c.Assert(args[1].SystemIDs, gc.DeepEquals, []string{"test1"})
	c.Assert(args[2].SystemIDs, gc.DeepEquals, []string{"test2"})
	c.Assert(args[3].SystemIDs, gc.DeepEquals, []string{"test3"})
}

func (suite *maas2EnvironSuite) checkStopInstancesFails(c *gc.C, withError error) {
	controller := &fakeController{
		releaseMachinesErrors: []error{withError},
		files: []gomaasapi.File{&fakeFile{name: "agent-prefix-provider-state"}},
	}
	err := suite.makeEnviron(c, controller).StopInstances("test1", "test2", "test3")
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("cannot release nodes: %s", withError))
	// Only tries once.
	c.Assert(controller.releaseMachinesArgs, gc.HasLen, 1)
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
	var env *maasEnviron
	env = suite.injectControllerWithSpacesAndCheck(c, nil, gomaasapi.AllocateMachineArgs{})

	params := environs.StartInstanceParams{}
	result, err := testing.StartInstanceWithParams(env, "1", params, nil)
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
		zones: []gomaasapi.Zone{&fakeZone{name: "foo"}},
	})
	suite.setupFakeTools(c)
	env = suite.makeEnviron(c, nil)
	params := environs.StartInstanceParams{
		Placement:   "zone=foo",
		Constraints: constraints.MustParse("mem=8G"),
	}
	result, err := testing.StartInstanceWithParams(env, "1", params, nil)
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
			subnets: []gomaasapi.Subnet{fakeSubnet{id: 99, vlan: fakeVLAN{vid: 66}, cidr: "192.168.12.0/24"}},
			id:      7,
		},
		fakeSpace{
			name:    "space-4",
			subnets: []gomaasapi.Subnet{fakeSubnet{id: 100, vlan: fakeVLAN{vid: 66}, cidr: "192.168.13.0/24"}},
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
	suite.injectController(&fakeController{
		allocateMachine: machine,
		machines:        []gomaasapi.Machine{machine},
	})
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
	suite.injectController(&fakeController{
		allocateMachine: machine,
		machines:        []gomaasapi.Machine{machine},
	})
	suite.setupFakeTools(c)
	env := suite.makeEnviron(c, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
}
