// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
)

type maas2EnvironSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&maas2EnvironSuite{})

func (suite *maas2EnvironSuite) injectController(controller gomaasapi.Controller) {
	mockGetController := func(maasServer, apiKey string) (gomaasapi.Controller, error) {
		return controller, nil
	}
	suite.PatchValue(&GetMAAS2Controller, mockGetController)
}

func (suite *maas2EnvironSuite) makeEnviron(c *gc.C, controller gomaasapi.Controller) *maasEnviron {
	if controller != nil {
		suite.injectController(controller)
	}
	testAttrs := coretesting.Attrs{}
	for k, v := range maasEnvAttrs {
		testAttrs[k] = v
	}
	testAttrs["maas-server"] = "http://any-old-junk.invalid/"
	attrs := coretesting.FakeConfig().Merge(testAttrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	env, err := NewEnviron(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.NotNil)
	return env
}

func (suite *maas2EnvironSuite) TestNewEnvironWithController(c *gc.C) {
	testServer := gomaasapi.NewSimpleServer()
	testServer.AddGetResponse("/api/2.0/version/", http.StatusOK, maas2VersionResponse)
	testServer.AddGetResponse("/api/2.0/users/?op=whoami", http.StatusOK, "{}")
	testServer.Start()
	defer testServer.Close()
	testAttrs := coretesting.Attrs{}
	for k, v := range maasEnvAttrs {
		testAttrs[k] = v
	}
	testAttrs["maas-server"] = testServer.Server.URL
	attrs := coretesting.FakeConfig().Merge(testAttrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	env, err := NewEnviron(cfg)
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
	c.Assert(result, jc.DeepEquals, []string{"amd64", "arm"})
}

func (suite *maas2EnvironSuite) TestSupportedArchitecturesError(c *gc.C) {
	env := suite.makeEnviron(c, &fakeController{bootResourcesError: errors.New("Something terrible!")})
	_, err := env.SupportedArchitectures()
	c.Assert(err, gc.ErrorMatches, "Something terrible!")
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
	c.Assert(actualMachines, jc.DeepEquals, expectedMachines)
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
	c.Assert(actualZones, jc.DeepEquals, expectedZones)
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
					fakeSubnet{id: 99, vlanVid: 66, cidr: "192.168.10.0/24"},
					fakeSubnet{id: 98, vlanVid: 67, cidr: "192.168.11.0/24"},
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
	err := suite.makeEnviron(c, &fakeController{}).StopInstances()
	c.Check(err, jc.ErrorIsNil)
	c.Fail()
}

func (suite *maas2EnvironSuite) TestStopInstancesStopsAndReleasesInstances(c *gc.C) {
	// mark test1 and test2 as being allocated, but not test3.
	// The release operation will ignore test3.
	err := suite.makeEnviron(c, &fakeController{}).StopInstances("test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)
	c.Fail()
}

func (suite *maas2EnvironSuite) TestStopInstancesIgnoresConflict(c *gc.C) {
	env := suite.makeEnviron(c, &fakeController{})
	err := env.StopInstances("test1")
	c.Assert(err, jc.ErrorIsNil)
	c.Fail()
}

func (suite *maas2EnvironSuite) TestStopInstancesIgnoresMissingNodeAndRecurses(c *gc.C) {
	env := suite.makeEnviron(c, &fakeController{})
	err := env.StopInstances("test1", "test2")
	c.Assert(err, jc.ErrorIsNil)
	c.Fail()
}

func (suite *maas2EnvironSuite) TestStopInstancesReturnsUnexpectedMAASError(c *gc.C) {
	env := suite.makeEnviron(c, &fakeController{})
	err := env.StopInstances("test1")
	c.Assert(err, gc.NotNil)
	c.Fail()
}

func (suite *maas2EnvironSuite) TestStopInstancesReturnsUnexpectedError(c *gc.C) {
	env := suite.makeEnviron(c, &fakeController{})
	err := env.StopInstances("test1")
	c.Assert(err, gc.NotNil)
	c.Assert(errors.Cause(err), gc.Equals, environs.ErrNoInstances)
}
