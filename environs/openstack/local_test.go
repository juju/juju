package openstack_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"launchpad.net/goose/nova"
	"launchpad.net/goose/testservices"
	"launchpad.net/goose/testservices/openstackservice"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/openstack"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
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

	Env     environs.Environ
	Service *openstackservice.Openstack
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
	s.Service = openstackservice.New(s.cred)
	s.Service.SetupHTTP(s.Mux)

	attrs := makeTestConfig()
	attrs["admin-secret"] = "secret"
	attrs["username"] = s.cred.User
	attrs["password"] = s.cred.Secrets
	attrs["region"] = s.cred.Region
	attrs["auth-url"] = s.cred.URL
	attrs["tenant-name"] = s.cred.TenantName
	attrs["default-image-id"] = testImageId
	if e, err := environs.NewFromAttrs(attrs); err != nil {
		c.Fatalf("cannot create local test environment: %s", err.Error())
	} else {
		s.Env = e
		putFakeTools(c, openstack.WritablePublicStorage(s.Env))
	}

	s.LiveTests.SetUpSuite(c)
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

func panicWrite(name string, cert, key []byte) error {
	panic("writeCertAndKey called unexpectedly")
}

func (s *localLiveSuite) TestBootstrapFailsWithoutPublicIP(c *C) {
	s.Service.Nova.RegisterControlPoint(
		"addFloatingIP",
		func(sc testservices.ServiceControl, args ...interface{}) error {
			return fmt.Errorf("failed on purpose")
		},
	)
	defer s.Service.Nova.RegisterControlPoint("addFloatingIP", nil)
	writeablePublicStorage := openstack.WritablePublicStorage(s.Env)
	putFakeTools(c, writeablePublicStorage)

	err := environs.Bootstrap(s.Env, true, panicWrite)
	c.Assert(err, ErrorMatches, ".*cannot allocate a public IP as needed.*")
	defer s.Env.Destroy(nil)
}

var instanceGathering = []struct {
	ids []state.InstanceId
	err error
}{
	{ids: []state.InstanceId{"id0"}},
	{ids: []state.InstanceId{"id0", "id0"}},
	{ids: []state.InstanceId{"id0", "id1"}},
	{ids: []state.InstanceId{"id1", "id0"}},
	{ids: []state.InstanceId{"id1", "id0", "id1"}},
	{
		ids: []state.InstanceId{""},
		err: environs.ErrNoInstances,
	},
	{
		ids: []state.InstanceId{"", ""},
		err: environs.ErrNoInstances,
	},
	{
		ids: []state.InstanceId{"", "", ""},
		err: environs.ErrNoInstances,
	},
	{
		ids: []state.InstanceId{"id0", ""},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []state.InstanceId{"", "id1"},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []state.InstanceId{"id0", "id1", ""},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []state.InstanceId{"id0", "", "id0"},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []state.InstanceId{"id0", "id0", ""},
		err: environs.ErrPartialInstances,
	},
	{
		ids: []state.InstanceId{"", "id0", "id1"},
		err: environs.ErrPartialInstances,
	},
}

func (s *localLiveSuite) TestInstancesGathering(c *C) {
	s.BootstrapOnce(c)
	inst0, err := s.Env.StartInstance("100", testing.InvalidStateInfo("100"), testing.InvalidAPIInfo("100"), nil)
	c.Assert(err, IsNil)
	id0 := inst0.Id()
	inst1, err := s.Env.StartInstance("101", testing.InvalidStateInfo("101"), testing.InvalidAPIInfo("101"), nil)
	c.Assert(err, IsNil)
	id1 := inst1.Id()
	defer func() {
		err := s.Env.StopInstances([]environs.Instance{inst0, inst1})
		c.Assert(err, IsNil)
	}()

	for i, test := range instanceGathering {
		c.Logf("test %d: find %v -> expect len %d, err: %v", i, test.ids, len(test.ids), test.err)
		ids := make([]state.InstanceId, len(test.ids))
		for j, id := range test.ids {
			switch id {
			case "id0":
				ids[j] = id0
			case "id1":
				ids[j] = id1
			}
		}
		insts, err := s.Env.Instances(ids)
		c.Assert(err, Equals, test.err)
		if err == environs.ErrNoInstances {
			c.Assert(insts, HasLen, 0)
		} else {
			c.Assert(insts, HasLen, len(test.ids))
		}
		for j, inst := range insts {
			if ids[j] != "" {
				c.Assert(inst.Id(), Equals, ids[j])
			} else {
				c.Assert(inst, IsNil)
			}
		}
	}
}
