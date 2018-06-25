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

func (s *credentialsSuite) createProvider(ctrl *gomock.Controller) (environs.EnvironProvider,
	environs.ProviderCredentials,
	*lxd.MockProviderLXDServer,
	*lxd.MockLXDCertificateReadWriter,
	*lxd.MockLXDCertificateGenerator,
	*lxd.MockLXDNetLookup,
) {
	server := lxd.NewMockProviderLXDServer(ctrl)

	certReadWriter := lxd.NewMockLXDCertificateReadWriter(ctrl)
	certGenerator := lxd.NewMockLXDCertificateGenerator(ctrl)
	lookup := lxd.NewMockLXDNetLookup(ctrl)
	creds := lxd.NewProviderCredentials(
		certReadWriter,
		certGenerator,
		lookup,
		func() (lxd.ProviderLXDServer, error) {
			return server, nil
		},
	)

	provider := lxd.NewProviderWithMocks(creds, utils.GetAddressForInterface, func() (lxd.ProviderLXDServer, error) {
		return server, nil
	})
	return provider, creds, server, certReadWriter, certGenerator, lookup
}

func (s *credentialsSuite) TestDetectCredentialsUsesJujuCert(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	_, provider, server, certsIO, _, _ := s.createProvider(ctrl)

	server.EXPECT().GetCertificate(gomock.Any()).Return(nil, "", nil)
	server.EXPECT().GetServerEnvironmentCertificate().Return("server-cert", nil)

	path := osenv.JujuXDGDataHomePath("lxd")
	certsIO.EXPECT().Read(path).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	credentials, err := provider.DetectCredentials()

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

	_, provider, _, certsIO, _, _ := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	certsIO.EXPECT().Read(path).Return(nil, nil, errors.NotValidf("certs"))

	_, err := provider.DetectCredentials()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "certs not valid")
}

func (s *credentialsSuite) TestDetectCredentialsUsesLXCCert(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	_, provider, server, certsIO, _, _ := s.createProvider(ctrl)

	server.EXPECT().GetCertificate(gomock.Any()).Return(nil, "", nil)
	server.EXPECT().GetServerEnvironmentCertificate().Return("server-cert", nil)

	path := osenv.JujuXDGDataHomePath("lxd")
	certsIO.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	certsIO.EXPECT().Read(path).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	credentials, err := provider.DetectCredentials()

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

	_, provider, _, certsIO, _, _ := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	certsIO.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	certsIO.EXPECT().Read(path).Return(nil, nil, errors.NotValidf("certs"))

	_, err := provider.DetectCredentials()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "certs not valid")
}

func (s *credentialsSuite) TestDetectCredentialsGeneratesCert(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	_, provider, server, certsIO, certsGen, _ := s.createProvider(ctrl)

	server.EXPECT().GetCertificate(gomock.Any()).Return(nil, "", nil)
	server.EXPECT().GetServerEnvironmentCertificate().Return("server-cert", nil)

	path := osenv.JujuXDGDataHomePath("lxd")
	certsIO.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)
	certsIO.EXPECT().Write(path, []byte(coretesting.CACert), []byte(coretesting.CAKey)).Return(nil)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	certsIO.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	certsGen.EXPECT().Generate(true).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	credential := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		},
	)
	credential.Label = `LXD credential "localhost"`

	credentials, err := provider.DetectCredentials()
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

	_, provider, _, certsIO, certsGen, _ := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	certsIO.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	certsIO.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	certsGen.EXPECT().Generate(true).Return(nil, nil, errors.Errorf("bad"))

	credential := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		},
	)
	credential.Label = `LXD credential "localhost"`

	_, err := provider.DetectCredentials()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "bad")
}

func (s *credentialsSuite) TestDetectCredentialsGeneratesCertFailsToGetCertificateOnError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	_, provider, _, certsIO, certsGen, _ := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	certsIO.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)
	certsIO.EXPECT().Write(path, []byte(coretesting.CACert), []byte(coretesting.CAKey)).Return(errors.Errorf("bad"))

	path = filepath.Join(utils.Home(), ".config", "lxc")
	certsIO.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	certsGen.EXPECT().Generate(true).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	credential := cloud.NewCredential(
		cloud.CertificateAuthType,
		map[string]string{
			"client-cert": coretesting.CACert,
			"client-key":  coretesting.CAKey,
			"server-cert": "server-cert",
		},
	)
	credential.Label = `LXD credential "localhost"`

	_, err := provider.DetectCredentials()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "bad")
}

