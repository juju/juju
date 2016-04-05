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

	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
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
	mockGetController := func(maasServer, apiKey string) (gomaasapi.Controller, error) {
		controller := fakeController{
			bootResources: []gomaasapi.BootResource{
				&fakeBootResource{name: "wily", architecture: "amd64/blah"},
				&fakeBootResource{name: "wily", architecture: "amd64/something"},
				&fakeBootResource{name: "xenial", architecture: "arm/somethingelse"},
			},
		}
		return &controller, nil
	}
	suite.PatchValue(&GetMAAS2Controller, mockGetController)
	env := makeEnviron(c)
	result, err := env.SupportedArchitectures()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []string{"amd64", "arm"})
}

func (suite *maas2EnvironSuite) TestSupportedArchitecturesError(c *gc.C) {
	mockGetController := func(maasServer, apiKey string) (gomaasapi.Controller, error) {
		return &fakeController{bootResourcesError: errors.New("Something terrible!")}, nil
	}
	suite.PatchValue(&GetMAAS2Controller, mockGetController)
	env := makeEnviron(c)
	_, err := env.SupportedArchitectures()
	c.Assert(err, gc.ErrorMatches, "Something terrible!")
}

func (suite *maas2EnvironSuite) TestAllInstances(c *gc.C) {
	var env *maasEnviron
	mockGetController := func(maasServer, apiKey string) (gomaasapi.Controller, error) {
		checkArgs := func(args gomaasapi.MachinesArgs) {
			c.Check(args.SystemIDs, jc.DeepEquals, []string{})
			c.Check(args.AgentName, gc.Equals, env.ecfg().maasAgentName())
		}
		return &fakeController{
			machines: []gomaasapi.Machine{
				&fakeMachine{systemID: "tuco"},
				&fakeMachine{systemID: "tio"},
				&fakeMachine{systemID: "gus"},
			},
			machinesArgsCheck: checkArgs,
		}, nil
	}
	suite.PatchValue(&GetMAAS2Controller, mockGetController)
	env = makeEnviron(c)
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
	mockGetController := func(maasServer, apiKey string) (gomaasapi.Controller, error) {
		return &fakeController{machinesError: errors.New("Something terrible!")}, nil
	}
	suite.PatchValue(&GetMAAS2Controller, mockGetController)
	env := makeEnviron(c)
	_, err := env.AllInstances()
	c.Assert(err, gc.ErrorMatches, "Something terrible!")
}
