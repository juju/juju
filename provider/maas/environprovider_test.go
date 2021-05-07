// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/gomaasapi/v2"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/testing"
)

type EnvironProviderSuite struct {
	providerSuite
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

func (suite *EnvironProviderSuite) TestPrepareConfig(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{"type": "maas"})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	_, err = providerInstance.PrepareConfig(environs.PrepareConfigParams{
		Config: config,
		Cloud:  suite.cloudSpec(),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *EnvironProviderSuite) TestPrepareConfigInvalidOAuth(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{"type": "maas"})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	spec := suite.cloudSpec()
	cred := oauthCredential("wrongly-formatted-oauth-string")
	spec.Credential = &cred
	_, err = providerInstance.PrepareConfig(environs.PrepareConfigParams{
		Config: config,
		Cloud:  spec,
	})
	c.Assert(err, gc.ErrorMatches, ".*malformed maas-oauth.*")
}

func (suite *EnvironProviderSuite) TestPrepareConfigInvalidEndpoint(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{"type": "maas"})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	spec := suite.cloudSpec()
	spec.Endpoint = "This should have been a URL or host."
	_, err = providerInstance.PrepareConfig(environs.PrepareConfigParams{
		Config: config,
		Cloud:  spec,
	})
	c.Assert(err, gc.ErrorMatches,
		`validating cloud spec: validating endpoint: endpoint "This should have been a URL or host." not valid`,
	)
}

func (suite *EnvironProviderSuite) TestPrepareConfigSetsDefaults(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{"type": "maas"})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := providerInstance.PrepareConfig(environs.PrepareConfigParams{
		Config: config,
		Cloud:  suite.cloudSpec(),
	})
	c.Assert(err, jc.ErrorIsNil)
	src, _ := cfg.StorageDefaultBlockSource()
	c.Assert(src, gc.Equals, "maas")
}

func (suite *EnvironProviderSuite) TestMAASServerFromEndpointURL(c *gc.C) {
	suite.testMAASServerFromEndpoint(c, suite.testMAASObject.TestServer.URL)
}

func (suite *EnvironProviderSuite) TestMAASServerFromEndpointHost(c *gc.C) {
	targetURL, err := url.Parse(suite.testMAASObject.TestServer.URL)
	c.Assert(err, jc.ErrorIsNil)

	rp := httputil.NewSingleHostReverseProxy(targetURL)
	rp.Director = func(req *http.Request) {
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/MAAS")
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
	}
	proxy := httptest.NewServer(rp)
	defer proxy.Close()

	// The proxy's host:port will be formatted into a URL, with a
	// fixed root path of "/MAAS".
	proxyURL, err := url.Parse(proxy.URL)
	c.Assert(err, jc.ErrorIsNil)
	suite.testMAASServerFromEndpoint(c, proxyURL.Host)
}

func (suite *EnvironProviderSuite) testMAASServerFromEndpoint(c *gc.C, endpoint string) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{"type": "maas"})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	cloudSpec := suite.cloudSpec()
	cloudSpec.Endpoint = endpoint
	env, err := providerInstance.Open(environs.OpenParams{
		Config: config,
		Cloud:  cloudSpec,
	})
	c.Assert(err, jc.ErrorIsNil)

	suite.addNode(`{"system_id":"test-allocated"}`)
	_, err = env.AllRunningInstances(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
}

// create a temporary file with the given content.  The file will be cleaned
// up at the end of the test calling this method.
func createTempFile(c *gc.C, content []byte) string {
	file, err := ioutil.TempFile(c.MkDir(), "")
	defer file.Close()
	c.Assert(err, jc.ErrorIsNil)
	filename := file.Name()
	err = ioutil.WriteFile(filename, content, 0644)
	c.Assert(err, jc.ErrorIsNil)
	return filename
}

func (suite *EnvironProviderSuite) TestOpenReturnsNilInterfaceUponFailure(c *gc.C) {
	attrs := testing.FakeConfig().Merge(testing.Attrs{"type": "maas"})
	config, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	spec := suite.cloudSpec()
	cred := oauthCredential("wrongly-formatted-oauth-string")
	spec.Credential = &cred
	env, err := providerInstance.Open(environs.OpenParams{
		Cloud:  spec,
		Config: config,
	})
	// When Open() fails (i.e. returns a non-nil error), it returns an
	// environs.Environ interface object with a nil value and a nil
	// type.
	c.Check(env, gc.Equals, nil)
	c.Check(err, gc.ErrorMatches, ".*malformed maas-oauth.*")
}

func (suite *EnvironProviderSuite) TestSchema(c *gc.C) {
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

	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&MaasPingSuite{})

func (s *MaasPingSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = context.NewEmptyCloudCallContext()
}

func (s *MaasPingSuite) TestPingNoEndpoint(c *gc.C) {
	endpoint := "https://foo.com/MAAS"
	var serverURLs []string
	err := ping(c, s.callCtx, endpoint,
		func(client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			serverURLs = append(serverURLs, client.URL().String())
			c.Assert(serverURL, gc.Equals, endpoint)
			return nil, errors.New("nope")
		},
	)
	c.Assert(err, gc.ErrorMatches, "No MAAS server running at "+endpoint)
	c.Assert(serverURLs, gc.DeepEquals, []string{
		"https://foo.com/MAAS/api/2.0/",
		"https://foo.com/MAAS/api/1.0/",
	})
}

func (s *MaasPingSuite) TestPingOK(c *gc.C) {
	endpoint := "https://foo.com/MAAS"
	var serverURLs []string
	err := ping(c, s.callCtx, endpoint,
		func(client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
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
		func(client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
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
		func(client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			serverURLs = append(serverURLs, client.URL().String())
			c.Assert(serverURL, gc.Equals, "https://foo.com/MAAS/")
			return nil, errors.New("nope")
		},
	)
	c.Assert(err, gc.ErrorMatches, "No MAAS server running at "+endpoint)
	c.Assert(serverURLs, gc.DeepEquals, []string{
		"https://foo.com/MAAS/api/10.1/",
	})
}

func ping(c *gc.C, callCtx context.ProviderCallContext, endpoint string, getCapabilities MaasCapabilities) error {
	p, err := environs.Provider("maas")
	c.Assert(err, jc.ErrorIsNil)
	m, ok := p.(MaasEnvironProvider)
	c.Assert(ok, jc.IsTrue)
	m.GetCapabilities = getCapabilities
	return m.Ping(callCtx, endpoint)
}
