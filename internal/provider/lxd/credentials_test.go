// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"encoding/base64"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	containerLXD "github.com/juju/juju/internal/container/lxd"
	"github.com/juju/juju/internal/provider/lxd"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package lxd -destination net_mock_test.go net Addr

type credentialsSuite struct {
	lxd.BaseSuite
}

var _ = tc.Suite(&credentialsSuite{})

var errNotFound = api.StatusErrorf(http.StatusNotFound, "")

func (s *credentialsSuite) TestCredentialSchemas(c *tc.C) {
	provider := lxd.NewProvider()
	envtesting.AssertProviderAuthTypes(c, provider, "certificate", "interactive")
}

type credentialsSuiteDeps struct {
	provider       environs.EnvironProvider
	creds          environs.ProviderCredentials
	server         *lxd.MockServer
	serverFactory  *lxd.MockServerFactory
	certReadWriter *lxd.MockCertificateReadWriter
	certGenerator  *lxd.MockCertificateGenerator
	configReader   *lxd.MockLXCConfigReader
}

func (s *credentialsSuite) createProvider(ctrl *gomock.Controller) credentialsSuiteDeps {
	server := lxd.NewMockServer(ctrl)
	factory := lxd.NewMockServerFactory(ctrl)
	factory.EXPECT().LocalServer().Return(server, nil).AnyTimes()

	certReadWriter := lxd.NewMockCertificateReadWriter(ctrl)
	certGenerator := lxd.NewMockCertificateGenerator(ctrl)
	configReader := lxd.NewMockLXCConfigReader(ctrl)
	creds := lxd.NewProviderCredentials(
		certReadWriter,
		certGenerator,
		factory,
		configReader,
	)
	credsRegister := creds.(environs.ProviderCredentialsRegister)

	provider := lxd.NewProviderWithMocks(creds, credsRegister, factory, configReader)
	return credentialsSuiteDeps{
		provider:       provider,
		creds:          creds,
		server:         server,
		serverFactory:  factory,
		certReadWriter: certReadWriter,
		certGenerator:  certGenerator,
		configReader:   configReader,
	}
}

func (s *credentialsSuite) TestDetectCredentialsFailsWithJujuCert(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, errors.NotValidf("certs"))

	_, err := deps.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorMatches, "certs not valid")
}

func (s *credentialsSuite) TestDetectCredentialsFailsWithJujuAndLXCCert(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, errors.NotValidf("certs"))

	_, err := deps.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorMatches, "certs not valid")
}

func (s *credentialsSuite) TestDetectCredentialsGeneratesCertFailsToWriteOnError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read(filepath.Join(utils.Home(), ".config", "lxc")).Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read("snap/lxd/current/.config/lxc").Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read("snap/lxd/common/config").Return(nil, nil, os.ErrNotExist)
	deps.certGenerator.EXPECT().Generate(true, true).Return(nil, nil, errors.Errorf("bad"))

	_, err := deps.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorMatches, "bad")
}

func (s *credentialsSuite) TestDetectCredentialsGeneratesCertFailsToGetCertificateOnError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read(filepath.Join(utils.Home(), ".config", "lxc")).Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read("snap/lxd/current/.config/lxc").Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read("snap/lxd/common/config").Return(nil, nil, os.ErrNotExist)
	deps.certGenerator.EXPECT().Generate(true, true).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)
	deps.certReadWriter.EXPECT().Write(path, []byte(coretesting.CACert), []byte(coretesting.CAKey)).Return(errors.Errorf("bad"))

	_, err := deps.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorMatches, "bad")
}

func (s *credentialsSuite) setupLocalhost(deps credentialsSuiteDeps, c *tc.C) {
	deps.certReadWriter.EXPECT().Read(osenv.JujuXDGDataHomePath("lxd")).Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read(path.Join(utils.Home(), ".config/lxc")).Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read(path.Join(utils.Home(), "snap/lxd/current/.config/lxc")).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)
}

