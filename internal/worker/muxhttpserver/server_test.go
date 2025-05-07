// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package muxhttpserver_test

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki"
	pkitest "github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/worker/muxhttpserver"
)

type ServerSuite struct {
	authority pki.Authority
	client    *http.Client
}

var _ = tc.Suite(&ServerSuite{})

func (s *ServerSuite) SetUpSuite(c *tc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)
	s.authority = authority

	_, err = s.authority.LeafRequestForGroup(pki.DefaultLeafGroup).
		AddDNSNames("localhost").
		Commit()
	c.Assert(err, jc.ErrorIsNil)

	certPool := x509.NewCertPool()
	certPool.AddCert(s.authority.Certificate())

	config := &tls.Config{
		InsecureSkipVerify: false,
		RootCAs:            certPool,
	}

	s.client = &http.Client{
		Transport: &http.Transport{TLSClientConfig: config},
	}
}

func (s *ServerSuite) TestNoRouteHTTPServer(c *tc.C) {
	server, err := muxhttpserver.NewServer(
		s.authority, loggertesting.WrapCheckLog(c), muxhttpserver.Config{
			Address: "localhost",
			Port:    "0",
		})
	c.Assert(err, jc.ErrorIsNil)

	resp, err := s.client.Get("https://localhost:" + server.Port())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusNotFound)

	server.Kill()
	err = server.Wait()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ServerSuite) TestRouteHandlerCalled(c *tc.C) {
	server, err := muxhttpserver.NewServer(
		s.authority, loggertesting.WrapCheckLog(c), muxhttpserver.Config{
			Address: "localhost",
			Port:    "0",
		})
	c.Assert(err, jc.ErrorIsNil)

	handlerCalled := false
	server.Mux.AddHandler(http.MethodGet, "/test",
		http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			handlerCalled = true
		}))

	resp, err := s.client.Get("https://localhost:" + server.Port() + "/test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	c.Assert(handlerCalled, tc.Equals, true)

	server.Kill()
	err = server.Wait()
	c.Assert(err, jc.ErrorIsNil)
}
