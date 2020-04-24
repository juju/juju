// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"

	"github.com/juju/charm/v7"
	"github.com/juju/charmrepo/v5/csclient"
	csparams "github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	jujucharmstore "github.com/juju/juju/charmstore"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/version"
)

type store interface {
	Latest(channel csparams.Channel, ids []*charm.URL, headers map[string][]string) ([]csparams.CharmRevision, error)
	ListResources(channel csparams.Channel, id *charm.URL) ([]csparams.Resource, error)
	GetResource(channel csparams.Channel, id *charm.URL, name string, revision int) (csclient.ResourceData, error)
	ResourceMeta(channel csparams.Channel, id *charm.URL, name string, revision int) (csparams.Resource, error)
	ServerURL() string
}

type mockStore struct {
	store
	*jtesting.CallMocker
	errors map[string]error
}

func (m *mockStore) UploadCharm(url string, headers map[string][]string) {
	curl := charm.MustParseURL(url)
	rev := curl.Revision
	curl.Revision = -1
	err := m.errors[curl.Name]
	m.Call("Latest", csparams.NoChannel, curl, headers).Returns(
		rev,
		err,
	)
}

func (m *mockStore) Latest(channel csparams.Channel, ids []*charm.URL, headers map[string][]string) ([]csparams.CharmRevision, error) {
	var revs []csparams.CharmRevision
	for _, id := range ids {
		curl := *id
		curl.Revision = -1
		var rev csparams.CharmRevision
		results := m.MethodCall(m, "Latest", channel, &curl, headers)
		if results == nil {
			rev.Err = errors.NotFoundf("charm %s", curl)
		} else {
			rev.Revision = results[0].(int)
		}
		revs = append(revs, rev)
	}
	return revs, nil
}

func (m *mockStore) ListResources(channel csparams.Channel, id *charm.URL) ([]csparams.Resource, error) {
	return nil, nil
}

// CharmSuite provides infrastructure to set up and perform tests associated
// with charm versioning. A testing charm store server is created and populated
// with some known charms used for testing.
type CharmSuite struct {
	jcSuite *jujutesting.JujuConnSuite

	charms map[string]*state.Charm
	store  *mockStore
}

func (s *CharmSuite) SetUpSuite(c *gc.C, jcSuite *jujutesting.JujuConnSuite) {
	s.jcSuite = jcSuite
}

func (s *CharmSuite) SetUpTest(c *gc.C) {
	urls := map[string]string{
		"mysql":     "quantal/mysql-23",
		"dummy":     "quantal/dummy-24",
		"riak":      "quantal/riak-25",
		"wordpress": "quantal/wordpress-26",
		"logging":   "quantal/logging-27",
	}
	var logger loggo.Logger
	s.store = &mockStore{
		CallMocker: jtesting.NewCallMocker(logger),
		errors:     make(map[string]error),
	}
	model, err := s.jcSuite.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	cloud, err := s.jcSuite.State.Cloud(model.CloudName())
	c.Assert(err, jc.ErrorIsNil)
	headers := []string{
		"arch=amd64", // This is the architecture of the deployed applications.
		"cloud=" + model.CloudName(),
		"cloud_region=" + model.CloudRegion(),
		"controller_uuid=" + s.jcSuite.State.ControllerUUID(),
		"controller_version=" + version.Current.String(),
		"environment_uuid=" + model.UUID(),
		"is_controller=true",
		"model_uuid=" + model.UUID(),
		"provider=" + cloud.Type,
		"series=quantal",
	}
	for _, url := range urls {
		s.store.UploadCharm(url, map[string][]string{
			"Juju-Metadata": headers,
		})
	}
	// Patch the charm repo initializer function: it is replaced with a charm
	// store repo pointing to the testing server.
	s.jcSuite.PatchValue(&charmrevisionupdater.NewCharmStoreClient, func(st *state.State) (jujucharmstore.Client, error) {
		return jujucharmstore.NewCustomClient(s.store), nil
	})
	s.charms = make(map[string]*state.Charm)
}

func (s *CharmSuite) SetStoreError(name string, err error) {
	s.store.errors[name] = err
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
	controllerCfg, err := s.jcSuite.State.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	inst, hc := jujutesting.AssertStartInstanceWithConstraints(c, s.jcSuite.Environ, s.jcSuite.ProviderCallContext, controllerCfg.ControllerUUID(), m.Id(), cons)
	err = m.SetProvisioned(inst.Id(), "", "fake_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)
}

// AddCharmWithRevision adds a charm with the specified revision to state.
func (s *CharmSuite) AddCharmWithRevision(c *gc.C, charmName string, rev int) *state.Charm {
	ch := testcharms.Repo.CharmDir(charmName)
	name := ch.Meta().Name
	curl := charm.MustParseURL(fmt.Sprintf("cs:quantal/%s-%d", name, rev))
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-path",
		SHA256:      fmt.Sprintf("%s-%d-sha256", name, rev),
	}
	dummy, err := s.jcSuite.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	s.charms[name] = dummy
	return dummy
}

// AddService adds a service for the specified charm to state.
func (s *CharmSuite) AddService(c *gc.C, charmName, serviceName string) {
	ch, ok := s.charms[charmName]
	c.Assert(ok, jc.IsTrue)
	_, err := s.jcSuite.State.AddApplication(state.AddApplicationArgs{Name: serviceName, Charm: ch})
	c.Assert(err, jc.ErrorIsNil)
}

// AddUnit adds a new unit for application to the specified machine.
func (s *CharmSuite) AddUnit(c *gc.C, serviceName, machineId string) {
	svc, err := s.jcSuite.State.Application(serviceName)
	c.Assert(err, jc.ErrorIsNil)
	u, err := svc.AddUnit(state.AddUnitParams{})
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
	svc, err := u.Application()
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
	s.AddService(c, "mysql", "mysql")
	s.AddUnit(c, "mysql", "1")

	// wordpress is up to date
	s.AddCharmWithRevision(c, "wordpress", 26)
	s.AddService(c, "wordpress", "wordpress")
	s.AddUnit(c, "wordpress", "2")
	s.AddUnit(c, "wordpress", "2")
	// wordpress/0 has a version, wordpress/1 is unknown
	s.SetUnitRevision(c, "wordpress/0", 26)

	// varnish is a charm that does not have a version in the mock store.
	s.AddCharmWithRevision(c, "varnish", 5)
	s.AddService(c, "varnish", "varnish")
	s.AddUnit(c, "varnish", "3")
}
