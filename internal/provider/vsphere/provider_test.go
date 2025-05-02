// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	stdcontext "context"
	"errors"
	"net/url"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

type providerSuite struct {
	ProviderFixture
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) TestRegistered(c *gc.C) {
	provider, err := environs.Provider("vsphere")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider, gc.NotNil)
}

func (s *providerSuite) TestOpen(c *gc.C) {
	config := fakeConfig(c)
	env, err := s.provider.Open(stdcontext.Background(), environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: config,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, jc.ErrorIsNil)

	envConfig := env.Config()
	c.Assert(envConfig.Name(), gc.Equals, "testmodel")
}

func (s *providerSuite) TestOpenInvalidCloudSpec(c *gc.C) {
	spec := fakeCloudSpec()
	spec.Name = ""
	s.testOpenError(c, spec, `validating cloud spec: cloud name "" not valid`)
}

func (s *providerSuite) TestOpenMissingCredential(c *gc.C) {
	spec := fakeCloudSpec()
	spec.Credential = nil
	s.testOpenError(c, spec, `validating cloud spec: missing credential not valid`)
}

func (s *providerSuite) TestOpenUnsupportedCredential(c *gc.C) {
	credential := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{})
	spec := fakeCloudSpec()
	spec.Credential = &credential
	s.testOpenError(c, spec, `validating cloud spec: "oauth1" auth-type not supported`)
}

func (s *providerSuite) testOpenError(c *gc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := s.provider.Open(stdcontext.Background(), environs.OpenParams{
		Cloud:  spec,
		Config: fakeConfig(c),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, gc.ErrorMatches, expect)
}

func (s *providerSuite) TestValidateCloud(c *gc.C) {
	err := s.provider.ValidateCloud(context.Background(), fakeCloudSpec())
	c.Check(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestValidate(c *gc.C) {
	config := fakeConfig(c)
	validCfg, err := s.provider.Validate(context.Background(), config, nil)
	c.Assert(err, jc.ErrorIsNil)

	validAttrs := validCfg.AllAttrs()
	c.Assert(config.AllAttrs(), gc.DeepEquals, validAttrs)
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

	err = s.provider.CloudSchema().Validate(v)
	c.Assert(err, jc.ErrorIsNil)
}

type pingSuite struct {
	ProviderFixture
}

var _ = gc.Suite(&pingSuite{})

func (s *pingSuite) TestPingInvalidHost(c *gc.C) {
	s.dialStub.SetErrors(
		errors.New("foo"),
		errors.New("bar"),
		errors.New("baz"),
	)
	tests := []string{
		"foo.com",
		"http://foo.test",
		"http://foo.test:77",
	}
	for _, t := range tests {
		err := s.provider.Ping(context.Background(), t)
		if err == nil {
			c.Errorf("ping %q: expected error, but got nil.", t)
			continue
		}
		expected := "No vCenter/ESXi available at " + t
		if err.Error() != expected {
			c.Errorf("ping %q: expected %q got %v", t, expected, err)
		}
	}
}

func (s *pingSuite) TestPingInvalidURL(c *gc.C) {
	err := s.provider.Ping(context.Background(), "abc%sdef")
	c.Assert(err, gc.ErrorMatches, "Invalid endpoint format, please give a full url or IP/hostname.")
}

func (s *pingSuite) TestPingInvalidScheme(c *gc.C) {
	err := s.provider.Ping(context.Background(), "gopher://abcdef.com")
	c.Assert(err, gc.ErrorMatches, "Invalid endpoint format, please use an http or https URL.")
}

func (s *pingSuite) TestPingLoginSucceeded(c *gc.C) {
	// This test shows that when - against all odds - the
	// login succeeds, Ping returns nil.

	err := s.provider.Ping(context.Background(), "testing.invalid")
	c.Assert(err, jc.ErrorIsNil)

	s.dialStub.CheckCallNames(c, "Dial")
	call := s.dialStub.Calls()[0]
	c.Assert(call.Args, gc.HasLen, 3)
	c.Assert(call.Args[0], gc.Implements, new(context.Context))
	c.Assert(call.Args[1], jc.DeepEquals, &url.URL{
		Scheme: "https",
		Host:   "testing.invalid",
		Path:   "/sdk",
		User:   url.User("juju"),
	})
	c.Assert(call.Args[2], gc.Equals, "")

	s.client.CheckCallNames(c, "Close")
}
