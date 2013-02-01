package openstack_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/testservices/openstackservice"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/openstack"
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

	openstack.ShortTimeouts(true)
	// Set up the HTTP server.
	s.Server = httptest.NewServer(nil)
	s.oldHandler = s.Server.Config.Handler
	s.Mux = http.NewServeMux()
	s.Server.Config.Handler = s.Mux

	s.cred.URL = s.Server.URL
	srv := openstackservice.New(s.cred)
	srv.SetupHTTP(s.Mux)

	s.LiveTests.SetUpSuite(c, srv)
}

func (s *localLiveSuite) TearDownSuite(c *C) {
	s.LiveTests.TearDownSuite(c)
	s.Mux = nil
	s.Server.Config.Handler = s.oldHandler
	s.Server.Close()
	openstack.ShortTimeouts(false)
}

func (s *localLiveSuite) SetUpTest(c *C) {
	s.LiveTests.SetUpTest(c)
}

func (s *localLiveSuite) TearDownTest(c *C) {
	s.LiveTests.TearDownTest(c)
}

// ported from lp:juju/juju/providers/openstack/tests/test_machine.py
var addressTests = []struct {
	summary  string
	private  []nova.IPAddress
	public   []nova.IPAddress
	networks []string
	expected string
	failure  error
}{
	{
		summary:  "missing",
		expected: "",
		failure:  environs.ErrNoDNSName,
	},
	{
		summary:  "empty",
		private:  []nova.IPAddress{},
		networks: []string{"private"},
		expected: "",
		failure:  environs.ErrNoDNSName,
	},
	{
		summary:  "private only",
		private:  []nova.IPAddress{{4, "127.0.0.4"}},
		networks: []string{"private"},
		expected: "127.0.0.4",
		failure:  nil,
	},
	{
		summary:  "private plus (HP cloud)",
		private:  []nova.IPAddress{{4, "127.0.0.4"}, {4, "8.8.4.4"}},
		networks: []string{"private"},
		expected: "8.8.4.4",
		failure:  nil,
	},
	{
		summary:  "public only",
		public:   []nova.IPAddress{{4, "8.8.8.8"}},
		networks: []string{"", "public"},
		expected: "8.8.8.8",
		failure:  nil,
	},
	{
		summary:  "public and private",
		private:  []nova.IPAddress{{4, "127.0.0.4"}},
		public:   []nova.IPAddress{{4, "8.8.4.4"}},
		networks: []string{"private", "public"},
		expected: "8.8.4.4",
		failure:  nil,
	},
	{
		summary:  "public private plus",
		private:  []nova.IPAddress{{4, "127.0.0.4"}, {4, "8.8.4.4"}},
		public:   []nova.IPAddress{{4, "8.8.8.8"}},
		networks: []string{"private", "public"},
		expected: "8.8.8.8",
		failure:  nil,
	},
	{
		summary:  "custom only",
		private:  []nova.IPAddress{{4, "127.0.0.2"}},
		networks: []string{"special"},
		expected: "127.0.0.2",
		failure:  nil,
	},
	{
		summary:  "custom and public",
		private:  []nova.IPAddress{{4, "127.0.0.2"}},
		public:   []nova.IPAddress{{4, "8.8.8.8"}},
		networks: []string{"special", "public"},
		expected: "8.8.8.8",
		failure:  nil,
	},
	{
		summary:  "non-IPv4",
		private:  []nova.IPAddress{{6, "::dead:beef:f00d"}},
		networks: []string{"private"},
		expected: "",
		failure:  environs.ErrNoDNSName,
	},
}

func (s *LiveTests) TestGetServerAddresses(c *C) {
	for i, t := range addressTests {
		c.Logf("#%d. %s -> %s (%v)", i, t.summary, t.expected, t.failure)
		addresses := make(map[string][]nova.IPAddress)
		if t.private != nil {
			if len(t.networks) < 1 {
				addresses["private"] = t.private
			} else {
				addresses[t.networks[0]] = t.private
			}
		}
		if t.public != nil {
			if len(t.networks) < 2 {
				addresses["public"] = t.public
			} else {
				addresses[t.networks[1]] = t.public
			}
		}
		addr, err := openstack.InstanceAddress(addresses)
		c.Assert(err, Equals, t.failure)
		c.Assert(addr, Equals, t.expected)
	}
}
