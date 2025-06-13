// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testserver

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"

	"github.com/juju/clock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
	"github.com/juju/juju/apiserver/stateauthenticator"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/cache"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/raft/queue"
	"github.com/juju/juju/pubsub/centralhub"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

// DefaultServerConfig returns the default configuration for starting a test server.
func DefaultServerConfig(c *gc.C, testclock clock.Clock) apiserver.ServerConfig {
	if testclock == nil {
		testclock = clock.WallClock
	}
	fakeOrigin := names.NewMachineTag("0")
	hub := centralhub.New(fakeOrigin, centralhub.PubsubNoOpMetrics{})
	return apiserver.ServerConfig{
		Clock:               testclock,
		Tag:                 names.NewMachineTag("0"),
		LogDir:              c.MkDir(),
		Hub:                 hub,
		Controller:          &cache.Controller{}, // Not useful for anything except providing a default.
		MultiwatcherFactory: &fakeMultiwatcherFactory{},
		Presence:            presence.New(testclock),
		LeaseManager:        apitesting.StubLeaseManager{},
		NewObserver:         func() observer.Observer { return &fakeobserver.Instance{} },
		GetAuditConfig:      func() auditlog.Config { return auditlog.Config{Enabled: false} },
		UpgradeComplete:     func() bool { return true },
		RestoreStatus: func() state.RestoreStatus {
			return state.RestoreNotActive
		},
		MetricsCollector: apiserver.NewMetricsCollector(),
		RaftOpQueue:      queue.NewOpQueue(testclock),
		SysLogger:        noopSysLogger{},
	}
}

type noopSysLogger struct{}

func (noopSysLogger) Log([]corelogger.LogRecord) error { return nil }

// NewServer returns a new running API server using the given state.
// The pool may be nil, in which case a pool using the given state
// will be used.
//
// It returns information suitable for connecting to the state
// without any authentication information or model tag, and the server
// that's been started.
func NewServer(c *gc.C, statePool *state.StatePool, controller *cache.Controller) *Server {
	config := DefaultServerConfig(c, nil)
	config.Controller = controller
	return NewServerWithConfig(c, statePool, config)
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
		c.Fatal(err.Error())
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

	if cfg.MetricsCollector == nil {
		cfg.MetricsCollector = apiserver.NewMetricsCollector()
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

type fakeMultiwatcherFactory struct {
	multiwatcher.Factory
}
