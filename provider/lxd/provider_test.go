// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"net/http"
	"net/http/httptest"

	gomock "github.com/golang/mock/gomock"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	containerLXD "github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/provider/lxd/lxdnames"
)

var (
	_ = gc.Suite(&providerSuite{})
	_ = gc.Suite(&ProviderFunctionalSuite{})
)

type providerSuite struct {
	lxd.BaseSuite

	provider environs.EnvironProvider
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

type providerSuiteDeps struct {
	provider     environs.EnvironProvider
	creds        *testing.MockProviderCredentials
	factory      *lxd.MockServerFactory
	configReader *lxd.MockLXCConfigReader
}

func (s *providerSuite) createProvider(ctrl *gomock.Controller) providerSuiteDeps {
	creds := testing.NewMockProviderCredentials(ctrl)
	factory := lxd.NewMockServerFactory(ctrl)
	configReader := lxd.NewMockLXCConfigReader(ctrl)

	provider := lxd.NewProviderWithMocks(creds, factory, configReader)
	return providerSuiteDeps{
		provider:     provider,
		creds:        creds,
		factory:      factory,
		configReader: configReader,
	}
}

func (s *providerSuite) TestDetectClouds(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	deps.configReader.EXPECT().ReadConfig(".config/lxc/config.yml").Return(lxd.LXCConfig{}, nil)

	cloudDetector := deps.provider.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 1)
	s.assertLocalhostCloud(c, clouds[0])
}

func (s *providerSuite) TestDetectCloud(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	cloudDetector := deps.provider.(environs.CloudDetector)

	cloud, err := cloudDetector.DetectCloud("localhost")
	c.Assert(err, jc.ErrorIsNil)
	s.assertLocalhostCloud(c, cloud)
	cloud, err = cloudDetector.DetectCloud("lxd")
	c.Assert(err, jc.ErrorIsNil)
	s.assertLocalhostCloud(c, cloud)
}

func (s *providerSuite) TestDetectCloudError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	deps.configReader.EXPECT().ReadConfig(".config/lxc/config.yml").Return(lxd.LXCConfig{}, errors.New("bad"))

	cloudDetector := deps.provider.(environs.CloudDetector)

	_, err := cloudDetector.DetectCloud("foo")
	c.Assert(err, gc.ErrorMatches, `cloud foo not found`)
}

func (s *providerSuite) assertLocalhostCloud(c *gc.C, found cloud.Cloud) {
	c.Assert(found, jc.DeepEquals, cloud.Cloud{
		Name: "localhost",
		Type: "lxd",
		AuthTypes: []cloud.AuthType{
			cloud.CertificateAuthType,
			"interactive",
		},
		Regions: []cloud.Region{{
			Name: "localhost",
		}},
		Description: "LXD Container Hypervisor",
	})
}

func (s *providerSuite) TestFinalizeCloud(c *gc.C) {
	c.Skip("To be rewritten during LXD code refactoring for cluster support")

	in := cloud.Cloud{
		Name:      "foo",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions: []cloud.Region{{
			Name: "bar",
		}},
	}

	var ctx mockContext
	out, err := s.Provider.FinalizeCloud(&ctx, in)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(out, jc.DeepEquals, cloud.Cloud{
		Name:      "foo",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Endpoint:  "1.2.3.4:1234",
		Regions: []cloud.Region{{
			Name:     "bar",
			Endpoint: "1.2.3.4:1234",
		}},
	})
	ctx.CheckCallNames(c, "Verbosef")
	ctx.CheckCall(
		c, 0, "Verbosef", "Resolved LXD host address on bridge %s: %s",
		[]interface{}{"test-bridge", "1.2.3.4:1234"},
	)

	// Finalizing a CloudSpec with an empty endpoint involves
	// configuring the local LXD to listen for HTTPS.
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"DefaultProfileBridgeName", nil},
		{"InterfaceAddress", []interface{}{"test-bridge"}},
		{"ServerStatus", nil},
		{"SetServerConfig", []interface{}{"core.https_address", "[::]"}},
		{"ServerAddresses", nil},
	})
}

