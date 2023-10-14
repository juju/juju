// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/maas"
	coretesting "github.com/juju/juju/testing"
)

type environSuite struct {
	coretesting.BaseSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&environSuite{})

func TestMAAS(t *stdtesting.T) {
	gc.TestingT(t)
}

// TDOO: jam 2013-12-06 This is copied from the providerSuite which is in a
// whitebox package maas. Either move that into a whitebox test so it can be
// shared, or into a 'testing' package so we can use it here.
func (s *environSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *environSuite) SetUpTest(c *gc.C) {
	s.ToolsFixture.SetUpTest(c)

	mockGetController := func(string, string) (gomaasapi.Controller, error) {
		return nil, gomaasapi.NewUnsupportedVersionError("oops")
	}
	s.PatchValue(&maas.GetMAAS2Controller, mockGetController)
}

func (s *environSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *environSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func getSimpleTestConfig(c *gc.C, extraAttrs coretesting.Attrs) *config.Config {
	attrs := coretesting.FakeConfig()
	attrs["type"] = "maas"
	attrs["bootstrap-timeout"] = "1200"
	for k, v := range extraAttrs {
		attrs[k] = v
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

type badEndpointSuite struct {
	coretesting.BaseSuite

	fakeServer *httptest.Server
	cloudSpec  environscloudspec.CloudSpec
}

var _ = gc.Suite(&badEndpointSuite{})

func (s *badEndpointSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	always404 := func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "uh-oh")
	}
	s.fakeServer = httptest.NewServer(http.HandlerFunc(always404))
}

func (s *badEndpointSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	cred := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{
		"maas-oauth": "a:b:c",
	})
	s.cloudSpec = environscloudspec.CloudSpec{
		Type:       "maas",
		Name:       "maas",
		Endpoint:   s.fakeServer.URL,
		Credential: &cred,
	}
}

func (s *badEndpointSuite) TestBadEndpointMessageNoMAAS(c *gc.C) {
	cfg := getSimpleTestConfig(c, coretesting.Attrs{})
	env, err := maas.NewEnviron(s.cloudSpec, cfg, nil)
	c.Assert(env, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `could not connect to MAAS controller - check the endpoint is correct \(it normally ends with /MAAS\)`)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *badEndpointSuite) TestBadEndpointMessageWithMAAS(c *gc.C) {
	cfg := getSimpleTestConfig(c, coretesting.Attrs{})
	s.cloudSpec.Endpoint += "/MAAS"
	env, err := maas.NewEnviron(s.cloudSpec, cfg, nil)
	c.Assert(env, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `could not connect to MAAS controller - check the endpoint is correct`)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *badEndpointSuite) TestBadEndpointMessageWithMAASAndSlash(c *gc.C) {
	cfg := getSimpleTestConfig(c, coretesting.Attrs{})
	s.cloudSpec.Endpoint += "/MAAS/"
	env, err := maas.NewEnviron(s.cloudSpec, cfg, nil)
	c.Assert(env, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `could not connect to MAAS controller - check the endpoint is correct`)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}
