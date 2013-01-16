package openstack_test

import (
	. "launchpad.net/gocheck"
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

type localLiveSuite struct {
	LiveTests
	// The following attributes are for using the service doubles.
	Server         *httptest.Server
	Mux            *http.ServeMux
	oldHandler     http.Handler
	identityDouble http.Handler
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
	// Create the identity service.
	s.identityDouble = identityservice.NewUserPass()
	token := s.identityDouble.(*identityservice.UserPass).AddUser(s.cred.User, s.cred.Secrets)
	s.Mux.Handle(baseIdentityURL, s.identityDouble)

	// Register Swift endpoints with identity service.
	ep := identityservice.Endpoint{
		AdminURL:    s.Server.URL + baseSwiftURL,
		InternalURL: s.Server.URL + baseSwiftURL,
		PublicURL:   s.Server.URL + baseSwiftURL,
		Region:      s.cred.Region,
	}
	service := identityservice.Service{"swift", "object-store", []identityservice.Endpoint{ep}}
	s.identityDouble.(*identityservice.UserPass).AddService(service)
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
	s.identityDouble.(*identityservice.UserPass).AddService(service)
	s.novaDouble = novaservice.New("localhost", "V1", token, "1")
	s.novaDouble.SetupHTTP(s.Mux)

	// Set up the Openstack tests to use our fake credentials.
	attrs := makeTestConfig()
	attrs["username"] = s.cred.User
	attrs["password"] = s.cred.Secrets
	attrs["region"] = s.cred.Region
	attrs["auth-url"] = s.cred.URL
	s.LiveTests.Config = attrs
	s.LiveTests.SetUpSuite(c)
}

func (s *localLiveSuite) TearDownSuite(c *C) {
	s.LiveTests.TearDownSuite(c)
	s.Mux = nil
	s.Server.Config.Handler = s.oldHandler
	if s.Server != nil {
		s.Server.Close()
	}
}

func (s *localLiveSuite) SetUpTest(c *C) {
	s.LiveTests.SetUpTest(c)
}

func (s *localLiveSuite) TearDownTest(c *C) {
	s.LiveTests.TearDownTest(c)
}
