// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type maas2EnvironSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&maas2EnvironSuite{})

func makeEnviron(c *gc.C) *maasEnviron {
	testAttrs := coretesting.Attrs{}
	for k, v := range maasEnvAttrs {
		testAttrs[k] = v
	}
	testAttrs["maas-server"] = "http://any-old-junk.invalid/"
	testAttrs["agent-version"] = version.Current.String()
	attrs := coretesting.FakeConfig().Merge(testAttrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	env, err := NewEnviron(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
	return env
}

func (suite *maas2EnvironSuite) SetUpTest(c *gc.C) {
	suite.baseProviderSuite.SetUpTest(c)
	suite.SetFeatureFlags(feature.MAAS2)
}

func (suite *maas2EnvironSuite) getEnvWithServer(c *gc.C) (*maasEnviron, error) {
	testServer := gomaasapi.NewSimpleServer()
	testServer.AddGetResponse("/api/2.0/version/", http.StatusOK, maas2VersionResponse)
	testServer.AddGetResponse("/api/2.0/users/?op=whoami", http.StatusOK, "{}")
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
	controller := fakeController{
		bootResources: []gomaasapi.BootResource{
			&fakeBootResource{name: "wily", architecture: "amd64/blah"},
			&fakeBootResource{name: "wily", architecture: "amd64/something"},
			&fakeBootResource{name: "xenial", architecture: "arm/somethingelse"},
		},
	}
	suite.injectController(&controller)
	env := makeEnviron(c)
	result, err := env.SupportedArchitectures()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []string{"amd64", "arm"})
}

func (suite *maas2EnvironSuite) TestSupportedArchitecturesError(c *gc.C) {
	suite.injectController(&fakeController{bootResourcesError: errors.New("Something terrible!")})
	env := makeEnviron(c)
	_, err := env.SupportedArchitectures()
	c.Assert(err, gc.ErrorMatches, "Something terrible!")
}

func (suite *maas2EnvironSuite) injectController(controller gomaasapi.Controller) {
	mockGetController := func(maasServer, apiKey string) (gomaasapi.Controller, error) {
		return controller, nil
	}
	suite.PatchValue(&GetMAAS2Controller, mockGetController)
}

func (suite *maas2EnvironSuite) makeEnvironWithMachines(c *gc.C, expectedSystemIDs []string, returnSystemIDs []string) *maasEnviron {
	var env *maasEnviron
	checkArgs := func(args gomaasapi.MachinesArgs) {
		c.Check(args.SystemIDs, jc.DeepEquals, expectedSystemIDs)
		c.Check(args.AgentName, gc.Equals, env.ecfg().maasAgentName())
	}
	machines := make([]gomaasapi.Machine, len(returnSystemIDs))
	for index, id := range returnSystemIDs {
		machines[index] = &fakeMachine{systemID: id}
	}
	suite.injectController(&fakeController{
		machines:          machines,
		machinesArgsCheck: checkArgs,
	})
	env = makeEnviron(c)
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
	c.Assert(actualMachines, jc.DeepEquals, expectedMachines)
}

func (suite *maas2EnvironSuite) TestAllInstancesError(c *gc.C) {
	suite.injectController(&fakeController{machinesError: errors.New("Something terrible!")})
	env := makeEnviron(c)
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
	c.Assert(actualMachines, jc.DeepEquals, expectedMachines)
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
	suite.injectController(&fakeController{
		zones: []gomaasapi.Zone{
			&fakeZone{name: "mossack"},
			&fakeZone{name: "fonseca"},
		},
	})
	env := makeEnviron(c)
	result, err := env.AvailabilityZones()
	c.Assert(err, jc.ErrorIsNil)
	expectedZones := set.NewStrings("mossack", "fonseca")
	actualZones := set.NewStrings()
	for _, zone := range result {
		actualZones.Add(zone.Name())
	}
	c.Assert(actualZones, jc.DeepEquals, expectedZones)
}

func (suite *maas2EnvironSuite) TestAvailabilityZonesError(c *gc.C) {
	suite.injectController(&fakeController{
		zonesError: errors.New("a bad thing"),
	})
	env := makeEnviron(c)
	_, err := env.AvailabilityZones()
	c.Assert(err, gc.ErrorMatches, "a bad thing")
}

func (suite *maas2EnvironSuite) TestSpaces(c *gc.C) {
	suite.injectController(&fakeController{
		spaces: []gomaasapi.Space{
			fakeSpace{
				name: "pepper",
				id:   1234,
			},
			fakeSpace{
				name: "freckles",
				id:   4567,
				subnets: []gomaasapi.Subnet{
					fakeSubnet{id: 99, vlanVid: 66, cidr: "192.168.10.0/24"},
					fakeSubnet{id: 98, vlanVid: 67, cidr: "192.168.11.0/24"},
				},
			},
		},
	})
	env := makeEnviron(c)
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
	suite.injectController(&fakeController{
		spacesError: errors.New("Joe Manginiello"),
	})
	env := makeEnviron(c)
	_, err := env.Spaces()
	c.Assert(err, gc.ErrorMatches, "Joe Manginiello")
}

func (suite *maas2EnvironSuite) TestStartInstanceError(c *gc.C) {
	suite.injectController(&fakeController{
		allocateMachineError: errors.New("Charles Babbage"),
	})
	env := makeEnviron(c)
	_, err := env.StartInstance(environs.StartInstanceParams{})
	c.Assert(err, gc.ErrorMatches, ".* cannot run instance: Charles Babbage")
}

func (suite *maas2EnvironSuite) TestStartInstance(c *gc.C) {
	var env *maasEnviron
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, jc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.ecfg().maasAgentName()})
		},
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
	})
	suite.setupFakeTools(c)
	env = makeEnviron(c)
	params := environs.StartInstanceParams{}
	result, err := testing.StartInstanceWithParams(env, "1", params, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Instance.Id(), gc.Equals, instance.Id("Bruce Sterling"))
}

