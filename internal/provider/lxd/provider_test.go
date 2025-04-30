// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"

	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/internal/provider/lxd/lxdnames"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
)

var (
	_ = gc.Suite(&providerSuite{})
	_ = gc.Suite(&ProviderFunctionalSuite{})
)

type providerSuite struct {
	lxd.BaseSuite
}

type providerSuiteDeps struct {
	provider      environs.EnvironProvider
	creds         *testing.MockProviderCredentials
	credsRegister *testing.MockProviderCredentialsRegister
	factory       *lxd.MockServerFactory
	configReader  *lxd.MockLXCConfigReader
}

func (s *providerSuite) createProvider(ctrl *gomock.Controller) providerSuiteDeps {
	creds := testing.NewMockProviderCredentials(ctrl)
	credsRegister := testing.NewMockProviderCredentialsRegister(ctrl)
	factory := lxd.NewMockServerFactory(ctrl)
	configReader := lxd.NewMockLXCConfigReader(ctrl)

	provider := lxd.NewProviderWithMocks(creds, credsRegister, factory, configReader)
	return providerSuiteDeps{
		provider:      provider,
		creds:         creds,
		credsRegister: credsRegister,
		factory:       factory,
		configReader:  configReader,
	}
}

func (s *providerSuite) TestDetectClouds(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{}, nil)
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))

	cloudDetector := deps.provider.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 1)
	s.assertLocalhostCloud(c, clouds[0])
}

func (s *providerSuite) TestRemoteDetectClouds(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{
		DefaultRemote: "localhost",
		Remotes: map[string]lxd.LXCRemoteConfig{
			"nuc1": {
				Addr:     "https://10.0.0.1:8443",
				AuthType: "certificate",
				Protocol: "lxd",
				Public:   false,
			},
		},
	}, nil)
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))

	cloudDetector := deps.provider.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 2)
	c.Assert(clouds, jc.DeepEquals, []cloud.Cloud{
		{
			Name: "localhost",
			Type: "lxd",
			AuthTypes: []cloud.AuthType{
				cloud.CertificateAuthType,
			},
			Regions: []cloud.Region{{
				Name: "localhost",
			}},
			Description: "LXD Container Hypervisor",
		},
		{
			Name:     "nuc1",
			Type:     "lxd",
			Endpoint: "https://10.0.0.1:8443",
			AuthTypes: []cloud.AuthType{
				cloud.CertificateAuthType,
			},
			Regions: []cloud.Region{{
				Name:     "default",
				Endpoint: "https://10.0.0.1:8443",
			}},
			Description: "LXD Cluster",
		},
	})
}

func (s *providerSuite) TestRemoteDetectCloudsWithConfigError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))

	cloudDetector := deps.provider.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 1)
	s.assertLocalhostCloud(c, clouds[0])
}

func (s *providerSuite) TestDetectCloud(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	cloudDetector := deps.provider.(environs.CloudDetector)

	cloud, err := cloudDetector.DetectCloud("localhost")
	c.Assert(err, jc.ErrorIsNil)
	s.assertLocalhostCloud(c, cloud)
	cloud, err = cloudDetector.DetectCloud("lxd")
	c.Assert(err, jc.ErrorIsNil)
	s.assertLocalhostCloud(c, cloud)
}

func (s *providerSuite) TestRemoteDetectCloud(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	cloudDetector := deps.provider.(environs.CloudDetector)

	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{
		DefaultRemote: "localhost",
		Remotes: map[string]lxd.LXCRemoteConfig{
			"nuc1": {
				Addr:     "https://10.0.0.1:8443",
				AuthType: "certificate",
				Protocol: "lxd",
				Public:   false,
			},
		},
	}, nil)

	got, err := cloudDetector.DetectCloud("nuc1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, cloud.Cloud{
		Name:     "nuc1",
		Type:     "lxd",
		Endpoint: "https://10.0.0.1:8443",
		AuthTypes: []cloud.AuthType{
			cloud.CertificateAuthType,
		},
		Regions: []cloud.Region{{
			Name:     "default",
			Endpoint: "https://10.0.0.1:8443",
		}},
		Description: "LXD Cluster",
	})
}

func (s *providerSuite) TestRemoteDetectCloudWithConfigError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	cloudDetector := deps.provider.(environs.CloudDetector)

	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))

	_, err := cloudDetector.DetectCloud("nuc1")
	c.Assert(err, gc.ErrorMatches, `cloud nuc1 not found`)
}

func (s *providerSuite) TestDetectCloudError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))

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
		},
		Regions: []cloud.Region{{
			Name: "localhost",
		}},
		Description: "LXD Container Hypervisor",
	})
}

