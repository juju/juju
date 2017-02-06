// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"io/ioutil"
	"net"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/provider/lxd"
)

type credentialsSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	envtesting.AssertProviderAuthTypes(c, s.Provider, "certificate")
}

func (s *credentialsSuite) TestDetectCredentialsUsesLXCCert(c *gc.C) {
	home := c.MkDir()
	utils.SetHome(home)
	s.writeFile(c, filepath.Join(home, ".config/lxc/client.crt"), "client-cert-data")
	s.writeFile(c, filepath.Join(home, ".config/lxc/client.key"), "client-key-data")

	credentials, err := s.Provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, jc.DeepEquals, &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"default": cloud.NewCredential(
				cloud.CertificateAuthType,
				map[string]string{
					"client-cert": "client-cert-data",
					"client-key":  "client-key-data",
				},
			),
		},
	})
	s.Stub.CheckCallNames(c)
}

func (s *credentialsSuite) TestDetectCredentialsUsesJujuLXDCert(c *gc.C) {
	// If there's a keypair for both the LXC client and Juju, we will
	// always pick the Juju one.
	home := c.MkDir()
	utils.SetHome(home)
	xdg := osenv.JujuXDGDataHomeDir()
	s.writeFile(c, filepath.Join(home, ".config/lxc/client.crt"), "lxc-client-cert-data")
	s.writeFile(c, filepath.Join(home, ".config/lxc/client.key"), "lxc-client-key-data")
	s.writeFile(c, filepath.Join(xdg, "lxd/client.crt"), "juju-client-cert-data")
	s.writeFile(c, filepath.Join(xdg, "lxd/client.key"), "juju-client-key-data")

	credentials, err := s.Provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, jc.DeepEquals, &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"default": cloud.NewCredential(
				cloud.CertificateAuthType,
				map[string]string{
					"client-cert": "juju-client-cert-data",
					"client-key":  "juju-client-key-data",
				},
			),
		},
	})
	s.Stub.CheckCallNames(c)
}

func (S *credentialsSuite) writeFile(c *gc.C, path, content string) {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(path, []byte(content), 0600)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *credentialsSuite) TestDetectCredentialsGeneratesCert(c *gc.C) {
	credentials, err := s.Provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, jc.DeepEquals, &cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"default": cloud.NewCredential(
				cloud.CertificateAuthType,
				map[string]string{
					"client-cert": "client.crt",
					"client-key":  "client.key",
				},
			),
		},
	})
	s.Stub.CheckCallNames(c, "GenerateMemCert")

	// The cert/key pair should have been cached in the juju/lxd dir.
	xdg := osenv.JujuXDGDataHomeDir()
	content, err := ioutil.ReadFile(filepath.Join(xdg, "lxd/client.crt"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "client.crt")
	content, err = ioutil.ReadFile(filepath.Join(xdg, "lxd/client.key"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "client.key")
}

func (s *credentialsSuite) TestFinalizeCredentialLocal(c *gc.C) {
	cert, _ := s.TestingCert(c)
	out, err := s.Provider.FinalizeCredential(nil, environs.FinalizeCredentialParams{
		CloudEndpoint: "1.2.3.4",
		Credential: cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
			"client-cert": string(cert.CertPEM),
			"client-key":  string(cert.KeyPEM),
		}),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(out.AuthType(), gc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"client-cert": string(cert.CertPEM),
		"client-key":  string(cert.KeyPEM),
		"server-cert": "server-cert",
	})
	s.Stub.CheckCallNames(c,
		"LookupHost",
		"InterfaceAddrs",
		"CertByFingerprint",
		"ServerStatus",
	)
}

func (s *credentialsSuite) TestFinalizeCredentialLocalAddCert(c *gc.C) {
	s.Stub.SetErrors(errors.NotFoundf("certificate"))
	cert, _ := s.TestingCert(c)
	out, err := s.Provider.FinalizeCredential(nil, environs.FinalizeCredentialParams{
		CloudEndpoint: "", // skips host lookup
		Credential: cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
			"client-cert": string(cert.CertPEM),
			"client-key":  string(cert.KeyPEM),
		}),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(out.AuthType(), gc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"client-cert": string(cert.CertPEM),
		"client-key":  string(cert.KeyPEM),
		"server-cert": "server-cert",
	})
	s.Stub.CheckCallNames(c,
		"CertByFingerprint",
		"AddCert",
		"ServerStatus",
	)
}

func (s *credentialsSuite) TestFinalizeCredentialLocalAddCertAlreadyThere(c *gc.C) {
	// If we get back an error from AddCert, we'll make another call
	// to CertByFingerprint. If that call succeeds, then we assume
	// that the AddCert failure was due to a concurrent AddCert.
	s.Stub.SetErrors(
		errors.NotFoundf("certificate"),
		errors.New("UNIQUE constraint failed: certificates.fingerprint"),
	)
	cert, _ := s.TestingCert(c)
	out, err := s.Provider.FinalizeCredential(nil, environs.FinalizeCredentialParams{
		CloudEndpoint: "", // skips host lookup
		Credential: cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
			"client-cert": string(cert.CertPEM),
			"client-key":  string(cert.KeyPEM),
		}),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(out.AuthType(), gc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"client-cert": string(cert.CertPEM),
		"client-key":  string(cert.KeyPEM),
		"server-cert": "server-cert",
	})
	s.Stub.CheckCallNames(c,
		"CertByFingerprint",
		"AddCert",
		"CertByFingerprint",
		"ServerStatus",
	)
}

func (s *credentialsSuite) TestFinalizeCredentialLocalAddCertFatal(c *gc.C) {
	// If we get back an error from AddCert, we'll make another call
	// to CertByFingerprint. If that call fails with "not found", then
	// we assume that the AddCert failure is fatal.
	s.Stub.SetErrors(
		errors.NotFoundf("certificate"),
		errors.New("some fatal error"),
		errors.NotFoundf("certificate"),
	)
	cert, _ := s.TestingCert(c)
	_, err := s.Provider.FinalizeCredential(nil, environs.FinalizeCredentialParams{
		CloudEndpoint: "", // skips host lookup
		Credential: cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
			"client-cert": string(cert.CertPEM),
			"client-key":  string(cert.KeyPEM),
		}),
	})
	c.Assert(err, gc.ErrorMatches, `adding certificate "juju": some fatal error`)
}

func (s *credentialsSuite) TestFinalizeCredentialNonLocal(c *gc.C) {
	// Patch the interface addresses for the calling machine, so
	// it appears that we're not on the LXD server host.
	s.PatchValue(&s.InterfaceAddrs, []net.Addr{&net.IPNet{IP: net.ParseIP("8.8.8.8")}})
	in := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": "foo",
		"client-key":  "bar",
	})
	out, err := s.Provider.FinalizeCredential(nil, environs.FinalizeCredentialParams{
		CloudEndpoint: "8.8.8.8",
		Credential:    in,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, &in)
}