func (suite *maas2EnvironSuite) TestStartInstanceParams(c *gc.C) {
	var env *maasEnviron
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, jc.DeepEquals, gomaasapi.AllocateMachineArgs{
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
	env = makeEnviron(c)
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
			c.Assert(args, jc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.ecfg().maasAgentName()})
		},
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
	})
	suite.setupFakeTools(c)
	env = makeEnviron(c)

	_, err := env.acquireNode2("", "", constraints.Value{}, nil, nil)

	c.Check(err, jc.ErrorIsNil)
	//	nodeRequestValues, found := requestValues["node0"]
	//	c.Assert(found, jc.IsTrue)
	//	c.Assert(nodeRequestValues[0].Get("agent_name"), gc.Equals, exampleAgentName)
}

func (suite *maas2EnvironSuite) TestAcquireNodePassesPositiveAndNegativeTags(c *gc.C) {
	var env *maasEnviron
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, jc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.ecfg().maasAgentName()})
		},
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
	})
	suite.setupFakeTools(c)
	env = makeEnviron(c)

	_, err := env.acquireNode2(
		"", "",
		constraints.Value{Tags: stringslicep("tag1", "^tag2", "tag3", "^tag4")},
		nil, nil,
	)

	c.Check(err, jc.ErrorIsNil)
	//	nodeValues, found := requestValues["node0"]
	//	c.Assert(found, jc.IsTrue)
	//	c.Assert(nodeValues[0].Get("tags"), gc.Equals, "tag1,tag3")
	//	c.Assert(nodeValues[0].Get("not_tags"), gc.Equals, "tag2,tag4")
}

