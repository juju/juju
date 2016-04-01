// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
)

type fakeController struct {
	gomaasapi.Controller
	bootResources      []gomaasapi.BootResource
	bootResourcesError error
}

func (c *fakeController) BootResources() ([]gomaasapi.BootResource, error) {
	if c.bootResourcesError != nil {
		return nil, c.bootResourcesError
	}
	return c.bootResources, nil
}

type fakeBootResource struct {
	gomaasapi.BootResource
	name         string
	architecture string
}

func (r *fakeBootResource) Name() string {
	return r.name
}

func (r *fakeBootResource) Architecture() string {
	return r.architecture
}

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
	testServer.AddResponse("/api/2.0/version/", http.StatusOK, maas2VersionResponse)
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
