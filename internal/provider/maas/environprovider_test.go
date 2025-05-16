// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"errors"
	"os"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/gomaasapi/v2"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
)

type EnvironProviderSuite struct {
	maasSuite
}

func TestEnvironProviderSuite(t *stdtesting.T) { tc.Run(t, &EnvironProviderSuite{}) }
func (s *EnvironProviderSuite) cloudSpec() environscloudspec.CloudSpec {
	credential := oauthCredential("aa:bb:cc")
	return environscloudspec.CloudSpec{
		Type:       "maas",
		Name:       "maas",
		Endpoint:   "http://maas.testing.invalid/maas/",
		Credential: &credential,
	}
}

func oauthCredential(token string) cloud.Credential {
	return cloud.NewCredential(
		cloud.OAuth1AuthType,
		map[string]string{
			"maas-oauth": token,
		},
	)
}

func (s *EnvironProviderSuite) TestValidateCloud(c *tc.C) {
	err := providerInstance.ValidateCloud(c.Context(), s.cloudSpec())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *EnvironProviderSuite) TestValidateCloudSkipTLSVerify(c *tc.C) {
	cloud := s.cloudSpec()
	cloud.SkipTLSVerify = true
	err := providerInstance.ValidateCloud(c.Context(), cloud)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *EnvironProviderSuite) TestValidateCloudInvalidOAuth(c *tc.C) {
	spec := s.cloudSpec()
	cred := oauthCredential("wrongly-formatted-oauth-string")
	spec.Credential = &cred
	err := providerInstance.ValidateCloud(c.Context(), spec)
	c.Assert(err, tc.ErrorMatches, ".*malformed maas-oauth.*")
}

func (s *EnvironProviderSuite) TestValidateCloudInvalidEndpoint(c *tc.C) {
	spec := s.cloudSpec()
	spec.Endpoint = "This should have been a URL or host."
	err := providerInstance.ValidateCloud(c.Context(), spec)
	c.Assert(err, tc.ErrorMatches,
		`validating cloud spec: validating endpoint: endpoint "This should have been a URL or host." not valid`,
	)
}

// create a temporary file with the given content.  The file will be cleaned
// up at the end of the test calling this method.
func createTempFile(c *tc.C, content []byte) string {
	file, err := os.CreateTemp(c.MkDir(), "")
	defer file.Close()
	c.Assert(err, tc.ErrorIsNil)
	filename := file.Name()
	err = os.WriteFile(filename, content, 0o644)
	c.Assert(err, tc.ErrorIsNil)
	return filename
}

func (s *EnvironProviderSuite) TestOpenReturnsNilInterfaceUponFailure(c *tc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{"type": "maas"})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)
	spec := s.cloudSpec()
	cred := oauthCredential("wrongly-formatted-oauth-string")
	spec.Credential = &cred
	env, err := providerInstance.Open(c.Context(), environs.OpenParams{
		Cloud:  spec,
		Config: config,
	}, environs.NoopCredentialInvalidator())
	// When Open() fails (i.e. returns a non-nil error), it returns an
	// environs.Environ interface object with a nil value and a nil
	// type.
	c.Check(env, tc.Equals, nil)
	c.Check(err, tc.ErrorMatches, ".*malformed maas-oauth.*")
}

func (s *EnvironProviderSuite) TestSchema(c *tc.C) {
	y := []byte(`
auth-types: [oauth1]
endpoint: http://foo.com/openstack
`[1:])
	var v interface{}
	err := yaml.Unmarshal(y, &v)
	c.Assert(err, tc.ErrorIsNil)
	v, err = utils.ConformYAML(v)
	c.Assert(err, tc.ErrorIsNil)

	p, err := environs.Provider("maas")
	c.Assert(err, tc.ErrorIsNil)
	err = p.CloudSchema().Validate(v)
	c.Assert(err, tc.ErrorIsNil)
}

type MaasPingSuite struct {
	testing.BaseSuite
}

func TestMaasPingSuite(t *stdtesting.T) { tc.Run(t, &MaasPingSuite{}) }
func (s *MaasPingSuite) TestPingNoEndpoint(c *tc.C) {
	endpoint := "https://foo.com/MAAS"
	var serverURLs []string
	err := ping(c, endpoint,
		func(ctx context.Context, client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			serverURLs = append(serverURLs, client.URL().String())
			c.Assert(serverURL, tc.Equals, endpoint)
			return nil, errors.New("nope")
		},
	)
	c.Assert(err, tc.ErrorMatches, "No MAAS server running at "+endpoint+": nope")
	c.Assert(serverURLs, tc.DeepEquals, []string{
		"https://foo.com/MAAS/api/2.0/",
	})
}

func (s *MaasPingSuite) TestPingOK(c *tc.C) {
	endpoint := "https://foo.com/MAAS"
	var serverURLs []string
	err := ping(c, endpoint,
		func(ctx context.Context, client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			serverURLs = append(serverURLs, client.URL().String())
			c.Assert(serverURL, tc.Equals, endpoint)
			return set.NewStrings("network-deployment-ubuntu"), nil
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(serverURLs, tc.DeepEquals, []string{
		"https://foo.com/MAAS/api/2.0/",
	})
}

func (s *MaasPingSuite) TestPingVersionURLOK(c *tc.C) {
	endpoint := "https://foo.com/MAAS/api/10.1/"
	var serverURLs []string
	err := ping(c, endpoint,
		func(ctx context.Context, client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			serverURLs = append(serverURLs, client.URL().String())
			c.Assert(serverURL, tc.Equals, "https://foo.com/MAAS/")
			return set.NewStrings("network-deployment-ubuntu"), nil
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(serverURLs, tc.DeepEquals, []string{
		"https://foo.com/MAAS/api/10.1/",
	})
}

func (s *MaasPingSuite) TestPingVersionURLBad(c *tc.C) {
	endpoint := "https://foo.com/MAAS/api/10.1/"
	var serverURLs []string
	err := ping(c, endpoint,
		func(ctx context.Context, client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			serverURLs = append(serverURLs, client.URL().String())
			c.Assert(serverURL, tc.Equals, "https://foo.com/MAAS/")
			return nil, errors.New("nope")
		},
	)
	c.Assert(err, tc.ErrorMatches, "No MAAS server running at "+endpoint+": nope")
	c.Assert(serverURLs, tc.DeepEquals, []string{
		"https://foo.com/MAAS/api/10.1/",
	})
}

func ping(c *tc.C, endpoint string, getCapabilities Capabilities) error {
	p, err := environs.Provider("maas")
	c.Assert(err, tc.ErrorIsNil)
	m, ok := p.(EnvironProvider)
	c.Assert(ok, tc.IsTrue)
	m.GetCapabilities = getCapabilities
	return m.Ping(c.Context(), endpoint)
}
