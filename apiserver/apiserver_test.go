// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3/dependency"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
	"github.com/juju/juju/apiserver/stateauthenticator"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/websocket/websockettest"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/cache"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/jujuclient"
	psapiserver "github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/pubsub/centralhub"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
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
}

func (s *apiserverConfigFixture) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	authenticator, err := stateauthenticator.NewAuthenticator(s.StatePool, clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.authenticator = authenticator
	s.mux = apiserverhttp.NewMux()

	certPool, err := api.CreateCertPool(testing.CACert)
	if err != nil {
		panic(err)
	}
	s.tlsConfig = api.NewTLSConfig(certPool)
	s.tlsConfig.ServerName = "juju-apiserver"
	s.tlsConfig.Certificates = []tls.Certificate{*testing.ServerTLSCert}
	s.mux = apiserverhttp.NewMux()
	allWatcherBacking, err := state.NewAllWatcherBacking(s.StatePool)
	c.Assert(err, jc.ErrorIsNil)
	multiWatcherWorker, err := multiwatcher.NewWorker(multiwatcher.Config{
		Clock:                clock.WallClock,
		Logger:               loggo.GetLogger("test"),
		Backing:              allWatcherBacking,
		PrometheusRegisterer: noopRegisterer{},
	})
	c.Assert(err, jc.ErrorIsNil)

	// The worker itself is a coremultiwatcher.Factory.
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, multiWatcherWorker) })

	machineTag := names.NewMachineTag("0")
	hub := centralhub.New(machineTag, centralhub.PubsubNoOpMetrics{})

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
		StatePool:                  s.StatePool,
		Controller:                 controller,
		MultiwatcherFactory:        multiWatcherWorker,
		LocalMacaroonAuthenticator: s.authenticator,
		Clock:                      clock.WallClock,
		GetAuditConfig:             func() auditlog.Config { return auditlog.Config{} },
		Tag:                        machineTag,
		DataDir:                    c.MkDir(),
		LogDir:                     c.MkDir(),
		Hub:                        hub,
		Presence:                   presence.New(clock.WallClock),
		LeaseManager:               apitesting.StubLeaseManager{},
		Mux:                        s.mux,
		NewObserver:                func() observer.Observer { return &fakeobserver.Instance{} },
		UpgradeComplete:            func() bool { return true },
		RegisterIntrospectionHandlers: func(f func(path string, h http.Handler)) {
			f("navel", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, "gazing")
			}))
		},
		MetricsCollector: apiserver.NewMetricsCollector(),
		ExecEmbeddedCommand: func(ctx *cmd.Context, store jujuclient.ClientStore, whitelist []string, cmdPlusArgs string) int {
			allowed := set.NewStrings(whitelist...)
			args := strings.Split(cmdPlusArgs, " ")
			if !allowed.Contains(args[0]) {
				fmt.Fprintf(ctx.Stderr, "%q not allowed\n", args[0])
				return 1
			}
			ctrl, err := store.CurrentController()
			if err != nil {
				fmt.Fprintf(ctx.Stderr, "%s", err.Error())
				return 1
			}
			model, err := store.CurrentModel(ctrl)
			if err != nil {
				fmt.Fprintf(ctx.Stderr, "%s", err.Error())
				return 1
			}
			ad, err := store.AccountDetails(ctrl)
			if err != nil {
				fmt.Fprintf(ctx.Stderr, "%s", err.Error())
				return 1
			}
			if strings.Contains(cmdPlusArgs, "macaroon error") {
				fmt.Fprintf(ctx.Stderr, "ERROR: cannot get discharge from https://controller")
				fmt.Fprintf(ctx.Stderr, "\n")
			} else {
				cmdStr := fmt.Sprintf("%s@%s:%s -> %s", ad.User, ctrl, model, cmdPlusArgs)
				fmt.Fprintf(ctx.Stdout, "%s", cmdStr)
				fmt.Fprintf(ctx.Stdout, "\n")
			}
			return 0
		},
		SysLogger: noopSysLogger{},
		DBGetter:  apiserver.StubDBGetter{},
	}
}

type noopSysLogger struct{}

func (noopSysLogger) Log([]corelogger.LogRecord) error { return nil }

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
		CACert: testing.CACert,
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
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), jobs...)
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
	c.Assert(caCerts.AppendCertsFromPEM([]byte(testing.CACert)), jc.IsTrue)
	tlsConfig := jujuhttp.SecureTLSConfig()
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
	body, err := io.ReadAll(resp.Body)
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

func (s *apiserverSuite) TestEmbeddedCommand(c *gc.C) {
	cmdArgs := params.CLICommands{
		User:     "fred",
		Commands: []string{"status --color"},
	}
	s.assertEmbeddedCommand(c, cmdArgs, "fred@interactive:test-admin/testmodel -> status --color", nil)
}

func (s *apiserverSuite) TestEmbeddedCommandNotAllowed(c *gc.C) {
	cmdArgs := params.CLICommands{
		User:     "fred",
		Commands: []string{"bootstrap aws"},
	}
	s.assertEmbeddedCommand(c, cmdArgs, `"bootstrap" not allowed`, nil)
}

func (s *apiserverSuite) TestEmbeddedCommandMissingUser(c *gc.C) {
	cmdArgs := params.CLICommands{
		Commands: []string{"status --color"},
	}
	s.assertEmbeddedCommand(c, cmdArgs, "", &params.Error{Message: `CLI command for anonymous user not supported`, Code: "not supported"})
}

func (s *apiserverSuite) TestEmbeddedCommandInvalidUser(c *gc.C) {
	cmdArgs := params.CLICommands{
		User:     "123@",
		Commands: []string{"status --color"},
	}
	s.assertEmbeddedCommand(c, cmdArgs, "", &params.Error{Message: `user name "123@" not valid`, Code: params.CodeNotValid})
}

func (s *apiserverSuite) TestEmbeddedCommandInvalidMacaroon(c *gc.C) {
	cmdArgs := params.CLICommands{
		User:     "fred",
		Commands: []string{"status macaroon error"},
	}
	s.assertEmbeddedCommand(c, cmdArgs, "", &params.Error{
		Code:    params.CodeDischargeRequired,
		Message: `macaroon discharge required: cannot get discharge from https://controller`})
}

func (s *apiserverSuite) assertEmbeddedCommand(c *gc.C, cmdArgs params.CLICommands, expected string, resultErr *params.Error) {
	address := s.server.Listener.Addr().String()
	path := fmt.Sprintf("/model/%s/commands", s.State.ModelUUID())
	commandURL := &url.URL{
		Scheme: "wss",
		Host:   address,
		Path:   path,
	}
	conn, _, err := dialWebsocketFromURL(c, commandURL.String(), http.Header{})
	c.Assert(err, jc.ErrorIsNil)
	defer conn.Close()

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	done := make(chan struct{})
	var result params.CLICommandStatus
	go func() {
		for {
			var update params.CLICommandStatus
			err := conn.ReadJSON(&update)
			c.Assert(err, jc.ErrorIsNil)

			result.Output = append(result.Output, update.Output...)
			result.Done = update.Done
			result.Error = update.Error
			if result.Done {
				done <- struct{}{}
				break
			}
		}
	}()

	err = conn.WriteJSON(cmdArgs)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("no command result")
	}

	// Close connection.
	err = conn.Close()
	c.Assert(err, jc.ErrorIsNil)

	var expectedOutput []string
	if expected != "" {
		expectedOutput = []string{expected}
	}
	c.Assert(result, jc.DeepEquals, params.CLICommandStatus{
		Output: expectedOutput,
		Done:   true,
		Error:  resultErr,
	})
}
