// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver_test

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/juju/juju/worker/httpserver"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type tlsStateFixture struct {
	stateFixture
	cert *tls.Certificate
}

func (s *tlsStateFixture) SetUpTest(c *gc.C) {
	s.stateFixture.SetUpTest(c)
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

func (s *tlsStateFixture) getCertificate() *tls.Certificate {
	return s.cert
}

type TLSStateSuite struct {
	tlsStateFixture
}

var _ = gc.Suite(&TLSStateSuite{})

func (s *TLSStateSuite) TestNewTLSConfig(c *gc.C) {
	tlsConfig, handler, err := httpserver.NewTLSConfig(s.State, s.getCertificate)
	c.Assert(err, jc.ErrorIsNil)

	cert, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{
		ServerName: "anything.invalid",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cert, gc.Equals, s.cert)

	c.Assert(handler, gc.Equals, nil)
}

type TLSStateAutocertSuite struct {
	tlsStateFixture
	autocertQueried bool
}

var _ = gc.Suite(&TLSStateAutocertSuite{})

func (s *TLSStateAutocertSuite) SetUpSuite(c *gc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.autocertQueried = true
		http.Error(w, "burp", http.StatusInternalServerError)
	}))
	s.ControllerConfig = map[string]interface{}{
		"autocert-dns-name": "public.invalid",
		"autocert-url":      server.URL,
	}
	s.tlsStateFixture.SetUpSuite(c)
	s.AddCleanup(func(c *gc.C) { server.Close() })
}

func (s *TLSStateAutocertSuite) SetUpTest(c *gc.C) {
	s.tlsStateFixture.SetUpTest(c)
	s.autocertQueried = false
}

func (s *TLSStateAutocertSuite) TestAutocertExceptions(c *gc.C) {
	tlsConfig, handler, err := httpserver.NewTLSConfig(s.State, s.getCertificate)
	c.Assert(err, jc.ErrorIsNil)
	s.testGetCertificate(c, tlsConfig, "127.0.0.1")
	s.testGetCertificate(c, tlsConfig, "juju-apiserver")
	s.testGetCertificate(c, tlsConfig, "testing1.invalid")
	c.Assert(s.autocertQueried, jc.IsFalse)
	s.testHandler(c, handler)
}

func (s *TLSStateAutocertSuite) TestAutocert(c *gc.C) {
	tlsConfig, handler, err := httpserver.NewTLSConfig(s.State, s.getCertificate)
	c.Assert(err, jc.ErrorIsNil)
	s.testGetCertificate(c, tlsConfig, "public.invalid")
	c.Assert(s.autocertQueried, jc.IsTrue)
	s.testHandler(c, handler)
}

func (s *TLSStateAutocertSuite) TestAutocertHostPolicy(c *gc.C) {
	tlsConfig, handler, err := httpserver.NewTLSConfig(s.State, s.getCertificate)
	c.Assert(err, jc.ErrorIsNil)
	s.testGetCertificate(c, tlsConfig, "always.invalid")
	c.Assert(s.autocertQueried, jc.IsFalse)
	s.testHandler(c, handler)
}

func (s *TLSStateAutocertSuite) testGetCertificate(c *gc.C, tlsConfig *tls.Config, serverName string) {
	cert, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: serverName})
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("server name %q", serverName))
	// NOTE(axw) we always expect to get back s.cert, because we don't have
	// a functioning autocert test server. We do check that we attempt to
	// query the autocert server, but that's as far as we test here.
	c.Assert(cert, gc.Equals, s.cert, gc.Commentf("server name %q", serverName))
}

func (s *TLSStateAutocertSuite) testHandler(c *gc.C, handler http.Handler) {
	// Just check enough to see that it's the autocert handler.
	request := httptest.NewRequest("GET", "http://public.invalid/.well-known/acme-challenge/", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	response := recorder.Result()
	c.Assert(response.StatusCode, gc.Equals, http.StatusNotFound)
	content, err := ioutil.ReadAll(response.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "acme/autocert: certificate cache miss\n")
}