func (s *credentialsSuite) TestRemoteDetectCredentials(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	s.setupLocalhost(deps, c)

	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, nil)
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, nil)
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
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, nil)
	deps.certReadWriter.EXPECT().Read("snap/lxd/current/.config/lxc").Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)
	deps.configReader.EXPECT().ReadCert("snap/lxd/current/.config/lxc/servercerts/nuc1.crt").Return([]byte(coretesting.ServerCert), nil)
	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return(coretesting.ServerCert)

	credentials, err := deps.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)

	nuc1Credential := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": coretesting.ServerCert,
		},
	)
	nuc1Credential.Label = `LXD credential "nuc1"`

	localCredential := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": coretesting.ServerCert,
		},
	)
	localCredential.Label = `LXD credential "localhost"`

	c.Assert(credentials, tc.DeepEquals, &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"nuc1":      nuc1Credential,
			"localhost": localCredential,
		},
	})
}

func (s *credentialsSuite) TestRemoteDetectCredentialsNoRemoteCert(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	s.setupLocalhost(deps, c)

	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, nil)
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, nil)
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{}, nil)
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{
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
	deps.certReadWriter.EXPECT().Read("snap/lxd/common/config").Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)
	deps.configReader.EXPECT().ReadCert("snap/lxd/common/config/servercerts/nuc1.crt").Return(nil, os.ErrNotExist)
	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return(coretesting.ServerCert)

	credentials, err := deps.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)

	nuc1Credential := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
		},
	)
	nuc1Credential.Label = `LXD credential "nuc1"`

	localCredential := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": coretesting.ServerCert,
		},
	)
	localCredential.Label = `LXD credential "localhost"`

	c.Assert(credentials, tc.DeepEquals, &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"nuc1":      nuc1Credential,
			"localhost": localCredential,
		},
	})
}

func (s *credentialsSuite) TestRemoteDetectCredentialsWithConfigFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	s.setupLocalhost(deps, c)

	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, nil)
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, nil)
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/current/.config/lxc/config.yml")).Return(lxd.LXCConfig{}, nil)
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, nil)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", errors.New("bad"))

	credentials, err := deps.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentials, tc.DeepEquals, &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{},
	})
}

func (s *credentialsSuite) TestRemoteDetectCredentialsWithCertFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	s.setupLocalhost(deps, c)

	deps.configReader.EXPECT().ReadConfig(path.Join(osenv.JujuXDGDataHomePath("lxd"), "config.yml")).Return(lxd.LXCConfig{}, nil)
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), ".config/lxc/config.yml")).Return(lxd.LXCConfig{}, nil)
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
	deps.configReader.EXPECT().ReadConfig(path.Join(utils.Home(), "snap/lxd/common/config/config.yml")).Return(lxd.LXCConfig{}, nil)
	deps.certReadWriter.EXPECT().Read(path.Join(utils.Home(), "snap/lxd/current/.config/lxc")).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)
	deps.configReader.EXPECT().ReadCert("snap/lxd/current/.config/lxc/servercerts/nuc1.crt").Return(nil, errors.New("bad"))
	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", errors.New("bad"))

	credentials, err := deps.provider.DetectCredentials("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentials, tc.DeepEquals, &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{},
	})
}

func (s *credentialsSuite) TestRegisterCredentials(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read(filepath.Join(utils.Home(), ".config", "lxc")).Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read("snap/lxd/current/.config/lxc").Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read("snap/lxd/common/config").Return(nil, nil, os.ErrNotExist)
	deps.certGenerator.EXPECT().Generate(true, true).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)
	deps.certReadWriter.EXPECT().Write(path, []byte(coretesting.CACert), []byte(coretesting.CAKey)).Return(nil)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	expected := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		},
	)
	expected.Label = `LXD credential "localhost"`

	provider := deps.provider.(environs.ProviderCredentialsRegister)
	credentials, err := provider.RegisterCredentials(cloud.Cloud{
		Name: "localhost",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentials, tc.DeepEquals, map[string]*cloud.CloudCredential{
		"localhost": {
			DefaultCredential: "localhost",
			AuthCredentials: map[string]cloud.Credential{
				"localhost": expected,
			},
		},
	})
}

