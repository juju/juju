// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"strings"
)

type permSuite struct {
	baseSuite
}

var _ = Suite(&permSuite{})

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
	about: "Client.ServiceUpdate",
	op:    opClientServiceUpdate,
	allow: []string{"user-admin", "user-other"},
}, {
	about: "Client.ServiceSetCharm",
	op:    opClientServiceSetCharm,
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
}}

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

func (s *permSuite) TestOperationPerm(c *C) {
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
				c.Check(params.ErrCode(err), Equals, params.CodeUnauthorized)
			}
			reset()
			st.Close()
		}
	}
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
	if params.ErrCode(err) == params.CodeNotFound {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyRelation(c *C, st *api.State, mst *state.State) (func(), error) {
	err := st.Client().DestroyRelation("nosuch1", "nosuch2")
	if params.ErrCode(err) == params.CodeNotFound {
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
	err := st.Client().ServiceSetYAML("wordpress", `"wordpress": {"blog-title": "foo"}`)
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
	if err != nil && params.ErrCode(err) == params.CodeUnauthorized {
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
	err := st.Client().ServiceDeploy("mad:bad/url-1", "x", 1, "", constraints.Value{})
	if err.Error() == `charm URL has invalid schema: "mad:bad/url-1"` {
		err = nil
	}
	return func() {}, err
}

func opClientServiceUpdate(c *C, st *api.State, mst *state.State) (func(), error) {
	serviceName := "no-such-charm"
	charmUrl := "cs:series/wordpress-42"
	forceCharmUrl := true
	minUnits := 2
	settingsStrings := map[string]string{"blog-title": "foo"}
	settingsYAML := `"wordpress": {"blog-title": "foo"}`
	constraints := constraints.Value{}
	err := st.Client().ServiceUpdate(serviceName, charmUrl, forceCharmUrl,
		&minUnits, settingsStrings, settingsYAML, &constraints)
	if params.ErrCode(err) == params.CodeNotFound {
		err = nil
	}
	return func() {}, err
}

func opClientServiceSetCharm(c *C, st *api.State, mst *state.State) (func(), error) {
	err := st.Client().ServiceSetCharm("nosuch", "local:series/wordpress", false)
	if params.ErrCode(err) == params.CodeNotFound {
		err = nil
	}
	return func() {}, err
}

func opClientAddServiceUnits(c *C, st *api.State, mst *state.State) (func(), error) {
	_, err := st.Client().AddServiceUnits("nosuch", 1)
	if params.ErrCode(err) == params.CodeNotFound {
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
	if params.ErrCode(err) == params.CodeNotFound {
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
