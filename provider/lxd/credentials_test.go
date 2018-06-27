// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"net"
	"os"
	"path/filepath"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	containerLXD "github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/provider/lxd"
	coretesting "github.com/juju/juju/testing"
)

type credentialsSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	provider := lxd.NewProvider()
	envtesting.AssertProviderAuthTypes(c, provider, "interactive", "certificate")
}

type credentialsSuiteDeps struct {
	provider       environs.EnvironProvider
	creds          environs.ProviderCredentials
	server         *lxd.MockProviderLXDServer
	certReadWriter *lxd.MockCertificateReadWriter
	certGenerator  *lxd.MockCertificateGenerator
	netLookup      *lxd.MockNetLookup
}

func (s *credentialsSuite) createProvider(ctrl *gomock.Controller) credentialsSuiteDeps {
	server := lxd.NewMockProviderLXDServer(ctrl)

	certReadWriter := lxd.NewMockCertificateReadWriter(ctrl)
	certGenerator := lxd.NewMockCertificateGenerator(ctrl)
	lookup := lxd.NewMockNetLookup(ctrl)
	creds := lxd.NewProviderCredentials(
		certReadWriter,
		certGenerator,
		lookup,
		func() (lxd.ProviderLXDServer, error) {
			return server, nil
		},
	)
	interfaceAddress := lxd.NewMockLXDInterfaceAddress(ctrl)

	provider := lxd.NewProviderWithMocks(creds, interfaceAddress, func() (lxd.ProviderLXDServer, error) {
		return server, nil
	})
	return credentialsSuiteDeps{
		provider:       provider,
		creds:          creds,
		server:         server,
		certReadWriter: certReadWriter,
		certGenerator:  certGenerator,
		netLookup:      lookup,
	}
}

func (s *credentialsSuite) TestDetectCredentialsUsesJujuCert(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	credentials, err := deps.provider.DetectCredentials()

	expected := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		},
	)
	expected.Label = `LXD credential "localhost"`

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, jc.DeepEquals, &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"localhost": expected,
		},
	})
}

func (s *credentialsSuite) TestDetectCredentialsFailsWithJujuCert(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, errors.NotValidf("certs"))

	_, err := deps.provider.DetectCredentials()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "certs not valid")
}

func (s *credentialsSuite) TestDetectCredentialsUsesLXCCert(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	deps.certReadWriter.EXPECT().Read(path).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	credentials, err := deps.provider.DetectCredentials()

	expected := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		},
	)
	expected.Label = `LXD credential "localhost"`

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, jc.DeepEquals, &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"localhost": expected,
		},
	})
}

func (s *credentialsSuite) TestDetectCredentialsFailsWithJujuAndLXCCert(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, errors.NotValidf("certs"))

	_, err := deps.provider.DetectCredentials()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "certs not valid")
}

func (s *credentialsSuite) TestDetectCredentialsGeneratesCert(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Write(path, []byte(coretesting.CACert), []byte(coretesting.CAKey)).Return(nil)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	deps.certGenerator.EXPECT().Generate(true).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	credential := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		},
	)
	credential.Label = `LXD credential "localhost"`

	credentials, err := deps.provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, jc.DeepEquals, &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"localhost": credential,
		},
	})
}

func (s *credentialsSuite) TestDetectCredentialsGeneratesCertFailsToWriteOnError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	deps.certGenerator.EXPECT().Generate(true).Return(nil, nil, errors.Errorf("bad"))

	credential := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		},
	)
	credential.Label = `LXD credential "localhost"`

	_, err := deps.provider.DetectCredentials()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "bad")
}

func (s *credentialsSuite) TestDetectCredentialsGeneratesCertFailsToGetCertificateOnError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)
	deps.certReadWriter.EXPECT().Write(path, []byte(coretesting.CACert), []byte(coretesting.CAKey)).Return(errors.Errorf("bad"))

	path = filepath.Join(utils.Home(), ".config", "lxc")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	deps.certGenerator.EXPECT().Generate(true).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	credential := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		},
	)
	credential.Label = `LXD credential "localhost"`

	_, err := deps.provider.DetectCredentials()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "bad")
}

//go:generate mockgen -package lxd -destination net_mock_test.go net Addr

func (s *credentialsSuite) TestFinalizeCredentialLocal(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	localhostIP := net.IPv4(127, 0, 0, 1)
	ipNet := &net.IPNet{IP: localhostIP, Mask: localhostIP.DefaultMask()}

	deps.netLookup.EXPECT().LookupHost("localhost").Return([]string{"127.0.0.1"}, nil)
	deps.netLookup.EXPECT().InterfaceAddrs().Return([]net.Addr{ipNet}, nil)

	out, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		CloudEndpoint: "localhost",
		Credential: cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
			"client-cert": string(coretesting.CACert),
			"client-key":  string(coretesting.CAKey),
		}),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(out.AuthType(), gc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"client-cert": string(coretesting.CACert),
		"client-key":  string(coretesting.CAKey),
		"server-cert": "server-cert",
	})
}

func (s *credentialsSuite) TestFinalizeCredentialLocalLocalAddCert(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	out, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		CloudEndpoint: "",
		Credential: cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
			"client-cert": string(coretesting.CACert),
			"client-key":  string(coretesting.CAKey),
		}),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(out.AuthType(), gc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"client-cert": string(coretesting.CACert),
		"client-key":  string(coretesting.CAKey),
		"server-cert": "server-cert",
	})
}