func (s *credentialsSuite) TestRegisterCredentialsWithAlternativeCloudName(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read(filepath.Join(utils.Home(), ".config", "lxc")).Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read("snap/lxd/current/.config/lxc").Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Read("snap/lxd/common/config").Return(nil, nil, os.ErrNotExist)
	deps.certGenerator.EXPECT().Generate(true, true).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)
	deps.certReadWriter.EXPECT().Write(path, []byte(coretesting.CACert), []byte(coretesting.CAKey)).Return(nil)
	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	expected := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		},
	)
	expected.Label = `LXD credential "localhost"`

	provider := deps.provider.(environs.ProviderCredentialsRegister)
	credentials, err := provider.RegisterCredentials(cloud.Cloud{
		Name: "lxd",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(credentials, tc.DeepEquals, map[string]*cloud.CloudCredential{
		"lxd": {
			DefaultCredential: "lxd",
			AuthCredentials: map[string]cloud.Credential{
				"lxd": expected,
			},
		},
	})
}

func (s *credentialsSuite) TestRegisterCredentialsUsesJujuCert(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	provider := deps.provider.(environs.ProviderCredentialsRegister)
	credentials, err := provider.RegisterCredentials(cloud.Cloud{
		Name: "localhost",
	})
	c.Assert(err, tc.ErrorIsNil)

	expected := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		},
	)
	expected.Label = `LXD credential "localhost"`

	c.Assert(credentials, tc.DeepEquals, map[string]*cloud.CloudCredential{
		"localhost": {
			DefaultCredential: "localhost",
			AuthCredentials: map[string]cloud.Credential{
				"localhost": expected,
			},
		},
	})
}

func (s *credentialsSuite) TestRegisterCredentialsUsesLXCCert(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	deps.certReadWriter.EXPECT().Read(path).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	provider := deps.provider.(environs.ProviderCredentialsRegister)
	credentials, err := provider.RegisterCredentials(cloud.Cloud{
		Name: "localhost",
	})
	c.Assert(err, tc.ErrorIsNil)

	expected := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		},
	)
	expected.Label = `LXD credential "localhost"`

	c.Assert(credentials, tc.DeepEquals, map[string]*cloud.CloudCredential{
		"localhost": {
			DefaultCredential: "localhost",
			AuthCredentials: map[string]cloud.Credential{
				"localhost": expected,
			},
		},
	})
}

func (s *credentialsSuite) TestFinalizeCredentialLocal(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	out, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		Credential: cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
		}),
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(out.AuthType(), tc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), tc.DeepEquals, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": "server-cert",
	})
}

func (s *credentialsSuite) TestFinalizeCredentialLocalAddCert(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	out, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		Credential: cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
		}),
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(out.AuthType(), tc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), tc.DeepEquals, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": "server-cert",
	})
}

func (s *credentialsSuite) TestFinalizeCredentialLocalAddCertAlreadyExists(c *tc.C) {
	// If we get back an error from CreateClientCertificate, we'll make another
	// call to GetCertificate. If that call succeeds, then we assume
	// that the CreateClientCertificate failure was due to a concurrent call.

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	gomock.InOrder(
		deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", errNotFound),
		deps.server.EXPECT().CreateClientCertificate(s.clientCert()).Return(errors.New("UNIQUE constraint failed: interactives.fingerprint")),
		deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil),
		deps.server.EXPECT().ServerCertificate().Return("server-cert"),
	)

	out, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		Credential: cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
		}),
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(out.AuthType(), tc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), tc.DeepEquals, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": "server-cert",
	})
}

func (s *credentialsSuite) TestFinalizeCredentialLocalAddCertFatal(c *tc.C) {
	// If we get back an error from CreateClientCertificate, we'll make another
	// call to GetCertificate. If that call succeeds, then we assume
	// that the CreateClientCertificate failure was due to a concurrent call.

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	gomock.InOrder(
		deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", errNotFound),
		deps.server.EXPECT().CreateClientCertificate(s.clientCert()).Return(errors.New("UNIQUE constraint failed: interactives.fingerprint")),
		deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", errNotFound),
	)

	_, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		Credential: cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
		}),
	})
	c.Assert(err, tc.ErrorMatches, "adding certificate \"juju\": UNIQUE constraint failed: interactives.fingerprint")
}

func (s *credentialsSuite) TestFinalizeCredentialLocalCertificateWithEmptyClientCert(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	ctx := cmdtesting.Context(c)
	_, err := deps.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		CloudEndpoint: "localhost",
		Credential:    cloud.NewCredential("certificate", map[string]string{}),
	})
	c.Assert(err, tc.ErrorMatches, `missing or empty "client-cert" attribute not valid`)
}

func (s *credentialsSuite) TestFinalizeCredentialLocalCertificateWithEmptyClientKey(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	ctx := cmdtesting.Context(c)
	_, err := deps.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		CloudEndpoint: "localhost",
		Credential: cloud.NewCredential("certificate", map[string]string{
			"client-cert": coretesting.CACert,
		}),
	})
	c.Assert(err, tc.ErrorMatches, `missing or empty "client-key" attribute not valid`)
}

