// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver"
	coretesting "launchpad.net/juju-core/testing"
	"strings"
)

// Most (if not all) of the permission tests below aim to test
// end-to-end operations execution through the API, but do not care
// about the results. They only test that a call is succeeds or fails
// (usually due to "permission denied"). There are separate test cases
// testing each individual API call data flow later on.

var operationPermTests = []struct {
	about string
	// op performs the operation to be tested using the given state
	// connection. It returns a function that should be used to
	// undo any changes made by the operation.
	op    func(c *C, st *api.State, mst *state.State) (reset func(), err error)
	allow []string
	deny  []string
}{{
	about: "Unit.Get",
	op:    opGetUnitWordpress0,
	deny:  []string{"user-admin", "user-other"},
}, {
	about: "Machine.Get",
	op:    opGetMachine1,
	deny:  []string{"user-admin", "user-other"},
}, {
	about: "Machine.SetAgentAlive",
	op:    opMachine1SetAgentAlive,
	allow: []string{"machine-1"},
}, {
	about: "Machine.SetPassword",
	op:    opMachine1SetPassword,
	// Machine 0 is allowed because it is an environment manager.
	allow: []string{"machine-0", "machine-1"},
}, {
	about: "Machine.SetProvisioned",
	op:    opMachine1SetProvisioned,
	allow: []string{"machine-0"},
}, {
	about: "Machine.Constraints",
	op:    opMachine1Constraints,
	// TODO (dimitern): revisit this and relax the restrictions as
	// needed once all agents/tasks are using the API.
	allow: []string{"machine-0"},
}, {
	about: "Machine.Remove",
	op:    opMachine1Remove,
	allow: []string{"machine-0"},
}, {
	about: "Machine.EnsureDead",
	op:    opMachine1EnsureDead,
	// Machine 0 is allowed because it is an environment manager.
	allow: []string{"machine-0", "machine-1"},
}, {
	about: "Machine.Status",
	op:    opMachine1Status,
	// TODO (dimitern): revisit this and relax the restrictions as
	// needed once all agents/tasks are using the API.
	allow: []string{"machine-0"},
}, {
	about: "Machine.SetStatus",
	op:    opMachine1SetStatus,
	// Machine 0 is allowed because it is an environment manager.
	allow: []string{"machine-0", "machine-1"},
}, {
	about: "Unit.SetPassword (on principal unit)",
	op:    opUnitSetPassword("wordpress/0"),
	allow: []string{"unit-wordpress-0", "machine-1"},
}, {
	about: "Unit.SetPassword (on subordinate unit)",
	op:    opUnitSetPassword("logging/0"),
	allow: []string{"unit-logging-0", "unit-wordpress-0"},
}, {
	about: "State.AllMachines",
	op:    opStateAllMachines,
	allow: []string{"machine-0"},
}, {
	about: "Client.Status",
	op:    opClientStatus,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.ServiceSet",
	op:    opClientServiceSet,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.ServiceSetYAML",
	op:    opClientServiceSetYAML,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.ServiceGet",
	op:    opClientServiceGet,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.Resolved",
	op:    opClientResolved,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.ServiceExpose",
	op:    opClientServiceExpose,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.ServiceUnexpose",
	op:    opClientServiceUnexpose,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.ServiceDeploy",
	op:    opClientServiceDeploy,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.GetAnnotations",
	op:    opClientGetAnnotations,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.SetAnnotations",
	op:    opClientSetAnnotations,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.AddServiceUnits",
	op:    opClientAddServiceUnits,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.DestroyServiceUnits",
	op:    opClientDestroyServiceUnits,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.ServiceDestroy",
	op:    opClientServiceDestroy,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.GetServiceConstraints",
	op:    opClientGetServiceConstraints,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.SetServiceConstraints",
	op:    opClientSetServiceConstraints,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.WatchAll",
	op:    opClientWatchAll,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.CharmInfo",
	op:    opClientCharmInfo,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.AddRelation",
	op:    opClientAddRelation,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.DestroyRelation",
	op:    opClientDestroyRelation,
	allow: []string{"user-admin", "user-other"},
},
}

