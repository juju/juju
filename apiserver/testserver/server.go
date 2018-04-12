// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testserver

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/pubsub/centralhub"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

// DefaultServerConfig returns the default configuration for starting a test server.
func DefaultServerConfig(c *gc.C) apiserver.ServerConfig {
	fakeOrigin := names.NewMachineTag("0")
	hub := centralhub.New(fakeOrigin)
	return apiserver.ServerConfig{
		Clock:           clock.WallClock,
		Tag:             names.NewMachineTag("0"),
		LogDir:          c.MkDir(),
		Hub:             hub,
		NewObserver:     func() observer.Observer { return &fakeobserver.Instance{} },
		RateLimitConfig: apiserver.DefaultRateLimitConfig(),
		GetAuditConfig:  func() auditlog.Config { return auditlog.Config{Enabled: false} },
		UpgradeComplete: func() bool { return true },
		RestoreStatus: func() state.RestoreStatus {
			return state.RestoreNotActive
		},
	}
}

// NewServer returns a new running API server using the given state.
// The pool may be nil, in which case a pool using the given state
// will be used.
//
// It returns information suitable for connecting to the state
// without any authentication information or model tag, and the server
// that's been started.
func NewServer(c *gc.C, statePool *state.StatePool) *Server {
	return NewServerWithConfig(c, statePool, DefaultServerConfig(c))
}

// NewServerWithConfig is like NewServer except that the entire
// server configuration may be specified (see DefaultServerConfig
// for a suitable starting point).
func NewServerWithConfig(c *gc.C, statePool *state.StatePool, cfg apiserver.ServerConfig) *Server {
	// Note that we can't listen on localhost here because TestAPIServerCanListenOnBothIPv4AndIPv6 assumes
	// that we listen on IPv6 too, and listening on localhost does not do that.
	listener, err := net.Listen("tcp", ":0")
	c.Assert(err, jc.ErrorIsNil)
	certPool, err := api.CreateCertPool(coretesting.CACert)
	if err != nil {
		c.Fatalf(err.Error())
	}
	tlsConfig := api.NewTLSConfig(certPool)
	tlsConfig.ServerName = "juju-apiserver"
	tlsConfig.Certificates = []tls.Certificate{*coretesting.ServerTLSCert}
	mux := apiserverhttp.NewMux()
	httpServer := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: mux},
		TLS:      tlsConfig,
	}

	cfg.Mux = mux
	cfg.StatePool = statePool

	if cfg.Authenticator == nil {
		authenticator, err := stateauthenticator.NewAuthenticator(statePool, cfg.Clock)
		c.Assert(err, jc.ErrorIsNil)
		cfg.Authenticator = authenticator
	}

	srv, err := apiserver.NewServer(cfg)
	c.Assert(err, jc.ErrorIsNil)
	httpServer.StartTLS()

	return &Server{
		APIServer:  srv,
		HTTPServer: httpServer,
		Info: &api.Info{
			Addrs:  []string{fmt.Sprintf("localhost:%d", listener.Addr().(*net.TCPAddr).Port)},
			CACert: coretesting.CACert,
		},
	}
}

// Server wraps both the HTTP and API servers needed to test API
// interactions and simplifies managing their lifecycles.
type Server struct {
	APIServer  *apiserver.Server
	HTTPServer *httptest.Server
	Info       *api.Info
}

// Stop stops both the API and HTTP servers.
func (s *Server) Stop() error {
	s.HTTPServer.Close()
	return s.APIServer.Stop()
}
