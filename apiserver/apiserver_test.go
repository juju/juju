// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/clock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/worker/v2/dependency"
	"github.com/juju/worker/v2/workertest"
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
	"github.com/juju/juju/core/presence"
	psapiserver "github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/pubsub/centralhub"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/modelcache"
	"github.com/juju/juju/worker/multiwatcher"
)

const (
	ownerPassword = "very very secret"
)

// apiserverConfigFixture provides a complete, valid, apiserver.Config.
// Unfortunately this also means that it requires State, at least until
// we update the tests to stop expecting state-based authentication.
//
// apiserverConfigFixture does not run an API server; see apiserverBaseSuite
// for that.
type apiserverConfigFixture struct {
	statetesting.StateSuite
	authenticator *stateauthenticator.Authenticator
	mux           *apiserverhttp.Mux
	tlsConfig     *tls.Config
	config        apiserver.ServerConfig

	controller *cache.Controller
}

func (s *apiserverConfigFixture) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	authenticator, err := stateauthenticator.NewAuthenticator(s.StatePool, clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.authenticator = authenticator
	s.mux = apiserverhttp.NewMux()

	certPool, err := api.CreateCertPool(coretesting.CACert)
	if err != nil {
		panic(err)
	}
	s.tlsConfig = api.NewTLSConfig(certPool)
	s.tlsConfig.ServerName = "juju-apiserver"
	s.tlsConfig.Certificates = []tls.Certificate{*coretesting.ServerTLSCert}
	s.mux = apiserverhttp.NewMux()

	multiWatcherWorker, err := multiwatcher.NewWorker(multiwatcher.Config{
		Logger:               loggo.GetLogger("test"),
		Backing:              state.NewAllWatcherBacking(s.StatePool),
		PrometheusRegisterer: noopRegisterer{},
	})
	// The worker itself is a coremultiwatcher.Factory.
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, multiWatcherWorker) })

	machineTag := names.NewMachineTag("0")
	hub := centralhub.New(machineTag)

	initialized := gate.NewLock()
	modelCache, err := modelcache.NewWorker(modelcache.Config{
		StatePool:            s.StatePool,
		Hub:                  hub,
		InitializedGate:      initialized,
		Logger:               loggo.GetLogger("test"),
		WatcherFactory:       multiWatcherWorker.WatchController,
		PrometheusRegisterer: noopRegisterer{},
		Cleanup:              func() {},
	}.WithDefaultRestartStrategy())
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, modelCache) })

	select {
	case <-initialized.Unlocked():
	case <-time.After(10 * time.Second):
		c.Error("model cache not initialized after 10 seconds")
	}

	var controller *cache.Controller
	err = modelcache.ExtractCacheController(modelCache, &controller)
	c.Assert(err, jc.ErrorIsNil)

	s.config = apiserver.ServerConfig{
		StatePool:           s.StatePool,
		Controller:          controller,
		MultiwatcherFactory: multiWatcherWorker,
		Authenticator:       s.authenticator,
		Clock:               clock.WallClock,
		GetAuditConfig:      func() auditlog.Config { return auditlog.Config{} },
		Tag:                 machineTag,
		DataDir:             c.MkDir(),
		LogDir:              c.MkDir(),
		Hub:                 hub,
		Presence:            presence.New(clock.WallClock),
		LeaseManager:        apitesting.StubLeaseManager{},
		Mux:                 s.mux,
		NewObserver:         func() observer.Observer { return &fakeobserver.Instance{} },
		UpgradeComplete:     func() bool { return true },
		RestoreStatus: func() state.RestoreStatus {
			return state.RestoreNotActive
		},
		RegisterIntrospectionHandlers: func(f func(path string, h http.Handler)) {
			f("navel", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, "gazing")
			}))
		},
		MetricsCollector: apiserver.NewMetricsCollector(),
	}
}

// apiserverBaseSuite runs an API server.
type apiserverBaseSuite struct {
	apiserverConfigFixture
	server    *httptest.Server
	apiServer *apiserver.Server
	baseURL   *url.URL
}

func (s *apiserverBaseSuite) SetUpTest(c *gc.C) {
	s.apiserverConfigFixture.SetUpTest(c)

	s.server = httptest.NewUnstartedServer(s.mux)
	s.server.TLS = s.tlsConfig
	s.server.StartTLS()
	s.AddCleanup(func(c *gc.C) { s.server.Close() })
	baseURL, err := url.Parse(s.server.URL)
	c.Assert(err, jc.ErrorIsNil)
	s.baseURL = baseURL
	c.Logf("started HTTP server listening on %q", s.server.Listener.Addr())

	server, err := apiserver.NewServer(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, server)
	})
	s.apiServer = server

	loggo.GetLogger("juju.apiserver").SetLogLevel(loggo.TRACE)
	u, err := s.State.User(s.Owner)
	c.Assert(err, jc.ErrorIsNil)
	err = u.SetPassword(ownerPassword)
	c.Assert(err, jc.ErrorIsNil)
}

// URL returns a URL for this server with the given path and
// query parameters. The URL scheme will be "https".
func (s *apiserverBaseSuite) URL(path string, queryParams url.Values) *url.URL {
	url := *s.baseURL
	url.Path = path
	url.RawQuery = queryParams.Encode()
	return &url
}

// sendHTTPRequest sends an HTTP request with an appropriate
// username and password.
func (s *apiserverBaseSuite) sendHTTPRequest(c *gc.C, p apitesting.HTTPRequestParams) *http.Response {
	p.Tag = s.Owner.String()
	p.Password = ownerPassword
	return apitesting.SendHTTPRequest(c, p)
}