func (s *credentialsSuite) TestFinalizeCredentialWithNonServerAuth(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	ctx := cmdtesting.Context(c)
	_, err := deps.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		CloudEndpoint: "localhost",
		Credential: cloud.NewCredential("certificate", map[string]string{
			"server-cert": coretesting.CACert,
			"client-cert": coretesting.CACert,
		}),
	})
	c.Assert(err, tc.ErrorMatches, `missing or empty "client-key" attribute not valid`)
}

func (s *credentialsSuite) TestFinalizeCredentialLocalCertificate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	ctx := cmdtesting.Context(c)
	out, err := deps.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		Credential: cloud.NewCredential("certificate", map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
		}),
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(out.AuthType(), tc.Equals, cloud.AuthType("certificate"))
	c.Assert(out.Attributes(), tc.DeepEquals, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": "server-cert",
	})
}

func (s *credentialsSuite) TestFinalizeCredentialNonLocalCertificate(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	// Patch the interface addresses for the calling machine, so
	// it appears that we're not on the LXD server host.
	_, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		CloudEndpoint: "8.8.8.8",
		Credential:    cloud.NewCredential("certificate", map[string]string{}),
	})
	c.Assert(err, tc.ErrorMatches, `missing or empty "client-cert" attribute not valid`)
}

func (s *credentialsSuite) TestFinalizeCredentialNonLocal(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	insecureCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert":    coretesting.CACert,
		"client-key":     coretesting.CAKey,
		"trust-password": "fred",
	})
	insecureSpec := lxd.CloudSpec{
		CloudSpec: environscloudspec.CloudSpec{
			Endpoint:   "8.8.8.8",
			Credential: &insecureCred,
		},
	}
	secureCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": coretesting.ServerCert,
	})
	secureSpec := lxd.CloudSpec{
		CloudSpec: environscloudspec.CloudSpec{
			Endpoint:   "8.8.8.8",
			Credential: &secureCred,
		},
	}
	params := environs.FinalizeCredentialParams{
		CloudEndpoint: "8.8.8.8",
		Credential:    insecureCred,
	}
	clientCert := containerLXD.NewCertificate([]byte(coretesting.CACert), []byte(coretesting.CAKey))
	clientX509Cert, err := clientCert.X509()
	c.Assert(err, tc.ErrorIsNil)
	clientX509Base64 := base64.StdEncoding.EncodeToString(clientX509Cert.Raw)
	fingerprint, err := clientCert.Fingerprint()
	c.Assert(err, tc.ErrorIsNil)

	deps.serverFactory.EXPECT().InsecureRemoteServer(insecureSpec).Return(deps.server, nil)
	deps.server.EXPECT().GetCertificate(fingerprint).Return(nil, "", errNotFound)
	deps.server.EXPECT().CreateCertificate(api.CertificatesPost{
		Name:        insecureCred.Label,
		Type:        "client",
		Certificate: clientX509Base64,
		Password:    "fred",
	}).Return(nil)
	deps.server.EXPECT().GetServer().Return(&api.Server{
		Environment: api.ServerEnvironment{
			Certificate: coretesting.ServerCert,
		},
	}, "etag", nil)
	deps.serverFactory.EXPECT().RemoteServer(secureSpec).Return(deps.server, nil)
	deps.server.EXPECT().ServerCertificate().Return(coretesting.ServerCert)

	expected := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": coretesting.ServerCert,
	})

	got, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, &expected)
}