// allowed returns the set of allowed entities given an allow list and a
// deny list.  If an allow list is specified, only those entities are
// allowed; otherwise those in deny are disallowed.
func allowed(all, allow, deny []string) map[string]bool {
	p := make(map[string]bool)
	if allow != nil {
		for _, e := range allow {
			p[e] = true
		}
		return p
	}
loop:
	for _, e0 := range all {
		for _, e1 := range deny {
			if e1 == e0 {
				continue loop
			}
		}
		p[e0] = true
	}
	return p
}

func (s *suite) TestOperationPerm(c *C) {
	entities := s.setUpScenario(c)
	for i, t := range operationPermTests {
		allow := allowed(entities, t.allow, t.deny)
		for _, e := range entities {
			c.Logf("test %d; %s; entity %q", i, t.about, e)
			st := s.openAs(c, e)
			reset, err := t.op(c, st, s.State)
			if allow[e] {
				c.Check(err, IsNil)
			} else {
				c.Check(err, ErrorMatches, "permission denied")
				c.Check(api.ErrCode(err), Equals, api.CodeUnauthorized)
			}
			reset()
			st.Close()
		}
	}
}

func opGetUnitWordpress0(c *C, st *api.State, mst *state.State) (func(), error) {
	u, err := st.Unit("wordpress/0")
	if err != nil {
		c.Check(u, IsNil)
	} else {
		name, ok := u.DeployerTag()
		c.Check(ok, Equals, true)
		c.Check(name, Equals, "machine-1")
	}
	return func() {}, err
}

func opUnitSetPassword(unitName string) func(c *C, st *api.State, mst *state.State) (func(), error) {
	return func(c *C, st *api.State, mst *state.State) (func(), error) {
		u, err := st.Unit(unitName)
		if err != nil {
			c.Check(u, IsNil)
			return func() {}, err
		}
		err = u.SetPassword("another password")
		if err != nil {
			return func() {}, err
		}
		return func() {
			setDefaultPassword(c, u)
		}, nil
	}
}

func opGetMachine1(c *C, st *api.State, mst *state.State) (func(), error) {
	m, err := st.Machine("1")
	if err != nil {
		c.Check(m, IsNil)
	} else {
		name, ok := m.InstanceId()
		c.Assert(ok, Equals, true)
		c.Assert(name, Equals, "i-machine-1")
	}
	return func() {}, err
}

func opMachine1SetPassword(c *C, st *api.State, mst *state.State) (func(), error) {
	m, err := st.Machine("1")
	if err != nil {
		c.Check(m, IsNil)
		return func() {}, err
	}
	err = m.SetPassword("another password")
	if err != nil {
		return func() {}, err
	}
	return func() {
		setDefaultPassword(c, m)
	}, nil
}

func opMachine1SetAgentAlive(c *C, st *api.State, mst *state.State) (func(), error) {
	m, err := st.Machine("1")
	if err != nil {
		c.Check(m, IsNil)
		return func() {}, err
	}
	pinger, err := m.SetAgentAlive()
	if err != nil {
		return func() {}, err
	}
	err = pinger.Stop()
	c.Check(err, IsNil)
	return func() {}, nil
}

func opMachine1SetProvisioned(c *C, st *api.State, mst *state.State) (func(), error) {
	m, err := st.Machine("1")
	if err != nil {
		c.Check(m, IsNil)
		return func() {}, err
	}
	err = m.SetProvisioned("foo", "bar")
	if err != nil && api.ErrCode(err) == api.CodeUnauthorized {
		// We expect this for any entity other than machine-0.
		return func() {}, err
	}

	c.Check(err.Error(), Matches, `cannot set instance id of machine "1": already set`)
	return func() {}, nil
}

func opMachine1Constraints(c *C, st *api.State, mst *state.State) (func(), error) {
	m, err := st.Machine("1")
	if err != nil {
		c.Check(m, IsNil)
		return func() {}, err
	}
	_, err = m.Constraints()
	return func() {}, err
}

func opMachine1Remove(c *C, st *api.State, mst *state.State) (func(), error) {
	m, err := st.Machine("1")
	if err != nil {
		c.Check(m, IsNil)
		return func() {}, err
	}
	err = m.Remove()
	if err != nil && api.ErrCode(err) == api.CodeUnauthorized {
		// We expect this for any entity other than machine-0.
		return func() {}, err
	}

	c.Check(err, ErrorMatches, "cannot remove machine 1: machine is not dead")
	return func() {}, nil
}

