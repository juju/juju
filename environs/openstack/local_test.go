package openstack_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/testservices/openstack"
	"net/http"
	"net/http/httptest"
)

// Register tests to run against a test Openstack instance (service doubles).
func registerServiceDoubleTests() {
	cred := &identity.Credentials{
		User:       "fred",
		Secrets:    "secret",
		Region:     "some region",
		TenantName: "some tenant",
	}
	Suite(&localLiveSuite{
		LiveTests: LiveTests{
			cred: cred,
		},
	})
}

type localLiveSuite struct {
	LiveTests
	// The following attributes are for using the service doubles.
	Server     *httptest.Server
	Mux        *http.ServeMux
	oldHandler http.Handler
}

func (s *localLiveSuite) SetUpSuite(c *C) {
	c.Logf("Using openstack service test doubles")

	// Set up the HTTP server.
	s.Server = httptest.NewServer(nil)
	s.oldHandler = s.Server.Config.Handler
	s.Mux = http.NewServeMux()
	s.Server.Config.Handler = s.Mux

	s.cred.URL = s.Server.URL
	openstack := openstack.New(s.cred)
	openstack.SetupHTTP(s.Mux)

	s.LiveTests.SetUpSuite(c)
}

func (s *localLiveSuite) TearDownSuite(c *C) {
	s.LiveTests.TearDownSuite(c)
	s.Mux = nil
	s.Server.Config.Handler = s.oldHandler
	s.Server.Close()
}

func (s *localLiveSuite) SetUpTest(c *C) {
	s.LiveTests.SetUpTest(c)
}

func (s *localLiveSuite) TearDownTest(c *C) {
	s.LiveTests.TearDownTest(c)
}
