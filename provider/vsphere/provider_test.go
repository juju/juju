// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/vsphere"
)

type providerSuite struct {
	vsphere.BaseSuite

	provider environs.EnvironProvider
	spec     environs.CloudSpec
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	var err error
	s.provider, err = environs.Provider("vsphere")
	c.Check(err, jc.ErrorIsNil)
	s.spec = vsphere.FakeCloudSpec()
}

func (s *providerSuite) TestRegistered(c *gc.C) {
	c.Assert(s.provider, gc.Equals, vsphere.Provider)
}

func (s *providerSuite) TestOpen(c *gc.C) {
	env, err := s.provider.Open(environs.OpenParams{
		Cloud:  s.spec,
		Config: s.Config,
	})
	c.Check(err, jc.ErrorIsNil)

	envConfig := env.Config()
	c.Assert(envConfig.Name(), gc.Equals, "testenv")
}

func (s *providerSuite) TestOpenInvalidCloudSpec(c *gc.C) {
	s.spec.Name = ""
	s.testOpenError(c, s.spec, `validating cloud spec: cloud name "" not valid`)
}

func (s *providerSuite) TestOpenMissingCredential(c *gc.C) {
	s.spec.Credential = nil
	s.testOpenError(c, s.spec, `validating cloud spec: missing credential not valid`)
}

func (s *providerSuite) TestOpenUnsupportedCredential(c *gc.C) {
	credential := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{})
	s.spec.Credential = &credential
	s.testOpenError(c, s.spec, `validating cloud spec: "oauth1" auth-type not supported`)
}

func (s *providerSuite) testOpenError(c *gc.C, spec environs.CloudSpec, expect string) {
	_, err := s.provider.Open(environs.OpenParams{
		Cloud:  spec,
		Config: s.Config,
	})
	c.Assert(err, gc.ErrorMatches, expect)
}

func (s *providerSuite) TestPrepareConfig(c *gc.C) {
	cfg, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Config: s.Config,
		Cloud:  s.spec,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cfg, gc.NotNil)
}

func (s *providerSuite) TestValidate(c *gc.C) {
	validCfg, err := s.provider.Validate(s.Config, nil)
	c.Check(err, jc.ErrorIsNil)

	validAttrs := validCfg.AllAttrs()
	c.Assert(s.Config.AllAttrs(), gc.DeepEquals, validAttrs)
}
func (s *providerSuite) TestSchema(c *gc.C) {
	y := []byte(`
auth-types: [userpass]
endpoint: http://foo.com/vsphere
regions:
  foo: {}
  bar: {}
`[1:])
	var v interface{}
	err := yaml.Unmarshal(y, &v)
	c.Assert(err, jc.ErrorIsNil)
	v, err = utils.ConformYAML(v)
	c.Assert(err, jc.ErrorIsNil)

	p, err := environs.Provider("vsphere")
	err = p.CloudSchema().Validate(v)
	c.Assert(err, jc.ErrorIsNil)
}

type pingSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(pingSuite{})

func (pingSuite) TestPingInvalidHost(c *gc.C) {
	tests := []string{
		"foo.com",
		"http://foo.test",
		"http://foo.test:77",
	}

	provider, err := environs.Provider("vsphere")
	c.Assert(err, jc.ErrorIsNil)

	for _, t := range tests {
		err := provider.Ping(t)
		if err == nil {
			c.Errorf("ping %q: expected error, but got nil.", t)
			continue
		}
		expected := "No VSphere server running at " + t
		if err.Error() != expected {
			c.Errorf("ping %q: expected %q got %v", t, expected, err)
		}
	}
}

func (pingSuite) TestPingInvalidURL(c *gc.C) {
	provider, err := environs.Provider("vsphere")
	c.Assert(err, jc.ErrorIsNil)

	err = provider.Ping("abc%sdef")
	c.Assert(err, gc.ErrorMatches, "Invalid endpoint format, please give a full url or IP/hostname.")
}

func (pingSuite) TestPingInvalidScheme(c *gc.C) {
	provider, err := environs.Provider("vsphere")
	c.Assert(err, jc.ErrorIsNil)

	err = provider.Ping("gopher://abcdef.com")
	c.Assert(err, gc.ErrorMatches, "Invalid endpoint format, please use an http or https URL.")
}

func (pingSuite) TestPingNoEndpoint(c *gc.C) {
	server := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer server.Close()

	provider, err := environs.Provider("vsphere")
	c.Assert(err, jc.ErrorIsNil)

	err = provider.Ping(server.URL)
	c.Assert(err, gc.ErrorMatches, "No VSphere server running at "+server.URL)
}

func (pingSuite) TestPingInvalidResponse(c *gc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Hi!")
	}))
	defer server.Close()
	provider, err := environs.Provider("vsphere")
	c.Assert(err, jc.ErrorIsNil)

	err = provider.Ping(server.URL)
	c.Assert(err, gc.ErrorMatches, "No VSphere server running at "+server.URL)
}
