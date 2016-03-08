// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"net/http/httptest"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmstore.v5-unstable"

	"github.com/juju/juju/apiserver/charmrevisionupdater"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

// CharmSuite provides infrastructure to set up and perform tests associated
// with charm versioning. A testing charm store server is created and populated
// with some known charms used for testing.
type CharmSuite struct {
	jcSuite *jujutesting.JujuConnSuite

	Handler charmstore.HTTPCloseHandler
	Server  *httptest.Server
	Client  *csclient.Client
	charms  map[string]*state.Charm
}

func (s *CharmSuite) SetUpSuite(c *gc.C, jcSuite *jujutesting.JujuConnSuite) {
	s.jcSuite = jcSuite
}

func (s *CharmSuite) TearDownSuite(c *gc.C) {}

func (s *CharmSuite) SetUpTest(c *gc.C) {
	db := s.jcSuite.Session.DB("juju-testing")
	params := charmstore.ServerParams{
		AuthUsername: "test-user",
		AuthPassword: "test-password",
	}
	handler, err := charmstore.NewServer(db, nil, "", params, charmstore.V4)
	c.Assert(err, jc.ErrorIsNil)
	s.Handler = handler
	s.Server = httptest.NewServer(handler)
	s.Client = csclient.New(csclient.Params{
		URL:      s.Server.URL,
		User:     params.AuthUsername,
		Password: params.AuthPassword,
	})
	urls := map[string]string{
		"mysql":     "quantal/mysql-23",
		"dummy":     "quantal/dummy-24",
		"riak":      "quantal/riak-25",
		"wordpress": "quantal/wordpress-26",
		"logging":   "quantal/logging-27",
	}
	for name, url := range urls {
		testcharms.UploadCharm(c, s.Client, url, name)
	}
	s.jcSuite.PatchValue(&charmrepo.CacheDir, c.MkDir())
	// Patch the charm repo initializer function: it is replaced with a charm
	// store repo pointing to the testing server.
	s.jcSuite.PatchValue(&charmrevisionupdater.NewCharmStore, func(p charmrepo.NewCharmStoreParams) *charmrepo.CharmStore {
		p.URL = s.Server.URL
		return charmrepo.NewCharmStore(p)
	})
	s.charms = make(map[string]*state.Charm)
}

func (s *CharmSuite) TearDownTest(c *gc.C) {
	s.Handler.Close()
	s.Server.Close()
}

// AddMachine adds a new machine to state.
func (s *CharmSuite) AddMachine(c *gc.C, machineId string, job state.MachineJob) {
	m, err := s.jcSuite.State.AddOneMachine(state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{job},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Id(), gc.Equals, machineId)
	cons, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	inst, hc := jujutesting.AssertStartInstanceWithConstraints(c, s.jcSuite.Environ, m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
}

// AddCharmWithRevision adds a charm with the specified revision to state.
func (s *CharmSuite) AddCharmWithRevision(c *gc.C, charmName string, rev int) *state.Charm {
	ch := testcharms.Repo.CharmDir(charmName)
	name := ch.Meta().Name
	curl := charm.MustParseURL(fmt.Sprintf("cs:quantal/%s-%d", name, rev))
	dummy, err := s.jcSuite.State.AddCharm(ch, curl, "dummy-path", fmt.Sprintf("%s-%d-sha256", name, rev))
	c.Assert(err, jc.ErrorIsNil)
	s.charms[name] = dummy
	return dummy
}

// AddService adds a service for the specified charm to state.
func (s *CharmSuite) AddService(c *gc.C, charmName, serviceName string, networks []string) {
	ch, ok := s.charms[charmName]
	c.Assert(ok, jc.IsTrue)
	owner := s.jcSuite.AdminUserTag(c)
	_, err := s.jcSuite.State.AddService(state.AddServiceArgs{Name: serviceName, Owner: owner.String(), Charm: ch, Networks: networks})
	c.Assert(err, jc.ErrorIsNil)
}

// AddUnit adds a new unit for service to the specified machine.
func (s *CharmSuite) AddUnit(c *gc.C, serviceName, machineId string) {
	svc, err := s.jcSuite.State.Service(serviceName)
	c.Assert(err, jc.ErrorIsNil)
	u, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.jcSuite.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
}

// SetUnitRevision sets the unit's charm to the specified revision.
func (s *CharmSuite) SetUnitRevision(c *gc.C, unitName string, rev int) {
	u, err := s.jcSuite.State.Unit(unitName)
	c.Assert(err, jc.ErrorIsNil)
	svc, err := u.Service()
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL(fmt.Sprintf("cs:quantal/%s-%d", svc.Name(), rev))
	err = u.SetCharmURL(curl)
	c.Assert(err, jc.ErrorIsNil)
}

// SetupScenario adds some machines and services to state.
// It assumes a controller machine has already been created.
func (s *CharmSuite) SetupScenario(c *gc.C) {
	s.AddMachine(c, "1", state.JobHostUnits)
	s.AddMachine(c, "2", state.JobHostUnits)
	s.AddMachine(c, "3", state.JobHostUnits)

	// mysql is out of date
	s.AddCharmWithRevision(c, "mysql", 22)
	s.AddService(c, "mysql", "mysql", nil)
	s.AddUnit(c, "mysql", "1")

	// wordpress is up to date
	s.AddCharmWithRevision(c, "wordpress", 26)
	s.AddService(c, "wordpress", "wordpress", nil)
	s.AddUnit(c, "wordpress", "2")
	s.AddUnit(c, "wordpress", "2")
	// wordpress/0 has a version, wordpress/1 is unknown
	s.SetUnitRevision(c, "wordpress/0", 26)

	// varnish is a charm that does not have a version in the mock store.
	s.AddCharmWithRevision(c, "varnish", 5)
	s.AddService(c, "varnish", "varnish", nil)
	s.AddUnit(c, "varnish", "3")
}