func (s *apiserverBaseSuite) newServerNoCleanup(c *gc.C, config apiserver.ServerConfig) *apiserver.Server {
	// To ensure we don't get two servers using the same mux (in which
	// case the original api server always handles requests), ensure
	// the original one is stopped.
	s.apiServer.Kill()
	err := s.apiServer.Wait()
	c.Assert(err, jc.ErrorIsNil)
	srv, err := apiserver.NewServer(config)
	c.Assert(err, jc.ErrorIsNil)
	return srv
}

func (s *apiserverBaseSuite) newServer(c *gc.C, config apiserver.ServerConfig) *apiserver.Server {
	srv := s.newServerNoCleanup(c, config)
	s.AddCleanup(func(c *gc.C) {
		workertest.CleanKill(c, srv)
	})
	return srv
}

func (s *apiserverBaseSuite) newServerDirtyKill(c *gc.C, config apiserver.ServerConfig) *apiserver.Server {
	srv := s.newServerNoCleanup(c, config)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, srv)
	})
	return srv
}

// APIInfo returns an info struct that has the server's address and ca-cert
// populated.
func (s *apiserverBaseSuite) APIInfo(server *apiserver.Server) *api.Info {
	address := s.server.Listener.Addr().String()
	return &api.Info{
		Addrs:  []string{address},
		CACert: coretesting.CACert,
	}
}

func (s *apiserverBaseSuite) openAPIAs(c *gc.C, srv *apiserver.Server, tag names.Tag, password, nonce string, controllerOnly bool) api.Connection {
	apiInfo := s.APIInfo(srv)
	apiInfo.Tag = tag
	apiInfo.Password = password
	apiInfo.Nonce = nonce
	if !controllerOnly {
		apiInfo.ModelTag = s.Model.ModelTag()
	}
	conn, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conn, gc.NotNil)
	s.AddCleanup(func(c *gc.C) {
		conn.Close()
	})
	return conn
}

// OpenAPIAsNewMachine creates a new client connection logging in as the
// controller owner. The returned api.Connection should not be closed by the
// caller as a cleanup function has been registered to do that.
func (s *apiserverBaseSuite) OpenAPIAsAdmin(c *gc.C, srv *apiserver.Server) api.Connection {
	return s.openAPIAs(c, srv, s.Owner, ownerPassword, "", false)
}

// OpenAPIAsNewMachine creates a new machine entry that lives in system state,
// and then uses that to open the API. The returned api.Connection should not be
// closed by the caller as a cleanup function has been registered to do that.
// The machine will run the supplied jobs; if none are given, JobHostUnits is assumed.
func (s *apiserverBaseSuite) OpenAPIAsNewMachine(c *gc.C, srv *apiserver.Server, jobs ...state.MachineJob) (api.Connection, *state.Machine) {
	if len(jobs) == 0 {
		jobs = []state.MachineJob{state.JobHostUnits}
	}
	machine, err := s.State.AddMachine("quantal", jobs...)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	return s.openAPIAs(c, srv, machine.Tag(), password, "fake_nonce", false), machine
}

func dialWebsocketFromURL(c *gc.C, server string, header http.Header) (*websocket.Conn, *http.Response, error) {
	// TODO(rogpeppe) merge this with the very similar dialWebsocket function.
	if header == nil {
		header = http.Header{}
	}
	header.Set("Origin", "http://localhost/")
	caCerts := x509.NewCertPool()
	c.Assert(caCerts.AppendCertsFromPEM([]byte(coretesting.CACert)), jc.IsTrue)
	tlsConfig := utils.SecureTLSConfig()
	tlsConfig.RootCAs = caCerts
	tlsConfig.ServerName = "juju-apiserver"

	dialer := &websocket.Dialer{
		TLSClientConfig: tlsConfig,
	}
	return dialer.Dial(server, header)
}

type apiserverSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&apiserverSuite{})

func (s *apiserverSuite) TestCleanStop(c *gc.C) {
	workertest.CleanKill(c, s.apiServer)
}

func (s *apiserverSuite) TestRestartMessage(c *gc.C) {
	_, err := s.config.Hub.Publish(psapiserver.RestartTopic, psapiserver.Restart{
		LocalOnly: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = workertest.CheckKilled(c, s.apiServer)
	c.Assert(err, gc.Equals, dependency.ErrBounce)
}

func (s *apiserverSuite) getHealth(c *gc.C) (string, int) {
	uri := s.server.URL + "/health"
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	result := string(body)
	// Ensure that the last value is a carriage return.
	c.Assert(strings.HasSuffix(result, "\n"), jc.IsTrue)
	return strings.TrimSuffix(result, "\n"), resp.StatusCode
}

func (s *apiserverSuite) TestHealthRunning(c *gc.C) {
	health, statusCode := s.getHealth(c)
	c.Assert(health, gc.Equals, "running")
	c.Assert(statusCode, gc.Equals, http.StatusOK)
}

func (s *apiserverSuite) TestHealthStopping(c *gc.C) {
	wg := apiserver.ServerWaitGroup(s.apiServer)
	wg.Add(1)

	s.apiServer.Kill()
	// There is a race here between the test and the goroutine setting
	// the value, so loop until we see the right health, then exit.
	timeout := time.After(testing.LongWait)
	for {
		health, statusCode := s.getHealth(c)
		if health == "stopping" {
			// Expected, we're done.
			c.Assert(statusCode, gc.Equals, http.StatusServiceUnavailable)
			wg.Done()
			return
		}
		select {
		case <-timeout:
			c.Fatalf("health not set to stopping")
		case <-time.After(testing.ShortWait):
			// Look again.
		}
	}
}
