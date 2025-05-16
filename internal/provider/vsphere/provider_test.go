// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"errors"
	"net/url"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"golang.org/x/net/context"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

type providerSuite struct {
	ProviderFixture
}

func TestProviderSuite(t *stdtesting.T) { tc.Run(t, &providerSuite{}) }
func (s *providerSuite) TestRegistered(c *tc.C) {
	provider, err := environs.Provider("vsphere")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(provider, tc.NotNil)
}

func (s *providerSuite) TestOpen(c *tc.C) {
	config := fakeConfig(c)
	env, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud:  fakeCloudSpec(),
		Config: config,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)

	envConfig := env.Config()
	c.Assert(envConfig.Name(), tc.Equals, "testmodel")
}

func (s *providerSuite) TestOpenInvalidCloudSpec(c *tc.C) {
	spec := fakeCloudSpec()
	spec.Name = ""
	s.testOpenError(c, spec, `validating cloud spec: cloud name "" not valid`)
}

func (s *providerSuite) TestOpenMissingCredential(c *tc.C) {
	spec := fakeCloudSpec()
	spec.Credential = nil
	s.testOpenError(c, spec, `validating cloud spec: missing credential not valid`)
}

func (s *providerSuite) TestOpenUnsupportedCredential(c *tc.C) {
	credential := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{})
	spec := fakeCloudSpec()
	spec.Credential = &credential
	s.testOpenError(c, spec, `validating cloud spec: "oauth1" auth-type not supported`)
}

func (s *providerSuite) testOpenError(c *tc.C, spec environscloudspec.CloudSpec, expect string) {
	_, err := s.provider.Open(c.Context(), environs.OpenParams{
		Cloud:  spec,
		Config: fakeConfig(c),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorMatches, expect)
}

func (s *providerSuite) TestValidateCloud(c *tc.C) {
	err := s.provider.ValidateCloud(c.Context(), fakeCloudSpec())
	c.Check(err, tc.ErrorIsNil)
}

func (s *providerSuite) TestValidate(c *tc.C) {
	config := fakeConfig(c)
	validCfg, err := s.provider.Validate(c.Context(), config, nil)
	c.Assert(err, tc.ErrorIsNil)

	validAttrs := validCfg.AllAttrs()
	c.Assert(config.AllAttrs(), tc.DeepEquals, validAttrs)
}

func (s *providerSuite) TestSchema(c *tc.C) {
	y := []byte(`
auth-types: [userpass]
endpoint: http://foo.com/vsphere
regions:
  foo: {}
  bar: {}
`[1:])
	var v interface{}
	err := yaml.Unmarshal(y, &v)
	c.Assert(err, tc.ErrorIsNil)
	v, err = utils.ConformYAML(v)
	c.Assert(err, tc.ErrorIsNil)

	err = s.provider.CloudSchema().Validate(v)
	c.Assert(err, tc.ErrorIsNil)
}

type pingSuite struct {
	ProviderFixture
}

func TestPingSuite(t *stdtesting.T) { tc.Run(t, &pingSuite{}) }
func (s *pingSuite) TestPingInvalidHost(c *tc.C) {
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
		err := s.provider.Ping(c.Context(), t)
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

func (s *pingSuite) TestPingInvalidURL(c *tc.C) {
	err := s.provider.Ping(c.Context(), "abc%sdef")
	c.Assert(err, tc.ErrorMatches, "Invalid endpoint format, please give a full url or IP/hostname.")
}

func (s *pingSuite) TestPingInvalidScheme(c *tc.C) {
	err := s.provider.Ping(c.Context(), "gopher://abcdef.com")
	c.Assert(err, tc.ErrorMatches, "Invalid endpoint format, please use an http or https URL.")
}

func (s *pingSuite) TestPingLoginSucceeded(c *tc.C) {
	// This test shows that when - against all odds - the
	// login succeeds, Ping returns nil.

	err := s.provider.Ping(c.Context(), "testing.invalid")
	c.Assert(err, tc.ErrorIsNil)

	s.dialStub.CheckCallNames(c, "Dial")
	call := s.dialStub.Calls()[0]
	c.Assert(call.Args, tc.HasLen, 3)
	c.Assert(call.Args[0], tc.Implements, new(context.Context))
	c.Assert(call.Args[1], tc.DeepEquals, &url.URL{
		Scheme: "https",
		Host:   "testing.invalid",
		Path:   "/sdk",
		User:   url.User("juju"),
	})
	c.Assert(call.Args[2], tc.Equals, "")

	s.client.CheckCallNames(c, "Close")
}