func (s *providerSuite) TestFinalizeCloud(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	server := lxd.NewMockServer(ctrl)
	finalizer := deps.provider.(environs.CloudFinalizer)

	deps.factory.EXPECT().LocalServer().Return(server, nil)
	server.EXPECT().LocalBridgeName().Return("lxdbr0")
	deps.factory.EXPECT().LocalServerAddress().Return("1.2.3.4:1234", nil)

	ctx := mockContext{Context: context.Background()}
	out, err := finalizer.FinalizeCloud(&ctx, cloud.Cloud{
		Name:      "localhost",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions: []cloud.Region{{
			Name: "localhost",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, cloud.Cloud{
		Name:      "localhost",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Endpoint:  "1.2.3.4:1234",
		Regions: []cloud.Region{{
			Name:     "localhost",
			Endpoint: "1.2.3.4:1234",
		}},
	})
	ctx.CheckCallNames(c, "Verbosef")
	ctx.CheckCall(
		c, 0, "Verbosef", "Resolved LXD host address on bridge %s: %s",
		[]interface{}{"lxdbr0", "1.2.3.4:1234"},
	)
}

func (s *providerSuite) TestFinalizeCloudWithRemoteProvider(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	finalizer := deps.provider.(environs.CloudFinalizer)

	ctx := mockContext{Context: context.Background()}
	out, err := finalizer.FinalizeCloud(&ctx, cloud.Cloud{
		Name:      "nuc8",
		Type:      "lxd",
		Endpoint:  "http://10.0.0.1:8443",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions:   []cloud.Region{},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, cloud.Cloud{
		Name:      "nuc8",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Endpoint:  "http://10.0.0.1:8443",
		Regions: []cloud.Region{{
			Name:     "default",
			Endpoint: "http://10.0.0.1:8443",
		}},
	})
}

func (s *providerSuite) TestFinalizeCloudWithRemoteProviderWithOnlyRegionEndpoint(c *gc.C) {
	ctrl := s.SetupMocks(c)
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
	ctrl := s.SetupMocks(c)
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
	ctrl := s.SetupMocks(c)
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
	ctrl := s.SetupMocks(c)
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

func (s *providerSuite) TestDetectRegions(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	cloudDetector := deps.provider.(environs.CloudRegionDetector)

	regions, err := cloudDetector.DetectRegions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(regions, jc.DeepEquals, []cloud.Region{{Name: lxdnames.DefaultLocalRegion}})
}

func (s *providerSuite) TestValidate(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	validCfg, err := deps.provider.Validate(context.Background(), s.Config, nil)
	c.Assert(err, jc.ErrorIsNil)
	validAttrs := validCfg.AllAttrs()

	c.Check(s.Config.AllAttrs(), gc.DeepEquals, validAttrs)
}

func (s *providerSuite) TestValidateWithInvalidConfig(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	config, err := jujutesting.ModelConfig(c).Apply(map[string]interface{}{
		"value": int64(1),
	})
	c.Assert(err, gc.IsNil)

	_, err = deps.provider.Validate(context.Background(), config, nil)
	c.Assert(err, gc.NotNil)
}

func (s *providerSuite) TestCloudSchema(c *gc.C) {
	ctrl := s.SetupMocks(c)
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

func (s *providerSuite) TestPingFailWithNoEndpoint(c *gc.C) {
	server := httptest.NewTLSServer(http.HandlerFunc(http.NotFound))
	defer server.Close()

	p, err := environs.Provider("lxd")
	c.Assert(err, jc.ErrorIsNil)
	err = p.Ping(context.Background(), server.URL)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(
		"no lxd server running at %[1]s: Failed to fetch %[1]s/1.0: 404 Not Found",
		server.URL))
}

func (s *providerSuite) TestPingFailWithHTTP(c *gc.C) {
	server := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer server.Close()

	p, err := environs.Provider("lxd")
	c.Assert(err, jc.ErrorIsNil)
	err = p.Ping(context.Background(), server.URL)
	httpsURL := "https://" + strings.TrimPrefix(server.URL, "http://")
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(
		`no lxd server running at %[1]s: Get "%[1]s/1.0": http: server gave HTTP response to HTTPS client`,
		httpsURL))
}

type ProviderFunctionalSuite struct {
	lxd.BaseSuite

	provider environs.EnvironProvider
}

func (s *ProviderFunctionalSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	provider, err := environs.Provider("lxd")
	c.Assert(err, jc.ErrorIsNil)

	s.provider = provider
}

func (s *ProviderFunctionalSuite) TestOpen(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env, err := environs.Open(context.Background(), s.provider, environs.OpenParams{
		Cloud:  lxdCloudSpec(),
		Config: s.Config,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, jc.ErrorIsNil)
	envConfig := env.Config()

	c.Check(envConfig.Name(), gc.Equals, "testmodel")
}

func (s *ProviderFunctionalSuite) TestValidateCloud(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	err := s.provider.ValidateCloud(context.Background(), lxdCloudSpec())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ProviderFunctionalSuite) TestValidateCloudUnsupportedEndpointScheme(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	cloudSpec := lxdCloudSpec()
	cloudSpec.Endpoint = "unix://foo"
	err := s.provider.ValidateCloud(context.Background(), cloudSpec)
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: invalid URL "unix://foo": only HTTPS is supported`)
}

func (s *ProviderFunctionalSuite) TestValidateCloudUnsupportedAuthType(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	cred := cloud.NewCredential("foo", nil)
	err := s.provider.ValidateCloud(context.Background(), environscloudspec.CloudSpec{
		Type:       "lxd",
		Name:       "remotehost",
		Credential: &cred,
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: "foo" auth-type not supported`)
}

func (s *ProviderFunctionalSuite) TestValidateCloudInvalidCertificateAttrs(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	cred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{})
	err := s.provider.ValidateCloud(context.Background(), environscloudspec.CloudSpec{
		Type:       "lxd",
		Name:       "remotehost",
		Credential: &cred,
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: certificate credentials not valid`)
}

func (s *ProviderFunctionalSuite) TestValidateCloudEmptyAuthNonLocal(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	cred := cloud.NewEmptyCredential()
	err := s.provider.ValidateCloud(context.Background(), environscloudspec.CloudSpec{
		Type:       "lxd",
		Name:       "remotehost",
		Endpoint:   "8.8.8.8",
		Credential: &cred,
	})
	c.Assert(err, gc.ErrorMatches, `validating cloud spec: "empty" auth-type not supported`)
}

type mockContext struct {
	context.Context
	jtesting.Stub
}

func (c *mockContext) Verbosef(f string, args ...interface{}) {
	c.MethodCall(c, "Verbosef", f, args)
}
