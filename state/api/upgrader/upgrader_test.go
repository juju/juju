// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/upgrader"
	"launchpad.net/juju-core/state/apiserver"
	coretesting "launchpad.net/juju-core/testing"
	stdtesting "testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type upgraderSuite struct {
	testing.JujuConnSuite

	server   *apiserver.Server
	stateAPI *api.State

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	rawCharm   *state.Charm
	rawService *state.Service
	rawUnit    *state.Unit

	upgrader *upgrader.Upgrader
}

var _ = Suite(&upgraderSuite{})

func defaultPassword(stm *state.Machine) string {
	return stm.Tag() + " password"
}

// Dial options with no timeouts and no retries
var fastDialOpts = api.DialOpts{}

func charmURL(revision int) *charm.URL {
	return charm.MustParseURL("cs:series/wordpress").WithRevision(revision)
}

// Grab a charm, create the service, add a unit for that service
func (s *upgraderSuite) createUnit(c *C) {
	var err error
	s.rawCharm, err = s.State.Charm(charmURL(0))
	c.Assert(err, IsNil)
	s.rawService, err = s.State.AddService("service-name", s.rawCharm)
	c.Assert(err, IsNil)
	s.rawUnit, err = s.rawService.AddUnit()
	c.Assert(err, IsNil)
}

func (s *upgraderSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine to work with
	var err error
	s.rawMachine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.rawMachine.SetPassword(defaultPassword(s.rawMachine))
	c.Assert(err, IsNil)

	s.createUnit(c)

	// Start the testing API server.
	s.server, err = apiserver.NewServer(
		s.State,
		"localhost:12345",
		[]byte(coretesting.ServerCert),
		[]byte(coretesting.ServerKey),
	)
	c.Assert(err, IsNil)

	// Login as the machine agent of the created machine.
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, IsNil)
	info.Tag = s.rawMachine.Tag()
	info.Password = defaultPassword(s.rawMachine)
	c.Logf("opening state; entity %q, password %q", info.Tag, info.Password)
	s.stateAPI, err = api.Open(info, fastDialOpts)
	c.Assert(err, IsNil)
	c.Assert(s.stateAPI, NotNil)

	// Create the upgrader facade.
	s.upgrader, err = s.stateAPI.Upgrader()
	c.Assert(err, IsNil)
	c.Assert(s.upgrader, NotNil)
}

func (s *upgraderSuite) TearDownTest(c *C) {
	if s.stateAPI != nil {
		err := s.stateAPI.Close()
		c.Assert(err, IsNil)
	}
	if s.server != nil {
		err := s.server.Stop()
		c.Assert(err, IsNil)
	}
	s.JujuConnSuite.TearDownTest(c)
}

// Note: This is really meant as a unit-test, this isn't a test that should
//       need all of the setup we have for this test suite
func (s *upgraderSuite) TestNew(c *C) {
	upgrader := upgrader.New(s.stateAPI)
	c.Assert(upgrader, NotNil)
}

func (s *upgraderSuite) TestVerifiesAuth(c *C) {
}
