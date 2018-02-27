// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/tls"
	"crypto/x509"
	"runtime"
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/cert"
	coretesting "github.com/juju/juju/testing"
)

type certSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&certSuite{})

func (s *certSuite) TestUpdateCert(c *gc.C) {
	tlsCert := coretesting.ServerTLSCert
	config := s.sampleConfig(c)
	config.GetCertificate = func() *tls.Certificate {
		return tlsCert
	}
	srv := s.newServer(c, config)

	// Sanity check that the server works initially.
	conn := s.OpenAPIAsAdmin(c, srv)
	c.Assert(pingConn(conn), jc.ErrorIsNil)

	// Create a new certificate that's a year out of date, so we can
	// tell that the server is using it because the connection will fail.
	srvCert, srvKey, err := cert.NewServer(coretesting.CACert, coretesting.CAKey, time.Now().AddDate(-1, 0, 0), nil)
	c.Assert(err, jc.ErrorIsNil)
	badTLSCert, err := tls.X509KeyPair([]byte(srvCert), []byte(srvKey))
	if err != nil {
		panic(err)
	}
	x509Cert, err := x509.ParseCertificate(badTLSCert.Certificate[0])
	if err != nil {
		panic(err)
	}
	badTLSCert.Leaf = x509Cert

	// Check that we can't connect to the server because of the bad certificate.
	tlsCert = &badTLSCert
	apiInfo := s.APIInfo(srv)
	apiInfo.Tag = s.Owner
	apiInfo.Password = ownerPassword
	_, err = api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, gc.ErrorMatches, `unable to connect to API: .*: certificate has expired or is not yet valid`)

	// Replace the working certificate and check that we can connect again.
	tlsCert = coretesting.ServerTLSCert
	conn = s.OpenAPIAsAdmin(c, srv)
	c.Assert(pingConn(conn), jc.ErrorIsNil)
}

func (s *certSuite) TestAutocertFailure(c *gc.C) {
	s.PatchValue(apiserver.EnableAutocertChallengeHandler, false)

	// We don't have a fake autocert server, but we can at least
	// smoke test that the autocert path is followed when we try
	// to connect to a DNS name - the AutocertURL configured
	// by the testing suite is invalid so it should fail.

	config := s.sampleConfig(c)
	config.AutocertDNSName = "somewhere.example"

	srv := s.newServer(c, config)
	apiInfo := s.APIInfo(srv)
	entries := gatherLog(func() {
		_, err := tls.Dial("tcp", apiInfo.Addrs[0], &tls.Config{
			ServerName: "somewhere.example",
		})
		expectedErr := `x509: certificate is valid for \*, not somewhere.example`
		if runtime.GOOS == "windows" {
			// For some reason, windows doesn't think that the certificate is signed
			// by a valid authority. This could be problematic.
			expectedErr = "x509: certificate signed by unknown authority"
		}
		// We can't get an autocert certificate, so we'll fall back to the local certificate
		// which isn't valid for connecting to somewhere.example.
		c.Assert(err, gc.ErrorMatches, expectedErr)
	})
	// We will log the failure to get the certificate, thus assuring us that we actually tried.
	c.Assert(entries, jc.LogMatches, jc.SimpleMessages{{
		loggo.ERROR,
		`.*cannot get autocert certificate for "somewhere.example": Get https://0\.1\.2\.3/no-autocert-here: .*`,
	}})
}

func (s *certSuite) TestAutocertNameMismatch(c *gc.C) {
	s.PatchValue(apiserver.EnableAutocertChallengeHandler, false)

	config := s.sampleConfig(c)
	config.AutocertDNSName = "somewhere.example"

	srv := s.newServer(c, config)
	apiInfo := s.APIInfo(srv)

	entries := gatherLog(func() {
		_, err := tls.Dial("tcp", apiInfo.Addrs[0], &tls.Config{
			ServerName: "somewhere.else",
		})
		expectedErr := `x509: certificate is valid for \*, not somewhere.else`
		if runtime.GOOS == "windows" {
			// For some reason, windows doesn't think that the certificate is signed
			// by a valid authority. This could be problematic.
			expectedErr = "x509: certificate signed by unknown authority"
		}
		// We can't get an autocert certificate, so we'll fall back to the local certificate
		// which isn't valid for connecting to somewhere.example.
		c.Assert(err, gc.ErrorMatches, expectedErr)
	})
	// Check that we logged the mismatch.
	c.Assert(entries, jc.LogMatches, jc.SimpleMessages{{
		loggo.ERROR,
		`.*cannot get autocert certificate for "somewhere.else": acme/autocert: host not configured`,
	}})
}

func (s *certSuite) TestAutocertNoAutocertDNSName(c *gc.C) {
	s.PatchValue(apiserver.EnableAutocertChallengeHandler, false)
	config := s.sampleConfig(c)
	c.Assert(config.AutocertDNSName, gc.Equals, "") // sanity check
	srv := s.newServer(c, config)
	apiInfo := s.APIInfo(srv)

	entries := gatherLog(func() {
		_, err := tls.Dial("tcp", apiInfo.Addrs[0], &tls.Config{
			ServerName: "somewhere.example",
		})
		expectedErr := `x509: certificate is valid for \*, not somewhere.example`
		if runtime.GOOS == "windows" {
			// For some reason, windows doesn't think that the certificate is signed
			// by a valid authority. This could be problematic.
			expectedErr = "x509: certificate signed by unknown authority"
		}
		// We can't get an autocert certificate, so we'll fall back to the local certificate
		// which isn't valid for connecting to somewhere.example.
		c.Assert(err, gc.ErrorMatches, expectedErr)
	})
	// Check that we never logged a failure to get the certificate.
	c.Assert(entries, gc.Not(jc.LogMatches), jc.SimpleMessages{{
		loggo.ERROR,
		`.*cannot get autocert certificate.*`,
	}})
}

func gatherLog(f func()) []loggo.Entry {
	var tw loggo.TestWriter
	err := loggo.RegisterWriter("test", &tw)
	if err != nil {
		panic(err)
	}
	defer loggo.RemoveWriter("test")
	f()
	return tw.Log()
}