func (s *credentialsSuite) TestFinalizeCredentialRemoteWithTrustToken(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	insecureCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"trust-token": "token1",
	})
	insecureSpec := lxd.CloudSpec{
		CloudSpec: environscloudspec.CloudSpec{
			Endpoint:   "8.8.8.8",
			Credential: &insecureCred,
		},
	}
	secureCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": coretesting.ServerCert,
		"trust-token": "token1",
	})
	secureSpec := lxd.CloudSpec{
		CloudSpec: environscloudspec.CloudSpec{
			Endpoint:   "8.8.8.8",
			Credential: &secureCred,
		},
	}
	params := environs.FinalizeCredentialParams{
		CloudEndpoint: "8.8.8.8",
		Credential:    insecureCred,
	}
	clientCert := containerLXD.NewCertificate([]byte(coretesting.CACert), []byte(coretesting.CAKey))
	clientX509Cert, err := clientCert.X509()
	c.Assert(err, tc.ErrorIsNil)
	clientX509Base64 := base64.StdEncoding.EncodeToString(clientX509Cert.Raw)
	fingerprint, err := clientCert.Fingerprint()
	c.Assert(err, tc.ErrorIsNil)

	deps.serverFactory.EXPECT().InsecureRemoteServer(insecureSpec).Return(deps.server, nil)
	deps.server.EXPECT().GetCertificate(fingerprint).Return(nil, "", errNotFound)
	deps.server.EXPECT().CreateCertificate(api.CertificatesPost{
		Name:        insecureCred.Label,
		Type:        "client",
		Certificate: clientX509Base64,
		TrustToken:  "token1",
	}).Return(nil)
	deps.server.EXPECT().GetServer().Return(&api.Server{
		Environment: api.ServerEnvironment{
			Certificate: coretesting.ServerCert,
		},
	}, "etag", nil)
	deps.serverFactory.EXPECT().RemoteServer(secureSpec).Return(deps.server, nil)
	deps.server.EXPECT().ServerCertificate().Return(coretesting.ServerCert)

	expected := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": coretesting.ServerCert,
	})

	got, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, &expected)
}

func (s *credentialsSuite) TestFinalizeCredentialRemoteWithTrustTokenAndTrustPasswordFails(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	insecureCred := cloud.NewCredential(cloud.InteractiveAuthType, map[string]string{
		"trust-token":    "token1",
		"trust-password": "password1",
	})
	params := environs.FinalizeCredentialParams{
		CloudEndpoint: "8.8.8.8",
		Credential:    insecureCred,
	}

	_, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), params)
	c.Assert(err, tc.ErrorMatches, "both trust token and trust password were supplied.*")
}

func (s *credentialsSuite) TestFinalizeCredentialNonLocalWithCertAlreadyExists(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	insecureCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert":    coretesting.CACert,
		"client-key":     coretesting.CAKey,
		"trust-password": "fred",
	})
	insecureSpec := lxd.CloudSpec{
		CloudSpec: environscloudspec.CloudSpec{
			Endpoint:   "8.8.8.8",
			Credential: &insecureCred,
		},
	}
	secureCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": coretesting.ServerCert,
	})
	secureSpec := lxd.CloudSpec{
		CloudSpec: environscloudspec.CloudSpec{
			Endpoint:   "8.8.8.8",
			Credential: &secureCred,
		},
	}
	params := environs.FinalizeCredentialParams{
		CloudEndpoint: "8.8.8.8",
		Credential:    insecureCred,
	}
	clientCert := containerLXD.NewCertificate([]byte(coretesting.CACert), []byte(coretesting.CAKey))
	fingerprint, err := clientCert.Fingerprint()
	c.Assert(err, tc.ErrorIsNil)

	deps.serverFactory.EXPECT().InsecureRemoteServer(insecureSpec).Return(deps.server, nil)
	deps.server.EXPECT().GetCertificate(fingerprint).Return(&api.Certificate{}, "", nil)
	deps.server.EXPECT().GetServer().Return(&api.Server{
		Environment: api.ServerEnvironment{
			Certificate: coretesting.ServerCert,
		},
	}, "etag", nil)
	deps.serverFactory.EXPECT().RemoteServer(secureSpec).Return(deps.server, nil)
	deps.server.EXPECT().ServerCertificate().Return(coretesting.ServerCert)

	expected := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": coretesting.ServerCert,
	})

	got, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, &expected)
}

func (s *credentialsSuite) TestFinalizeCredentialRemoteWithInsecureError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	insecureCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert":    coretesting.CACert,
		"client-key":     coretesting.CAKey,
		"trust-password": "fred",
	})
	insecureSpec := lxd.CloudSpec{
		CloudSpec: environscloudspec.CloudSpec{
			Endpoint:   "8.8.8.8",
			Credential: &insecureCred,
		},
	}
	params := environs.FinalizeCredentialParams{
		CloudEndpoint: "8.8.8.8",
		Credential:    insecureCred,
	}

	deps.serverFactory.EXPECT().InsecureRemoteServer(insecureSpec).Return(nil, errors.New("bad"))

	_, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), params)
	c.Assert(err, tc.ErrorMatches, "bad")
}