func (suite *maas2EnvironSuite) TestAcquireNodePassesPositiveAndNegativeSpaces(c *gc.C) {
	var env *maasEnviron
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, jc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.ecfg().maasAgentName()})
		},
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
	})
	suite.setupFakeTools(c)
	env = makeEnviron(c)

	_, err := env.acquireNode2(
		"", "",
		constraints.Value{Spaces: stringslicep("space-1", "^space-2", "space-3", "^space-4")},
		nil, nil,
	)
	c.Check(err, jc.ErrorIsNil)
	//nodeValues, found := requestValues["node0"]
	//c.Assert(found, jc.IsTrue)
	//c.Check(nodeValues[0].Get("interfaces"), gc.Equals, "0:space=2;1:space=4")
	//c.Check(nodeValues[0].Get("not_networks"), gc.Equals, "space:3,space:5")
}

func (suite *maas2EnvironSuite) TestAcquireNodeDisambiguatesNamedLabelsFromIndexedUpToALimit(c *gc.C) {
	var env *maasEnviron
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, jc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.ecfg().maasAgentName()})
		},
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
	})
	suite.setupFakeTools(c)
	env = makeEnviron(c)
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
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, jc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.ecfg().maasAgentName()})
		},
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
	})
	suite.setupFakeTools(c)
	for i, test := range []struct {
		volumes  []volumeInfo
		expected string
	}{{
		volumes:  nil,
		expected: "",
	}, {
		volumes:  []volumeInfo{{"volume-1", 1234, nil}},
		expected: "volume-1:1234",
	}, {
		volumes:  []volumeInfo{{"", 1234, []string{"tag1", "tag2"}}},
		expected: "1234(tag1,tag2)",
	}, {
		volumes:  []volumeInfo{{"volume-1", 1234, []string{"tag1", "tag2"}}},
		expected: "volume-1:1234(tag1,tag2)",
	}, {
		volumes: []volumeInfo{
			{"volume-1", 1234, []string{"tag1", "tag2"}},
			{"volume-2", 4567, []string{"tag1", "tag3"}},
		},
		expected: "volume-1:1234(tag1,tag2),volume-2:4567(tag1,tag3)",
	}} {
		c.Logf("test #%d: volumes=%v", i, test.volumes)
		env = makeEnviron(c)
		_, err := env.acquireNode2("", "", constraints.Value{}, nil, test.volumes)
		c.Check(err, jc.ErrorIsNil)
		//nodeRequestValues, found := requestValues["node0"]
		//if c.Check(found, jc.IsTrue) {
		//	c.Check(nodeRequestValues[0].Get("storage"), gc.Equals, test.expected)
		//}
	}
}

func (suite *maas2EnvironSuite) TestAcquireNodeInterfaces(c *gc.C) {
	var env *maasEnviron
	var getNegatives func() []string
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, jc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.ecfg().maasAgentName(),
				// Should have Interfaces too
				NotNetworks: getNegatives(),
			})
		},
		allocateMachine: &fakeMachine{
			systemID:     "Bruce Sterling",
			architecture: arch.HostArch(),
		},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "foo",
				subnets: []gomaasapi.Subnet{fakeSubnet{id: 99, vlanVid: 66, cidr: "192.168.10.0/24"}},
				id:      5,
			},
			fakeSpace{
				name:    "bar",
				subnets: []gomaasapi.Subnet{fakeSubnet{id: 100, vlanVid: 66, cidr: "192.168.11.0/24"}},
				id:      6,
			},
		},
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
		expectedPositives string
		expectedNegatives string
		expectedError     string
	}{{ // without specified bindings, spaces constraints are used instead.
		interfaces:        nil,
		expectedPositives: "0:space=5",
		expectedNegatives: "space:6",
		expectedError:     "",
	}, {
		interfaces:        []interfaceBinding{{"name-1", "space-1"}},
		expectedPositives: "name-1:space=space-1;0:space=5",
		expectedNegatives: "space:6",
	}, {
		interfaces: []interfaceBinding{
			{"name-1", "1"},
			{"name-2", "2"},
			{"name-3", "3"},
		},
		expectedPositives: "name-1:space=1;name-2:space=2;name-3:space=3;0:space=5",
		expectedNegatives: "space:6",
	}, {
		interfaces:    []interfaceBinding{{"", "anything"}},
		expectedError: "interface bindings cannot have empty names",
	}, {
		interfaces:    []interfaceBinding{{"shared-db", "6"}},
		expectedError: `negative space "bar" from constraints clashes with interface bindings`,
	}, {
		interfaces: []interfaceBinding{
			{"shared-db", "1"},
			{"db", "1"},
		},
		expectedPositives: "shared-db:space=1;db:space=1;0:space=5",
		expectedNegatives: "space:6",
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
		env = makeEnviron(c)
		// TODO (mfoord): need getPositives as well.
		getNegatives = func() []string {
			return strings.Split(test.expectedNegatives, ";")
		}
		_, err := env.acquireNode2("", "", cons, test.interfaces, nil)
		if test.expectedError != "" {
			c.Check(err, gc.ErrorMatches, test.expectedError)
			c.Check(err, jc.Satisfies, errors.IsNotValid)
			continue
		}
		c.Check(err, jc.ErrorIsNil)
		//		nodeRequestValues, found := requestValues["node0"]
		//		if c.Check(found, jc.IsTrue) {
		//			c.Check(nodeRequestValues[0].Get("interfaces"), gc.Equals, test.expectedPositives)
		//			c.Check(nodeRequestValues[0].Get("not_networks"), gc.Equals, test.expectedNegatives)
		//		}
	}
}

