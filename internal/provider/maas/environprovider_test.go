// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"errors"
	"os"

	"github.com/juju/collections/set"
	"github.com/juju/gomaasapi/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/testing"
)

type EnvironProviderSuite struct {
	maasSuite
}

var _ = gc.Suite(&EnvironProviderSuite{})

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

func (s *EnvironProviderSuite) TestValidateCloud(c *gc.C) {
	err := providerInstance.ValidateCloud(context.Background(), s.cloudSpec())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvironProviderSuite) TestValidateCloudSkipTLSVerify(c *gc.C) {
	cloud := s.cloudSpec()
	cloud.SkipTLSVerify = true
	err := providerInstance.ValidateCloud(context.Background(), cloud)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvironProviderSuite) TestValidateCloudInvalidOAuth(c *gc.C) {
	spec := s.cloudSpec()
	cred := oauthCredential("wrongly-formatted-oauth-string")
	spec.Credential = &cred
	err := providerInstance.ValidateCloud(context.Background(), spec)
	c.Assert(err, gc.ErrorMatches, ".*malformed maas-oauth.*")
}

func (s *EnvironProviderSuite) TestValidateCloudInvalidEndpoint(c *gc.C) {
	spec := s.cloudSpec()
	spec.Endpoint = "This should have been a URL or host."
	err := providerInstance.ValidateCloud(context.Background(), spec)
	c.Assert(err, gc.ErrorMatches,
		`validating cloud spec: validating endpoint: endpoint "This should have been a URL or host." not valid`,
	)
}

// create a temporary file with the given content.  The file will be cleaned
// up at the end of the test calling this method.
func createTempFile(c *gc.C, content []byte) string {
	file, err := os.CreateTemp(c.MkDir(), "")
	defer file.Close()
	c.Assert(err, jc.ErrorIsNil)
	filename := file.Name()
	err = os.WriteFile(filename, content, 0o644)
	c.Assert(err, jc.ErrorIsNil)
	return filename
}

func (s *EnvironProviderSuite) TestOpenReturnsNilInterfaceUponFailure(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{"type": "maas"})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	spec := s.cloudSpec()
	cred := oauthCredential("wrongly-formatted-oauth-string")
	spec.Credential = &cred
	env, err := providerInstance.Open(context.Background(), environs.OpenParams{
		Cloud:  spec,
		Config: config,
	}, environs.NoopCredentialInvalidator())
	// When Open() fails (i.e. returns a non-nil error), it returns an
	// environs.Environ interface object with a nil value and a nil
	// type.
	c.Check(env, gc.Equals, nil)
	c.Check(err, gc.ErrorMatches, ".*malformed maas-oauth.*")
}

func (s *EnvironProviderSuite) TestSchema(c *gc.C) {
	y := []byte(`
auth-types: [oauth1]
endpoint: http://foo.com/openstack
`[1:])
	var v interface{}
	err := yaml.Unmarshal(y, &v)
	c.Assert(err, jc.ErrorIsNil)
	v, err = utils.ConformYAML(v)
	c.Assert(err, jc.ErrorIsNil)

	p, err := environs.Provider("maas")
	c.Assert(err, jc.ErrorIsNil)
	err = p.CloudSchema().Validate(v)
	c.Assert(err, jc.ErrorIsNil)
}

type MaasPingSuite struct {
	testing.BaseSuite

	callCtx envcontext.ProviderCallContext
}

var _ = gc.Suite(&MaasPingSuite{})

func (s *MaasPingSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = envcontext.WithoutCredentialInvalidator(context.Background())
}

func (s *MaasPingSuite) TestPingNoEndpoint(c *gc.C) {
	endpoint := "https://foo.com/MAAS"
	var serverURLs []string
	err := ping(c, s.callCtx, endpoint,
		func(ctx context.Context, client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			serverURLs = append(serverURLs, client.URL().String())
			c.Assert(serverURL, gc.Equals, endpoint)
			return nil, errors.New("nope")
		},
	)
	c.Assert(err, gc.ErrorMatches, "No MAAS server running at "+endpoint+": nope")
	c.Assert(serverURLs, gc.DeepEquals, []string{
		"https://foo.com/MAAS/api/2.0/",
	})
}

func (s *MaasPingSuite) TestPingOK(c *gc.C) {
	endpoint := "https://foo.com/MAAS"
	var serverURLs []string
	err := ping(c, s.callCtx, endpoint,
		func(ctx context.Context, client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			serverURLs = append(serverURLs, client.URL().String())
			c.Assert(serverURL, gc.Equals, endpoint)
			return set.NewStrings("network-deployment-ubuntu"), nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverURLs, gc.DeepEquals, []string{
		"https://foo.com/MAAS/api/2.0/",
	})
}

func (s *MaasPingSuite) TestPingVersionURLOK(c *gc.C) {
	endpoint := "https://foo.com/MAAS/api/10.1/"
	var serverURLs []string
	err := ping(c, s.callCtx, endpoint,
		func(ctx context.Context, client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			serverURLs = append(serverURLs, client.URL().String())
			c.Assert(serverURL, gc.Equals, "https://foo.com/MAAS/")
			return set.NewStrings("network-deployment-ubuntu"), nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverURLs, gc.DeepEquals, []string{
		"https://foo.com/MAAS/api/10.1/",
	})
}

func (s *MaasPingSuite) TestPingVersionURLBad(c *gc.C) {
	endpoint := "https://foo.com/MAAS/api/10.1/"
	var serverURLs []string
	err := ping(c, s.callCtx, endpoint,
		func(ctx context.Context, client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			serverURLs = append(serverURLs, client.URL().String())
			c.Assert(serverURL, gc.Equals, "https://foo.com/MAAS/")
			return nil, errors.New("nope")
		},
	)
	c.Assert(err, gc.ErrorMatches, "No MAAS server running at "+endpoint+": nope")
	c.Assert(serverURLs, gc.DeepEquals, []string{
		"https://foo.com/MAAS/api/10.1/",
	})
}

func ping(c *gc.C, callCtx envcontext.ProviderCallContext, endpoint string, getCapabilities Capabilities) error {
	p, err := environs.Provider("maas")
	c.Assert(err, jc.ErrorIsNil)
	m, ok := p.(EnvironProvider)
	c.Assert(ok, jc.IsTrue)
	m.GetCapabilities = getCapabilities
	return m.Ping(callCtx, endpoint)
}