func opMachine1EnsureDead(c *C, st *api.State, mst *state.State) (func(), error) {
	m, err := st.Machine("1")
	if err != nil {
		c.Check(m, IsNil)
		return func() {}, err
	}
	err = m.EnsureDead()
	if err != nil && api.ErrCode(err) == api.CodeUnauthorized {
		// We expect this for any entity other than machine-1.
		return func() {}, err
	}

	c.Check(err.Error(), Matches, `machine 1 has unit "wordpress/0" assigned`)
	return func() {}, nil
}

func opMachine1Status(c *C, st *api.State, mst *state.State) (func(), error) {
	m, err := st.Machine("1")
	if err != nil {
		c.Check(m, IsNil)
		return func() {}, err
	}
	_, _, err = m.Status()
	return func() {}, err
}

func opMachine1SetStatus(c *C, st *api.State, mst *state.State) (func(), error) {
	m, err := st.Machine("1")
	if err != nil {
		c.Check(m, IsNil)
		return func() {}, err
	}
	stm, err := mst.Machine("1")
	c.Check(err, IsNil)

	orgStatus, orgInfo, err := stm.Status()
	c.Check(err, IsNil)

	err = m.SetStatus(params.StatusStopped, "blah")
	if err != nil {
		return func() {}, err
	}

	return func() {
		err := m.SetStatus(orgStatus, orgInfo)
		c.Check(err, IsNil)
	}, nil
}

func opClientCharmInfo(c *C, st *api.State, mst *state.State) (func(), error) {
	info, err := st.Client().CharmInfo("local:series/wordpress-3")
	if err != nil {
		c.Check(info, IsNil)
		return func() {}, err
	}
	c.Assert(info.URL, Equals, "local:series/wordpress-3")
	c.Assert(info.Meta.Name, Equals, "wordpress")
	c.Assert(info.Revision, Equals, 3)
	return func() {}, nil
}

