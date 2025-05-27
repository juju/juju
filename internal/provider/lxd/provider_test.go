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
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/internal/provider/lxd/lxdnames"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
)

func TestProviderSuite(t *stdtesting.T) {
	tc.Run(t, &providerSuite{})
}

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

func (s *providerSuite) TestDetectClouds(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{}, nil)
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))

	cloudDetector := deps.provider.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, 1)
	s.assertLocalhostCloud(c, clouds[0])
}

func (s *providerSuite) TestRemoteDetectClouds(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, 2)
	c.Assert(clouds, tc.DeepEquals, []cloud.Cloud{
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

func (s *providerSuite) TestRemoteDetectCloudsWithConfigError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))

	cloudDetector := deps.provider.(environs.CloudDetector)

	clouds, err := cloudDetector.DetectClouds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, 1)
	s.assertLocalhostCloud(c, clouds[0])
}

func (s *providerSuite) TestDetectCloud(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	s.assertLocalhostCloud(c, cloud)
	cloud, err = cloudDetector.DetectCloud("lxd")
	c.Assert(err, tc.ErrorIsNil)
	s.assertLocalhostCloud(c, cloud)
}

func (s *providerSuite) TestRemoteDetectCloud(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, cloud.Cloud{
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

func (s *providerSuite) TestRemoteDetectCloudWithConfigError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	cloudDetector := deps.provider.(environs.CloudDetector)

	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))

	_, err := cloudDetector.DetectCloud("nuc1")
	c.Assert(err, tc.ErrorMatches, `cloud nuc1 not found`)
}

func (s *providerSuite) TestDetectCloudError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, errors.New("bad"))

	cloudDetector := deps.provider.(environs.CloudDetector)

	_, err := cloudDetector.DetectCloud("foo")
	c.Assert(err, tc.ErrorMatches, `cloud foo not found`)
}

