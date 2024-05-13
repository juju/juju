// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	ctx "context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	gc "gopkg.in/check.v1"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/httpserver"
)

type tlsStateFixture struct {
	testing.IsolationSuite
	cert *tls.Certificate

	dnsName   string
	serverURL string
	cache     autocert.DirCache
}

func (s *tlsStateFixture) SetUpTest(c *gc.C) {
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

var _ = gc.Suite(&TLSStateSuite{})

func (s *TLSStateSuite) TestNewTLSConfig(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cert, gc.Equals, s.cert)
}

type TLSAutocertSuite struct {
	tlsStateFixture
	autocertQueried bool
}

var _ = gc.Suite(&TLSAutocertSuite{})

func (s *TLSAutocertSuite) SetUpSuite(c *gc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.autocertQueried = true
		http.Error(w, "burp", http.StatusUnavailableForLegalReasons)
	}))
	s.dnsName = "public.invalid"
	s.serverURL = server.URL
	s.tlsStateFixture.SetUpSuite(c)
	s.cache = autocert.DirCache(c.MkDir())
	s.cache.Put(ctx.TODO(), "public.invalid", []byte("data"))
	s.AddCleanup(func(c *gc.C) { server.Close() })
}

func (s *TLSAutocertSuite) SetUpTest(c *gc.C) {
	s.tlsStateFixture.SetUpTest(c)
	s.autocertQueried = false
}

func (s *TLSAutocertSuite) TestAutocertExceptions(c *gc.C) {
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
	c.Assert(s.autocertQueried, jc.IsFalse)
}

func (s *TLSAutocertSuite) TestAutocert(c *gc.C) {
	tlsConfig := httpserver.NewTLSConfig(
		s.dnsName,
		s.serverURL,
		s.cache,
		testSNIGetter(s.cert),
		loggertesting.WrapCheckLog(c),
	)
	s.testGetCertificate(c, tlsConfig, "public.invalid")
	c.Assert(s.autocertQueried, jc.IsTrue)
	c.Assert(tlsConfig.NextProtos, jc.DeepEquals, []string{"h2", "http/1.1", acme.ALPNProto})
}

func (s *TLSAutocertSuite) TestAutocertHostPolicy(c *gc.C) {
	tlsConfig := httpserver.NewTLSConfig(
		s.dnsName,
		s.serverURL,
		s.cache,
		testSNIGetter(s.cert),
		loggertesting.WrapCheckLog(c),
	)
	s.testGetCertificate(c, tlsConfig, "always.invalid")
	c.Assert(s.autocertQueried, jc.IsFalse)
}

func (s *TLSAutocertSuite) TestAutoCertNotCalledBadDNS(c *gc.C) {
	tlsConfig := httpserver.NewTLSConfig(
		s.dnsName,
		s.serverURL,
		s.cache,
		testSNIGetter(s.cert),
		loggertesting.WrapCheckLog(c),
	)
	s.testGetCertificate(c, tlsConfig, "invalid")
	c.Assert(s.autocertQueried, jc.IsFalse)
}

func (s *TLSAutocertSuite) testGetCertificate(c *gc.C, tlsConfig *tls.Config, serverName string) {
	cert, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{
		ServerName: serverName,
	})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("server name %q", serverName))
	// NOTE(axw) we always expect to get back s.cert, because we don't have
	// a functioning autocert test server. We do check that we attempt to
	// query the autocert server, but that's as far as we test here.
	c.Assert(cert, gc.Equals, s.cert, gc.Commentf("server name %q", serverName))
}
