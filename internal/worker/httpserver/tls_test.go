// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	ctx "context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	stdtesting "testing"

	"github.com/juju/tc"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/httpserver"
)

type tlsStateFixture struct {
	testhelpers.IsolationSuite
	cert *tls.Certificate

	dnsName   string
	serverURL string
	cache     autocert.DirCache
}

func (s *tlsStateFixture) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.cert = &tls.Certificate{
		Leaf: &x509.Certificate{
			DNSNames: []string{
				"testing1.invalid",
				"testing2.invalid",
				"testing3.invalid",
			},
		},
	}
}

type TLSStateSuite struct {
	tlsStateFixture
}

func TestTLSStateSuite(t *stdtesting.T) { tc.Run(t, &TLSStateSuite{}) }
func (s *TLSStateSuite) TestNewTLSConfig(c *tc.C) {
	tlsConfig := httpserver.NewTLSConfig(
		s.dnsName,
		s.serverURL,
		s.cache,
		testSNIGetter(s.cert),
		loggertesting.WrapCheckLog(c),
	)

	cert, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{
		ServerName: "anything.invalid",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cert, tc.Equals, s.cert)
}

type TLSAutocertSuite struct {
	tlsStateFixture
	autocertQueried bool
}

func TestTLSAutocertSuite(t *stdtesting.T) { tc.Run(t, &TLSAutocertSuite{}) }
func (s *TLSAutocertSuite) SetUpSuite(c *tc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.autocertQueried = true
		http.Error(w, "burp", http.StatusUnavailableForLegalReasons)
	}))
	s.dnsName = "public.invalid"
	s.serverURL = server.URL
	s.tlsStateFixture.SetUpSuite(c)
	s.cache = autocert.DirCache(c.MkDir())
	s.cache.Put(ctx.TODO(), "public.invalid", []byte("data"))
	s.AddCleanup(func(c *tc.C) { server.Close() })
}

func (s *TLSAutocertSuite) SetUpTest(c *tc.C) {
	s.tlsStateFixture.SetUpTest(c)
	s.autocertQueried = false
}

func (s *TLSAutocertSuite) TestAutocertExceptions(c *tc.C) {
	tlsConfig := httpserver.NewTLSConfig(
		s.dnsName,
		s.serverURL,
		s.cache,
		testSNIGetter(s.cert),
		loggertesting.WrapCheckLog(c),
	)
	s.testGetCertificate(c, tlsConfig, "127.0.0.1")
	s.testGetCertificate(c, tlsConfig, "juju-apiserver")
	s.testGetCertificate(c, tlsConfig, "testing1.invalid")
	c.Assert(s.autocertQueried, tc.IsFalse)
}

func (s *TLSAutocertSuite) TestAutocert(c *tc.C) {
	tlsConfig := httpserver.NewTLSConfig(
		s.dnsName,
		s.serverURL,
		s.cache,
		testSNIGetter(s.cert),
		loggertesting.WrapCheckLog(c),
	)
	s.testGetCertificate(c, tlsConfig, "public.invalid")
	c.Assert(s.autocertQueried, tc.IsTrue)
	c.Assert(tlsConfig.NextProtos, tc.DeepEquals, []string{"h2", "http/1.1", acme.ALPNProto})
}

func (s *TLSAutocertSuite) TestAutocertHostPolicy(c *tc.C) {
	tlsConfig := httpserver.NewTLSConfig(
		s.dnsName,
		s.serverURL,
		s.cache,
		testSNIGetter(s.cert),
		loggertesting.WrapCheckLog(c),
	)
	s.testGetCertificate(c, tlsConfig, "always.invalid")
	c.Assert(s.autocertQueried, tc.IsFalse)
}

func (s *TLSAutocertSuite) TestAutoCertNotCalledBadDNS(c *tc.C) {
	tlsConfig := httpserver.NewTLSConfig(
		s.dnsName,
		s.serverURL,
		s.cache,
		testSNIGetter(s.cert),
		loggertesting.WrapCheckLog(c),
	)
	s.testGetCertificate(c, tlsConfig, "invalid")
	c.Assert(s.autocertQueried, tc.IsFalse)
}

func (s *TLSAutocertSuite) testGetCertificate(c *tc.C, tlsConfig *tls.Config, serverName string) {
	cert, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{
		ServerName: serverName,
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("server name %q", serverName))
	// NOTE(axw) we always expect to get back s.cert, because we don't have
	// a functioning autocert test server. We do check that we attempt to
	// query the autocert server, but that's as far as we test here.
	c.Assert(cert, tc.Equals, s.cert, tc.Commentf("server name %q", serverName))
}