func (s *providerSuite) assertLocalhostCloud(c *tc.C, found cloud.Cloud) {
	c.Assert(found, tc.DeepEquals, cloud.Cloud{
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

func (s *providerSuite) TestFinalizeCloud(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	server := lxd.NewMockServer(ctrl)
	finalizer := deps.provider.(environs.CloudFinalizer)

	deps.factory.EXPECT().LocalServer().Return(server, nil)
	server.EXPECT().LocalBridgeName().Return("lxdbr0")
	deps.factory.EXPECT().LocalServerAddress().Return("1.2.3.4:1234", nil)

	ctx := mockContext{Context: c.Context()}
	out, err := finalizer.FinalizeCloud(&ctx, cloud.Cloud{
		Name:      "localhost",
		Type:      "lxd",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions: []cloud.Region{{
			Name: "localhost",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, cloud.Cloud{
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

func (s *providerSuite) TestFinalizeCloudWithRemoteProvider(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	finalizer := deps.provider.(environs.CloudFinalizer)

	ctx := mockContext{Context: c.Context()}
	out, err := finalizer.FinalizeCloud(&ctx, cloud.Cloud{
		Name:      "nuc8",
		Type:      "lxd",
		Endpoint:  "http://10.0.0.1:8443",
		AuthTypes: []cloud.AuthType{cloud.CertificateAuthType},
		Regions:   []cloud.Region{},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, cloud.Cloud{
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

func (s *providerSuite) TestFinalizeCloudWithRemoteProviderWithOnlyRegionEndpoint(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, cloudSpec)
}

func (s *providerSuite) TestFinalizeCloudWithRemoteProviderWithMixedRegions(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, cloud.Cloud{
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

func (s *providerSuite) TestFinalizeCloudWithRemoteProviderWithNoRegion(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, cloud.Cloud{
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

func (s *providerSuite) TestFinalizeCloudNotListening(c *tc.C) {
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
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.ErrorMatches, "bad")
}

func (s *providerSuite) TestDetectRegions(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	cloudDetector := deps.provider.(environs.CloudRegionDetector)

	regions, err := cloudDetector.DetectRegions()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(regions, tc.DeepEquals, []cloud.Region{{Name: lxdnames.DefaultLocalRegion}})
}

func (s *providerSuite) TestValidate(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	validCfg, err := deps.provider.Validate(c.Context(), s.Config, nil)
	c.Assert(err, tc.ErrorIsNil)
	validAttrs := validCfg.AllAttrs()

	c.Check(s.Config.AllAttrs(), tc.DeepEquals, validAttrs)
}

func (s *providerSuite) TestValidateWithInvalidConfig(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	config, err := jujutesting.ModelConfig(c).Apply(map[string]interface{}{
		"value": int64(1),
	})
	c.Assert(err, tc.IsNil)

	_, err = deps.provider.Validate(c.Context(), config, nil)
	c.Assert(err, tc.NotNil)
}

func (s *providerSuite) TestCloudSchema(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	config := `
auth-types: [certificate]
endpoint: http://foo.com/lxd
`[1:]
	var v interface{}
	err := yaml.Unmarshal([]byte(config), &v)
	c.Assert(err, tc.ErrorIsNil)
	v, err = utils.ConformYAML(v)
	c.Assert(err, tc.ErrorIsNil)

	err = deps.provider.CloudSchema().Validate(v)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerSuite) TestPingFailWithNoEndpoint(c *tc.C) {
	server := httptest.NewTLSServer(http.HandlerFunc(http.NotFound))
	defer server.Close()

	p, err := environs.Provider("lxd")
	c.Assert(err, tc.ErrorIsNil)
	err = p.Ping(c.Context(), server.URL)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(
		"no lxd server running at %[1]s: Failed to fetch %[1]s/1.0: 404 Not Found",
		server.URL))
}

func (s *providerSuite) TestPingFailWithHTTP(c *tc.C) {
	server := httptest.NewServer(http.HandlerFunc(http.NotFound))
	defer server.Close()

	p, err := environs.Provider("lxd")
	c.Assert(err, tc.ErrorIsNil)
	err = p.Ping(c.Context(), server.URL)
	httpsURL := "https://" + strings.TrimPrefix(server.URL, "http://")
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(
		`no lxd server running at %[1]s: Get "%[1]s/1.0": http: server gave HTTP response to HTTPS client`,
		httpsURL))
}

func (s *providerSuite) TestOpen(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	server := lxd.NewMockServer(ctrl)
	deps.factory.EXPECT().RemoteServer(gomock.Any()).DoAndReturn(func(cs lxd.CloudSpec) (lxd.Server, error) {
		return server, nil
	})
	server.EXPECT().HasProfile(gomock.Any()).Return(true, nil)

	env, err := environs.Open(c.Context(), deps.provider, environs.OpenParams{
		Cloud:  lxdCloudSpec(),
		Config: s.Config,
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	envConfig := env.Config()

	c.Check(envConfig.Name(), tc.Equals, "testmodel")
}

func (s *providerSuite) TestValidateCloud(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	err := deps.provider.ValidateCloud(c.Context(), lxdCloudSpec())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerSuite) TestValidateCloudUnsupportedEndpointScheme(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	cloudSpec := lxdCloudSpec()
	cloudSpec.Endpoint = "unix://foo"
	err := deps.provider.ValidateCloud(c.Context(), cloudSpec)
	c.Assert(err, tc.ErrorMatches, `validating cloud spec: invalid URL "unix://foo": only HTTPS is supported`)
}

func (s *providerSuite) TestValidateCloudUnsupportedAuthType(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	cred := cloud.NewCredential("foo", nil)
	err := deps.provider.ValidateCloud(c.Context(), environscloudspec.CloudSpec{
		Type:       "lxd",
		Name:       "remotehost",
		Credential: &cred,
	})
	c.Assert(err, tc.ErrorMatches, `validating cloud spec: "foo" auth-type not supported`)
}

func (s *providerSuite) TestValidateCloudInvalidCertificateAttrs(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	cred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{})
	err := deps.provider.ValidateCloud(c.Context(), environscloudspec.CloudSpec{
		Type:       "lxd",
		Name:       "remotehost",
		Credential: &cred,
	})
	c.Assert(err, tc.ErrorMatches, `validating cloud spec: certificate credentials not valid`)
}

func (s *providerSuite) TestValidateCloudEmptyAuthNonLocal(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	cred := cloud.NewEmptyCredential()
	err := deps.provider.ValidateCloud(c.Context(), environscloudspec.CloudSpec{
		Type:       "lxd",
		Name:       "remotehost",
		Endpoint:   "8.8.8.8",
		Credential: &cred,
	})
	c.Assert(err, tc.ErrorMatches, `validating cloud spec: "empty" auth-type not supported`)
}

type mockContext struct {
	context.Context
	testhelpers.Stub
}

func (c *mockContext) Verbosef(f string, args ...interface{}) {
	c.MethodCall(c, "Verbosef", f, args)
}