func (s *providerSuite) TestFinalizeCloudWithRemoteProvider(c *gc.C) {
	if !containerLXD.HasSupport() {
		c.Skip("To be rewritten during LXD code refactoring for cluster support")
	}

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	cloudFinalizer := deps.provider.(environs.CloudFinalizer)

	cloudSpec := cloud.Cloud{
		Name:      "foo",
		Type:      "lxd",
		Endpoint:  "https://123.123.12.12",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions: []cloud.Region{{
			Name:     "bar",
			Endpoint: "https://321.321.12.12",
		}},
	}

	ctx := testing.NewMockFinalizeCloudContext(ctrl)
	got, err := cloudFinalizer.FinalizeCloud(ctx, cloudSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, cloudSpec)
}

func (s *providerSuite) TestFinalizeCloudWithRemoteProviderWithOnlyRegionEndpoint(c *gc.C) {
	if !containerLXD.HasSupport() {
		c.Skip("To be rewritten during LXD code refactoring for cluster support")
	}

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	cloudFinalizer := deps.provider.(environs.CloudFinalizer)

	cloudSpec := cloud.Cloud{
		Name:      "foo",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions: []cloud.Region{{
			Name:     "bar",
			Endpoint: "https://321.321.12.12",
		}},
	}

	ctx := testing.NewMockFinalizeCloudContext(ctrl)
	got, err := cloudFinalizer.FinalizeCloud(ctx, cloudSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, cloudSpec)
}

func (s *providerSuite) TestFinalizeCloudWithRemoteProviderWithMixedRegions(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	cloudFinalizer := deps.provider.(environs.CloudFinalizer)

	server := lxd.NewMockServer(ctrl)

	deps.factory.EXPECT().LocalServer().Return(server, nil)
	server.EXPECT().LocalBridgeName().Return("lxdbr0")
	deps.factory.EXPECT().LocalServerAddress().Return("https://192.0.0.1:8443", nil)

	cloudSpec := cloud.Cloud{
		Name:      "localhost",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions: []cloud.Region{{
			Name:     "bar",
			Endpoint: "https://321.321.12.12",
		}},
	}

	ctx := testing.NewMockFinalizeCloudContext(ctrl)
	ctx.EXPECT().Verbosef("Resolved LXD host address on bridge %s: %s", "lxdbr0", "https://192.0.0.1:8443")

	got, err := cloudFinalizer.FinalizeCloud(ctx, cloudSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, cloud.Cloud{
		Name:      "localhost",
		Type:      "lxd",
		Endpoint:  "https://192.0.0.1:8443",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions: []cloud.Region{{
			Name:     "bar",
			Endpoint: "https://321.321.12.12",
		}},
	})
}

func (s *providerSuite) TestFinalizeCloudWithRemoteProviderWithNoRegion(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	cloudFinalizer := deps.provider.(environs.CloudFinalizer)

	cloudSpec := cloud.Cloud{
		Name:      "test",
		Type:      "lxd",
		Endpoint:  "https://192.0.0.1:8443",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions:   []cloud.Region{},
	}

	ctx := testing.NewMockFinalizeCloudContext(ctrl)

	got, err := cloudFinalizer.FinalizeCloud(ctx, cloudSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, cloud.Cloud{
		Name:      "test",
		Type:      "lxd",
		Endpoint:  "https://192.0.0.1:8443",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions: []cloud.Region{{
			Name:     "default",
			Endpoint: "https://192.0.0.1:8443",
		}},
	})
}

func (s *providerSuite) TestFinalizeCloudNotListening(c *gc.C) {
	if !containerLXD.HasSupport() {
		c.Skip("To be rewritten during LXD code refactoring for cluster support")
	}

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	cloudFinalizer := deps.provider.(environs.CloudFinalizer)

	deps.factory.EXPECT().LocalServer().Return(nil, errors.New("bad"))

	ctx := testing.NewMockFinalizeCloudContext(ctrl)
	_, err := cloudFinalizer.FinalizeCloud(ctx, cloud.Cloud{
		Name:      "localhost",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions: []cloud.Region{{
			Name: "bar",
		}},
	})
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "bad")
}

func (s *providerSuite) TestFinalizeCloudAlreadyListeningHTTPS(c *gc.C) {
	c.Skip("To be rewritten during LXD code refactoring for cluster support")

	s.Client.Server.Config["core.https_address"] = "[::]:9999"
	var ctx mockContext
	_, err := s.Provider.FinalizeCloud(&ctx, cloud.Cloud{
		Name:      "foo",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
	})
	c.Assert(err, jc.ErrorIsNil)

	// The LXD is already listening on HTTPS, so there should be
	// no SetServerConfig call.
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"DefaultProfileBridgeName", nil},
		{"InterfaceAddress", []interface{}{"test-bridge"}},
		{"ServerStatus", nil},
		{"SetServerConfig", []interface{}{"core.https_address", "[::]"}},
		{"ServerAddresses", nil},
	})
}