func opClientAddRelation(c *C, st *api.State, mst *state.State) (func(), error) {
	_, err := st.Client().AddRelation("nosuch1", "nosuch2")
	if api.ErrCode(err) == api.CodeNotFound {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyRelation(c *C, st *api.State, mst *state.State) (func(), error) {
	err := st.Client().DestroyRelation("nosuch1", "nosuch2")
	if api.ErrCode(err) == api.CodeNotFound {
		err = nil
	}
	return func() {}, err
}

func opClientStatus(c *C, st *api.State, mst *state.State) (func(), error) {
	status, err := st.Client().Status()
	if err != nil {
		c.Check(status, IsNil)
		return func() {}, err
	}
	c.Assert(status, DeepEquals, scenarioStatus)
	return func() {}, nil
}

func resetBlogTitle(c *C, st *api.State) func() {
	return func() {
		err := st.Client().ServiceSet("wordpress", map[string]string{
			"blog-title": "",
		})
		c.Assert(err, IsNil)
	}
}

func opClientServiceSet(c *C, st *api.State, mst *state.State) (func(), error) {
	err := st.Client().ServiceSet("wordpress", map[string]string{
		"blog-title": "foo",
	})
	if err != nil {
		return func() {}, err
	}
	return resetBlogTitle(c, st), nil
}

func opClientServiceSetYAML(c *C, st *api.State, mst *state.State) (func(), error) {
	err := st.Client().ServiceSetYAML("wordpress", `"blog-title": "foo"`)
	if err != nil {
		return func() {}, err
	}
	return resetBlogTitle(c, st), nil
}

func opClientServiceGet(c *C, st *api.State, mst *state.State) (func(), error) {
	_, err := st.Client().ServiceGet("wordpress")
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientServiceExpose(c *C, st *api.State, mst *state.State) (func(), error) {
	err := st.Client().ServiceExpose("wordpress")
	if err != nil {
		return func() {}, err
	}
	return func() {
		svc, err := mst.Service("wordpress")
		c.Assert(err, IsNil)
		svc.ClearExposed()
	}, nil
}

func opClientServiceUnexpose(c *C, st *api.State, mst *state.State) (func(), error) {
	err := st.Client().ServiceUnexpose("wordpress")
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientResolved(c *C, st *api.State, _ *state.State) (func(), error) {
	err := st.Client().Resolved("wordpress/0", false)
	// There are several scenarios in which this test is called, one is
	// that the user is not authorized.  In that case we want to exit now,
	// letting the error percolate out so the caller knows that the
	// permission error was correctly generated.
	if err != nil && api.ErrCode(err) == api.CodeUnauthorized {
		return func() {}, err
	}
	// Otherwise, the user was authorized, but we expect an error anyway
	// because the unit is not in an error state when we tried to resolve
	// the error.  Therefore, since it is complaining it means that the
	// call to Resolved worked, so we're happy.
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, `unit "wordpress/0" is not in an error state`)
	return func() {}, nil
}

func opClientGetAnnotations(c *C, st *api.State, mst *state.State) (func(), error) {
	ann, err := st.Client().GetAnnotations("service-wordpress")
	if err != nil {
		return func() {}, err
	}
	c.Assert(ann, DeepEquals, make(map[string]string))
	return func() {}, nil
}

func opClientSetAnnotations(c *C, st *api.State, mst *state.State) (func(), error) {
	pairs := map[string]string{"key1": "value1", "key2": "value2"}
	err := st.Client().SetAnnotations("service-wordpress", pairs)
	if err != nil {
		return func() {}, err
	}
	return func() {
		pairs := map[string]string{"key1": "", "key2": ""}
		st.Client().SetAnnotations("service-wordpress", pairs)
	}, nil
}

func opClientServiceDeploy(c *C, st *api.State, mst *state.State) (func(), error) {
	// We are cheating and using a local repo only.

	// Set the CharmStore to the test repository.
	serviceName := "mywordpress"
	charmUrl := "local:series/wordpress"
	parsedUrl := charm.MustParseURL(charmUrl)
	repo, err := charm.InferRepository(parsedUrl, coretesting.Charms.Path)
	originalServerCharmStore := apiserver.CharmStore
	apiserver.CharmStore = repo

	err = st.Client().ServiceDeploy(charmUrl, serviceName, 1, "", constraints.Value{})
	if err != nil {
		return func() {}, err
	}
	return func() {
		apiserver.CharmStore = originalServerCharmStore
		service, err := mst.Service(serviceName)
		c.Assert(err, IsNil)
		removeServiceAndUnits(c, service)
	}, nil
}

func opClientAddServiceUnits(c *C, st *api.State, mst *state.State) (func(), error) {
	_, err := st.Client().AddServiceUnits("nosuch", 1)
	if api.ErrCode(err) == api.CodeNotFound {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyServiceUnits(c *C, st *api.State, mst *state.State) (func(), error) {
	err := st.Client().DestroyServiceUnits([]string{"wordpress/99"})
	if err != nil && strings.HasPrefix(err.Error(), "no units were destroyed") {
		err = nil
	}
	return func() {}, err
}

func opClientServiceDestroy(c *C, st *api.State, mst *state.State) (func(), error) {
	err := st.Client().ServiceDestroy("non-existent")
	if api.ErrCode(err) == api.CodeNotFound {
		err = nil
	}
	return func() {}, err
}

func opClientGetServiceConstraints(c *C, st *api.State, mst *state.State) (func(), error) {
	_, err := st.Client().GetServiceConstraints("wordpress")
	return func() {}, err
}

func opClientSetServiceConstraints(c *C, st *api.State, mst *state.State) (func(), error) {
	nullConstraints := constraints.Value{}
	err := st.Client().SetServiceConstraints("wordpress", nullConstraints)
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientWatchAll(c *C, st *api.State, mst *state.State) (func(), error) {
	watcher, err := st.Client().WatchAll()
	if err == nil {
		watcher.Stop()
	}
	return func() {}, err
}

func opStateAllMachines(c *C, st *api.State, mst *state.State) (func(), error) {
	machines, err := st.AllMachines()
	if err != nil {
		c.Check(machines, IsNil)
	} else {
		c.Check(machines, HasLen, 3)
	}
	return func() {}, err
}