func (suite *maas2EnvironSuite) TestAcquireNodeConvertsSpaceNames(c *gc.C) {
	var env *maasEnviron
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, jc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.ecfg().maasAgentName(),
				// Should have Interfaces set
				// Interfaces: 0:space=2,
				NotNetworks: []string{"space:3"},
			})
		},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "foo",
				subnets: []gomaasapi.Subnet{fakeSubnet{id: 99, vlanVid: 66, cidr: "192.168.10.0/24"}},
				id:      2,
			},
			fakeSpace{
				name:    "bar",
				subnets: []gomaasapi.Subnet{fakeSubnet{id: 100, vlanVid: 66, cidr: "192.168.11.0/24"}},
				id:      3,
			},
		},
	})
	env = makeEnviron(c)
	cons := constraints.Value{
		Spaces: stringslicep("foo", "^bar"),
	}
	_, err := env.acquireNode2("", "", cons, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *maas2EnvironSuite) TestAcquireNodeTranslatesSpaceNames(c *gc.C) {
	var env *maasEnviron
	suite.injectController(&fakeController{
		allocateMachineArgsCheck: func(args gomaasapi.AllocateMachineArgs) {
			c.Assert(args, jc.DeepEquals, gomaasapi.AllocateMachineArgs{
				AgentName: env.ecfg().maasAgentName(),
				// Should have Interfaces set
				// Interfaces: 0:space=2,
				NotNetworks: []string{"space:3"},
			})
		},
		spaces: []gomaasapi.Space{
			fakeSpace{
				name:    "foo",
				subnets: []gomaasapi.Subnet{fakeSubnet{id: 99, vlanVid: 66, cidr: "192.168.10.0/24"}},
				id:      2,
			},
			fakeSpace{
				name:    "bar",
				subnets: []gomaasapi.Subnet{fakeSubnet{id: 100, vlanVid: 66, cidr: "192.168.11.0/24"}},
				id:      3,
			},
		},
	})
	env = makeEnviron(c)
	cons := constraints.Value{
		Spaces: stringslicep("foo-1", "^bar-3"),
	}
	_, err := env.acquireNode2("", "", cons, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *maas2EnvironSuite) TestAcquireNodeUnrecognisedSpace(c *gc.C) {
	suite.injectController(&fakeController{})
	env := makeEnviron(c)
	cons := constraints.Value{
		Spaces: stringslicep("baz"),
	}
	_, err := env.acquireNode2("", "", cons, nil, nil)
	c.Assert(err, gc.ErrorMatches, `unrecognised space in constraint "baz"`)
}