func (s *providerSuite) TestDetectRegions(c *gc.C) {
	regions, err := s.Provider.DetectRegions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(regions, jc.DeepEquals, []cloud.Region{{Name: lxdnames.DefaultLocalRegion}})
}

func (s *providerSuite) TestValidate(c *gc.C) {
	validCfg, err := s.Provider.Validate(s.Config, nil)
	c.Assert(err, jc.ErrorIsNil)
	validAttrs := validCfg.AllAttrs()

	c.Check(s.Config.AllAttrs(), gc.DeepEquals, validAttrs)
}

func (s *providerSuite) TestCloudSchema(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	config := `
auth-types: [certificate]
endpoint: http://foo.com/lxd
`[1:]
	var v interface{}
	err := yaml.Unmarshal([]byte(config), &v)
	c.Assert(err, jc.ErrorIsNil)
	v, err = utils.ConformYAML(v)
	c.Assert(err, jc.ErrorIsNil)

	err = deps.provider.CloudSchema().Validate(v)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestPingWithNoEndpoint(c *gc.C) {
	server := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer server.Close()

	p, err := environs.Provider("lxd")
	c.Assert(err, jc.ErrorIsNil)
	err = p.Ping(context.NewCloudCallContext(), server.URL)
	c.Assert(err, gc.ErrorMatches, "no lxd server running at "+containerLXD.EnsureHTTPS(server.URL))
}

type ProviderFunctionalSuite struct {
	lxd.BaseSuite

	provider environs.EnvironProvider
}

func (s *ProviderFunctionalSuite) SetUpTest(c *gc.C) {
	if !s.IsRunningLocally(c) {
		c.Skip("LXD not running locally")
	}

	s.BaseSuite.SetUpTest(c)

	provider, err := environs.Provider("lxd")
	c.Assert(err, jc.ErrorIsNil)

	s.provider = provider
}

func (s *ProviderFunctionalSuite) TestOpen(c *gc.C) {
	env, err := environs.Open(s.provider, environs.OpenParams{
		Cloud:  lxdCloudSpec(),
		Config: s.Config,
	})
	c.Assert(err, jc.ErrorIsNil)
	envConfig := env.Config()

	c.Check(envConfig.Name(), gc.Equals, "testmodel")
}

func (s *ProviderFunctionalSuite) TestPrepareConfig(c *gc.C) {
	cfg, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud:  lxdCloudSpec(),
		Config: s.Config,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg, gc.NotNil)
}

func (s *ProviderFunctionalSuite) TestPrepareConfigUnsupportedEndpointScheme(c *gc.C) {
	cloudSpec := lxdCloudSpec()
	cloudSpec.Endpoint = "unix://foo"
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud:  cloudSpec,
		Config: s.Config,
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: invalid URL "unix://foo": only HTTPS is supported`)
}

func (s *ProviderFunctionalSuite) TestPrepareConfigUnsupportedAuthType(c *gc.C) {
	cred := cloud.NewCredential("foo", nil)
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud: environs.CloudSpec{
			Type:       "lxd",
			Name:       "remotehost",
			Credential: &cred,
		},
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: "foo" auth-type not supported`)
}

func (s *ProviderFunctionalSuite) TestPrepareConfigInvalidCertificateAttrs(c *gc.C) {
	cred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{})
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud: environs.CloudSpec{
			Type:       "lxd",
			Name:       "remotehost",
			Credential: &cred,
		},
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: certificate credentials not valid`)
}

func (s *ProviderFunctionalSuite) TestPrepareConfigEmptyAuthNonLocal(c *gc.C) {
	cred := cloud.NewEmptyCredential()
	_, err := s.provider.PrepareConfig(environs.PrepareConfigParams{
		Cloud: environs.CloudSpec{
			Type:       "lxd",
			Name:       "remotehost",
			Endpoint:   "8.8.8.8",
			Credential: &cred,
		},
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: "empty" auth-type not supported`)
}

type mockContext struct {
	gitjujutesting.Stub
}

func (c *mockContext) Verbosef(f string, args ...interface{}) {
	c.MethodCall(c, "Verbosef", f, args)
}