func (s *credentialsSuite) TestFinalizeCredentialLocalLocalAddCertAlreadyExists(c *gc.C) {
	// If we get back an error from CreateClientCertificate, we'll make another
	// call to GetCertificate. If that call succeeds, then we assume
	// that the CreateClientCertificate failure was due to a concurrent call.

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	gomock.InOrder(
		deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", errors.New("not found")),
		deps.server.EXPECT().CreateClientCertificate(s.clientCert()).Return(errors.New("UNIQUE constraint failed: certificates.fingerprint")),
		deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil),
		deps.server.EXPECT().ServerCertificate().Return("server-cert"),
	)

	out, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		CloudEndpoint: "",
		Credential: cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
			"client-cert": string(coretesting.CACert),
			"client-key":  string(coretesting.CAKey),
		}),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(out.AuthType(), gc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"client-cert": string(coretesting.CACert),
		"client-key":  string(coretesting.CAKey),
		"server-cert": "server-cert",
	})
}

func (s *credentialsSuite) TestFinalizeCredentialLocalAddCertFatal(c *gc.C) {
	// If we get back an error from CreateClientCertificate, we'll make another
	// call to GetCertificate. If that call succeeds, then we assume
	// that the CreateClientCertificate failure was due to a concurrent call.

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	gomock.InOrder(
		deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", errors.New("not found")),
		deps.server.EXPECT().CreateClientCertificate(s.clientCert()).Return(errors.New("UNIQUE constraint failed: certificates.fingerprint")),
		deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", errors.New("not found")),
	)

	_, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		CloudEndpoint: "",
		Credential: cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
			"client-cert": string(coretesting.CACert),
			"client-key":  string(coretesting.CAKey),
		}),
	})
	c.Assert(err, gc.ErrorMatches, "adding certificate \"juju\": UNIQUE constraint failed: certificates.fingerprint")
}

func (s *credentialsSuite) TestFinalizeCredentialNonLocal(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	deps.netLookup.EXPECT().LookupHost("8.8.8.8").Return([]string{"8.8.8.8"}, nil)
	deps.netLookup.EXPECT().InterfaceAddrs().Return([]net.Addr{}, nil)

	in := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": "foo",
		"client-key":  "bar",
	})
	_, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		CloudEndpoint: "8.8.8.8",
		Credential:    in,
	})
	c.Assert(err, gc.ErrorMatches, `
cannot auto-generate credential for remote LXD

Until support is added for verifying and authenticating to remote LXD hosts,
you must generate the credential on the LXD host, and add the credential to
this client using "juju add-credential localhost".

See: https://jujucharms.com/docs/stable/clouds-LXD
`[1:])
}

func (s *credentialsSuite) TestFinalizeCredentialLocalInteractive(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	deps.certReadWriter.EXPECT().Read(path).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	localhostIP := net.IPv4(127, 0, 0, 1)
	ipNet := &net.IPNet{IP: localhostIP, Mask: localhostIP.DefaultMask()}

	deps.netLookup.EXPECT().LookupHost("localhost").Return([]string{"127.0.0.1"}, nil)
	deps.netLookup.EXPECT().InterfaceAddrs().Return([]net.Addr{ipNet}, nil)

	deps.server.EXPECT().GetCertificate(s.clientCertFingerprint(c)).Return(nil, "", nil)
	deps.server.EXPECT().ServerCertificate().Return("server-cert")

	ctx := cmdtesting.Context(c)
	out, err := deps.provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		CloudEndpoint: "localhost",
		Credential:    cloud.NewCredential("interactive", map[string]string{}),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(out.AuthType(), gc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"client-cert": string(coretesting.CACert),
		"client-key":  string(coretesting.CAKey),
		"server-cert": "server-cert",
	})
}

func (s *credentialsSuite) TestFinalizeCredentialNonLocalInteractive(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	deps := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	deps.certReadWriter.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	deps.certReadWriter.EXPECT().Read(path).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	deps.netLookup.EXPECT().LookupHost("8.8.8.8").Return([]string{"8.8.8.8"}, nil)
	deps.netLookup.EXPECT().InterfaceAddrs().Return([]net.Addr{}, nil)

	// Patch the interface addresses for the calling machine, so
	// it appears that we're not on the LXD server host.
	_, err := deps.provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		CloudEndpoint: "8.8.8.8",
		Credential:    cloud.NewCredential("interactive", map[string]string{}),
	})
	c.Assert(err, gc.ErrorMatches, `
certificate upload for remote LXD unsupported

Until support is added for verifying and authenticating to remote LXD hosts,
you must generate the credential on the LXD host, and add the credential to
this client using "juju add-credential localhost".

See: https://jujucharms.com/docs/stable/clouds-LXD
`[1:])
}

func (s *credentialsSuite) clientCert() *containerLXD.Certificate {
	return &containerLXD.Certificate{
		Name:    "juju",
		CertPEM: []byte(coretesting.CACert),
		KeyPEM:  []byte(coretesting.CAKey),
	}
}

func (s *credentialsSuite) clientCertFingerprint(c *gc.C) string {
	fp, err := s.clientCert().Fingerprint()
	c.Assert(err, jc.ErrorIsNil)
	return fp
}
