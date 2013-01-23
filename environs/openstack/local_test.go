package openstack_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/testservices/identityservice"
	"launchpad.net/goose/testservices/novaservice"
	"launchpad.net/goose/testservices/swiftservice"
	"net/http"
	"net/http/httptest"
)

const (
	baseIdentityURL = "/tokens"
	baseNovaURL     = "/V1/1"
	baseSwiftURL    = "/object-store"
)

// Register tests to run against a test Openstack instance (service doubles).
func registerServiceDoubleTests() {
	cred := &identity.Credentials{
		User:    "fred",
		Secrets: "secret",
		Region:  "some region"}
	Suite(&localLiveSuite{
		LiveTests: LiveTests{
			cred: cred,
		},
	})
}

type localLiveSuite struct {
	LiveTests
	// The following attributes are for using the service doubles.
	Server         *httptest.Server
	Mux            *http.ServeMux
	oldHandler     http.Handler
	identityDouble *identityservice.UserPass
	novaDouble     *novaservice.Nova
	swiftDouble    http.Handler
}

func (s *localLiveSuite) SetUpSuite(c *C) {
	c.Logf("Using openstack service test doubles")

	// Set up the HTTP server.
	s.Server = httptest.NewServer(nil)
	s.oldHandler = s.Server.Config.Handler
	s.Mux = http.NewServeMux()
	s.Server.Config.Handler = s.Mux

	s.cred.URL = s.Server.URL
	s.cred.TenantName = "tenant"
	// Create the identity service.
	s.identityDouble = identityservice.NewUserPass()
	token := s.identityDouble.AddUser(s.cred.User, s.cred.Secrets)
	s.Mux.Handle(baseIdentityURL, s.identityDouble)

	// Register Swift endpoints with identity service.
	ep := identityservice.Endpoint{
		AdminURL:    s.Server.URL + baseSwiftURL,
		InternalURL: s.Server.URL + baseSwiftURL,
		PublicURL:   s.Server.URL + baseSwiftURL,
		Region:      s.cred.Region,
	}
	service := identityservice.Service{"swift", "object-store", []identityservice.Endpoint{ep}}
	s.identityDouble.AddService(service)
	s.swiftDouble = swiftservice.New("localhost", baseSwiftURL+"/", token)
	s.Mux.Handle(baseSwiftURL+"/", s.swiftDouble)

	// Register Nova endpoints with identity service.
	ep = identityservice.Endpoint{
		AdminURL:    s.Server.URL + baseNovaURL,
		InternalURL: s.Server.URL + baseNovaURL,
		PublicURL:   s.Server.URL + baseNovaURL,
		Region:      s.cred.Region,
	}
	service = identityservice.Service{"nova", "compute", []identityservice.Endpoint{ep}}
	s.identityDouble.AddService(service)
	s.novaDouble = novaservice.New("localhost", "V1", token, "1")
	s.novaDouble.SetupHTTP(s.Mux)

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