func (s *credentialsSuite) TestFinalizeCredentialRemoteWithCreateCertificateError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	insecureCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert":    coretesting.CACert,
		"client-key":     coretesting.CAKey,
		"trust-password": "fred",
	})
	insecureSpec := lxd.CloudSpec{
		CloudSpec: environscloudspec.CloudSpec{
			Endpoint:   "8.8.8.8",
			Credential: &insecureCred,
		},
	}
	params := environs.FinalizeCredentialParams{
		CloudEndpoint: "8.8.8.8",
		Credential:    insecureCred,
	}
	clientCert := containerLXD.NewCertificate([]byte(coretesting.CACert), []byte(coretesting.CAKey))
	clientX509Cert, err := clientCert.X509()
	c.Assert(err, tc.ErrorIsNil)
	clientX509Base64 := base64.StdEncoding.EncodeToString(clientX509Cert.Raw)
	fingerprint, err := clientCert.Fingerprint()
	c.Assert(err, tc.ErrorIsNil)

	deps.serverFactory.EXPECT().InsecureRemoteServer(insecureSpec).Return(deps.server, nil)
	deps.server.EXPECT().GetCertificate(fingerprint).Return(nil, "", errNotFound)
	deps.server.EXPECT().CreateCertificate(api.CertificatesPost{
		Name:        insecureCred.Label,
		Type:        "client",
		Certificate: clientX509Base64,
		Password:    "fred",
	}).Return(errors.New("bad"))

	_, err = deps.provider.FinalizeCredential(cmdtesting.Context(c), params)
	c.Assert(err, tc.ErrorMatches, "bad")
}

func (s *credentialsSuite) TestFinalizeCredentialRemoveWithGetServerError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	insecureCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert":    coretesting.CACert,
		"client-key":     coretesting.CAKey,
		"trust-password": "fred",
	})
	insecureSpec := lxd.CloudSpec{
		CloudSpec: environscloudspec.CloudSpec{
			Endpoint:   "8.8.8.8",
			Credential: &insecureCred,
		},
	}
	params := environs.FinalizeCredentialParams{
		CloudEndpoint: "8.8.8.8",
		Credential:    insecureCred,
	}
	clientCert := containerLXD.NewCertificate([]byte(coretesting.CACert), []byte(coretesting.CAKey))
	clientX509Cert, err := clientCert.X509()
	c.Assert(err, tc.ErrorIsNil)
	clientX509Base64 := base64.StdEncoding.EncodeToString(clientX509Cert.Raw)
	fingerprint, err := clientCert.Fingerprint()
	c.Assert(err, tc.ErrorIsNil)

	deps.serverFactory.EXPECT().InsecureRemoteServer(insecureSpec).Return(deps.server, nil)
	deps.server.EXPECT().GetCertificate(fingerprint).Return(nil, "", errNotFound)
	deps.server.EXPECT().CreateCertificate(api.CertificatesPost{
		Name:        insecureCred.Label,
		Type:        "client",
		Certificate: clientX509Base64,
		Password:    "fred",
	}).Return(nil)
	deps.server.EXPECT().GetServer().Return(nil, "etag", errors.New("bad"))

	_, err = deps.provider.FinalizeCredential(cmdtesting.Context(c), params)
	c.Assert(err, tc.ErrorMatches, "bad")
}

