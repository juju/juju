// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"net/http"

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