//go:generate mockgen -package lxd -destination net_mock_test.go net Addr

func (s *credentialsSuite) TestFinalizeCredentialLocal(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	_, provider, server, _, _, netLookup := s.createProvider(ctrl)

	server.EXPECT().GetCertificate(gomock.Any()).Return(nil, "", nil)
	server.EXPECT().GetServerEnvironmentCertificate().Return("server-cert", nil)

	localhostIP := net.IPv4(127, 0, 0, 1)
	ipNet := &net.IPNet{IP: localhostIP, Mask: localhostIP.DefaultMask()}

	netLookup.EXPECT().LookupHost("1.2.3.4").Return([]string{"127.0.0.1"}, nil)
	netLookup.EXPECT().InterfaceAddrs().Return([]net.Addr{ipNet}, nil)

	out, err := provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
		CloudEndpoint: "1.2.3.4",
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

	_, provider, server, _, _, _ := s.createProvider(ctrl)

	server.EXPECT().GetCertificate(gomock.Any()).Return(nil, "", nil)
	server.EXPECT().GetServerEnvironmentCertificate().Return("server-cert", nil)

	out, err := provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
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

	_, provider, server, _, _, _ := s.createProvider(ctrl)

	gomock.InOrder(
		server.EXPECT().GetCertificate(gomock.Any()).Return(nil, "", errors.New("not found")),
		server.EXPECT().CreateClientCertificate(gomock.Any()).Return(errors.New("UNIQUE constraint failed: certificates.fingerprint")),
		server.EXPECT().GetCertificate(gomock.Any()).Return(nil, "", nil),
		server.EXPECT().GetServerEnvironmentCertificate().Return("server-cert", nil),
	)

	out, err := provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
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

	_, provider, server, _, _, _ := s.createProvider(ctrl)

	gomock.InOrder(
		server.EXPECT().GetCertificate(gomock.Any()).Return(nil, "", errors.New("not found")),
		server.EXPECT().CreateClientCertificate(gomock.Any()).Return(errors.New("UNIQUE constraint failed: certificates.fingerprint")),
		server.EXPECT().GetCertificate(gomock.Any()).Return(nil, "", errors.New("not found")),
	)

	_, err := provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
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

	_, provider, _, _, _, netLookup := s.createProvider(ctrl)

	netLookup.EXPECT().LookupHost("8.8.8.8").Return([]string{"8.8.8.8"}, nil)
	netLookup.EXPECT().InterfaceAddrs().Return([]net.Addr{}, nil)

	in := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": "foo",
		"client-key":  "bar",
	})
	_, err := provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
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

	_, provider, server, certsIO, _, netLookup := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	certsIO.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	certsIO.EXPECT().Read(path).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	localhostIP := net.IPv4(127, 0, 0, 1)
	ipNet := &net.IPNet{IP: localhostIP, Mask: localhostIP.DefaultMask()}

	netLookup.EXPECT().LookupHost("1.2.3.4").Return([]string{"127.0.0.1"}, nil)
	netLookup.EXPECT().InterfaceAddrs().Return([]net.Addr{ipNet}, nil)

	server.EXPECT().GetCertificate(gomock.Any()).Return(nil, "", nil)
	server.EXPECT().GetServerEnvironmentCertificate().Return("server-cert", nil)

	ctx := cmdtesting.Context(c)
	out, err := provider.FinalizeCredential(ctx, environs.FinalizeCredentialParams{
		CloudEndpoint: "1.2.3.4",
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

	_, provider, _, certsIO, _, netLookup := s.createProvider(ctrl)

	path := osenv.JujuXDGDataHomePath("lxd")
	certsIO.EXPECT().Read(path).Return(nil, nil, os.ErrNotExist)

	path = filepath.Join(utils.Home(), ".config", "lxc")
	certsIO.EXPECT().Read(path).Return([]byte(coretesting.CACert), []byte(coretesting.CAKey), nil)

	netLookup.EXPECT().LookupHost("8.8.8.8").Return([]string{"8.8.8.8"}, nil)
	netLookup.EXPECT().InterfaceAddrs().Return([]net.Addr{}, nil)

	// Patch the interface addresses for the calling machine, so
	// it appears that we're not on the LXD server host.
	_, err := provider.FinalizeCredential(cmdtesting.Context(c), environs.FinalizeCredentialParams{
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