func (s *credentialsSuite) TestFinalizeCredentialRemoteWithNewRemoteServerError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	insecureCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert":    coretesting.CACert,
		"client-key":     coretesting.CAKey,
		"trust-password": "fred",
	})
	insecureSpec := lxd.CloudSpec{
		CloudSpec: environscloudspec.CloudSpec{
			Endpoint:   "8.8.8.8",
			Credential: &insecureCred,
		},
	}
	secureCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": coretesting.ServerCert,
	})
	secureSpec := lxd.CloudSpec{
		CloudSpec: environscloudspec.CloudSpec{
			Endpoint:   "8.8.8.8",
			Credential: &secureCred,
		},
	}
	params := environs.FinalizeCredentialParams{
		CloudEndpoint: "8.8.8.8",
		Credential:    insecureCred,
	}
	clientCert := containerLXD.NewCertificate([]byte(coretesting.CACert), []byte(coretesting.CAKey))
	clientX509Cert, err := clientCert.X509()
	c.Assert(err, tc.ErrorIsNil)
	clientX509Base64 := base64.StdEncoding.EncodeToString(clientX509Cert.Raw)
	fingerprint, err := clientCert.Fingerprint()
	c.Assert(err, tc.ErrorIsNil)

	deps.serverFactory.EXPECT().InsecureRemoteServer(insecureSpec).Return(deps.server, nil)
	deps.server.EXPECT().GetCertificate(fingerprint).Return(nil, "", errNotFound)
	deps.server.EXPECT().CreateCertificate(api.CertificatesPost{
		Name:        insecureCred.Label,
		Type:        "client",
		Certificate: clientX509Base64,
		Password:    "fred",
	}).Return(nil)
	deps.server.EXPECT().GetServer().Return(&api.Server{
		Environment: api.ServerEnvironment{
			Certificate: coretesting.ServerCert,
		},
	}, "etag", nil)
	deps.serverFactory.EXPECT().RemoteServer(secureSpec).Return(nil, errors.New("bad"))

	_, err = deps.provider.FinalizeCredential(cmdtesting.Context(c), params)
	c.Assert(err, tc.ErrorMatches, "bad")
}

func (s *credentialsSuite) TestInteractiveFinalizeCredentialWithValidCredentials(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)
	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	out, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		Credential: cloud.NewCredential("interactive", map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		}),
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(out.AuthType(), tc.Equals, cloud.AuthType("certificate"))
	c.Assert(out.Attributes(), tc.DeepEquals, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": "server-cert",
	})
}

func (s *credentialsSuite) TestInteractiveFinalizeCredentialWithTrustPassword(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	deps.certReadWriter.EXPECT().Read(path).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	out, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		Credential: cloud.NewCredential("interactive", map[string]string{
			"trust-password": "password1",
		}),
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(out.AuthType(), tc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), tc.DeepEquals, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": "server-cert",
	})
}

func (s *credentialsSuite) TestInteractiveFinalizeCredentialWithTrustToken(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	deps.certReadWriter.EXPECT().Read(path).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	out, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		Credential: cloud.NewCredential("interactive", map[string]string{
			"trust-token": "token1",
		}),
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(out.AuthType(), tc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), tc.DeepEquals, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": "server-cert",
	})
}

func (s *credentialsSuite) TestInteractiveFinalizeCredentialWithCertFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, errors.New("bad"))

	_, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		CloudEndpoint: "localhost",
		Credential: cloud.NewCredential("interactive", map[string]string{
			"trust-password": "password1",
		}),
	})
	c.Assert(err, tc.ErrorMatches, "bad")
}

func (s *credentialsSuite) clientCert() *containerLXD.Certificate {
	return &containerLXD.Certificate{
		Name:    "juju",
		CertPEM: []byte(coretesting.CACert),
		KeyPEM:  []byte(coretesting.CAKey),
	}
}

func (s *credentialsSuite) clientCertFingerprint(c *tc.C) string {
	fp, err := s.clientCert().Fingerprint()
	c.Assert(err, tc.ErrorIsNil)
	return fp
}

func (s *credentialsSuite) TestGetCertificates(c *tc.C) {
	cred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
		"server-cert": "server.crt",
	})
	cert, server, ok := lxd.GetCertificates(cred)
	c.Assert(ok, tc.Equals, true)
	c.Assert(cert, tc.DeepEquals, s.clientCert())
	c.Assert(server, tc.Equals, "server.crt")
}

func (s *credentialsSuite) TestGetCertificatesMissingClientCert(c *tc.C) {
	cred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-key":  coretesting.CAKey,
		"server-cert": "server.crt",
	})
	_, _, ok := lxd.GetCertificates(cred)
	c.Assert(ok, tc.Equals, false)
}

func (s *credentialsSuite) TestGetCertificatesMissingClientKey(c *tc.C) {
	cred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": coretesting.CACert,
		"server-cert": "server.crt",
	})
	_, _, ok := lxd.GetCertificates(cred)
	c.Assert(ok, tc.Equals, false)
}

func (s *credentialsSuite) TestGetCertificatesMissingServerCert(c *tc.C) {
	cred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": coretesting.CACert,
		"client-key":  coretesting.CAKey,
	})
	_, _, ok := lxd.GetCertificates(cred)
	c.Assert(ok, tc.Equals, false)
}
