// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"strings"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

type permSuite struct {
	baseSuite
}

var _ = gc.Suite(&permSuite{})

// Most (if not all) of the permission tests below aim to test
// end-to-end operations execution through the API, but do not care
// about the results. They only test that a call is succeeds or fails
// (usually due to "permission denied"). There are separate test cases
// testing each individual API call data flow later on.

// allowed returns the set of allowed entities given an allow list and a
// deny list.  If an allow list is specified, only those entities are
// allowed; otherwise those in deny are disallowed.
func allowed(all, allow, deny []names.Tag) map[names.Tag]bool {
	p := make(map[names.Tag]bool)
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

func (s *permSuite) TestOperationPerm(c *gc.C) {
	var (
		userAdmin = s.AdminUserTag(c)
		userOther = names.NewLocalUserTag("other")
	)
	entities := s.setUpScenario(c)
	for i, t := range []struct {
		about string
		// op performs the operation to be tested using the given state
		// connection. It returns a function that should be used to
		// undo any changes made by the operation.
		op    func(c *gc.C, st api.Connection, mst *state.State) (reset func(), err error)
		allow []names.Tag
		deny  []names.Tag
	}{{
		about: "Client.Status",
		op:    opClientStatus,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ServiceSet",
		op:    opClientServiceSet,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ServiceSetYAML",
		op:    opClientServiceSetYAML,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ServiceGet",
		op:    opClientServiceGet,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.Resolved",
		op:    opClientResolved,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ServiceExpose",
		op:    opClientServiceExpose,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ServiceUnexpose",
		op:    opClientServiceUnexpose,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ServiceDeploy",
		op:    opClientServiceDeploy,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ServiceDeployWithNetworks",
		op:    opClientServiceDeployWithNetworks,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ServiceUpdate",
		op:    opClientServiceUpdate,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ServiceSetCharm",
		op:    opClientServiceSetCharm,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.GetAnnotations",
		op:    opClientGetAnnotations,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.SetAnnotations",
		op:    opClientSetAnnotations,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.AddServiceUnits",
		op:    opClientAddServiceUnits,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.DestroyServiceUnits",
		op:    opClientDestroyServiceUnits,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ServiceDestroy",
		op:    opClientServiceDestroy,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.GetServiceConstraints",
		op:    opClientGetServiceConstraints,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.SetServiceConstraints",
		op:    opClientSetServiceConstraints,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.SetEnvironmentConstraints",
		op:    opClientSetEnvironmentConstraints,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.EnvironmentGet",
		op:    opClientEnvironmentGet,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.EnvironmentSet",
		op:    opClientEnvironmentSet,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.SetEnvironAgentVersion",
		op:    opClientSetEnvironAgentVersion,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.WatchAll",
		op:    opClientWatchAll,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.CharmInfo",
		op:    opClientCharmInfo,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.AddRelation",
		op:    opClientAddRelation,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.DestroyRelation",
		op:    opClientDestroyRelation,
		allow: []names.Tag{userAdmin, userOther},
	}} {
		allow := allowed(entities, t.allow, t.deny)
		for j, e := range entities {
			c.Logf("\n------\ntest %d,%d; %s; entity %q", i, j, t.about, e)
			st := s.openAs(c, e)
			reset, err := t.op(c, st, s.State)
			if allow[e] {
				c.Check(err, jc.ErrorIsNil)
			} else {
				c.Check(err, gc.ErrorMatches, "permission denied")
				c.Check(err, jc.Satisfies, params.IsCodeUnauthorized)
			}
			reset()
			st.Close()
		}
	}
}

func opClientCharmInfo(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	info, err := st.Client().CharmInfo("local:quantal/wordpress-3")
	if err != nil {
		c.Check(info, gc.IsNil)
		return func() {}, err
	}
	c.Assert(info.URL, gc.Equals, "local:quantal/wordpress-3")
	c.Assert(info.Meta.Name, gc.Equals, "wordpress")
	c.Assert(info.Revision, gc.Equals, 3)
	c.Assert(info.Actions, jc.DeepEquals, &charm.Actions{
		ActionSpecs: map[string]charm.ActionSpec{
			"fakeaction": {
				Description: "No description",
				Params: map[string]interface{}{
					"type":        "object",
					"description": "No description",
					"properties":  map[string]interface{}{},
					"title":       "fakeaction",
				},
			},
		},
	})
	return func() {}, nil
}

func opClientAddRelation(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := st.Client().AddRelation("nosuch1", "nosuch2")
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyRelation(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := st.Client().DestroyRelation("nosuch1", "nosuch2")
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientStatus(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	status, err := st.Client().Status(nil)
	if err != nil {
		c.Check(status, gc.IsNil)
		return func() {}, err
	}
	clearSinceTimes(status)
	c.Assert(status, jc.DeepEquals, scenarioStatus)
	return func() {}, nil
}

func resetBlogTitle(c *gc.C, st api.Connection) func() {
	return func() {
		err := st.Client().ServiceSet("wordpress", map[string]string{
			"blog-title": "",
		})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func opClientServiceSet(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := st.Client().ServiceSet("wordpress", map[string]string{
		"blog-title": "foo",
	})
	if err != nil {
		return func() {}, err
	}
	return resetBlogTitle(c, st), nil
}

func opClientServiceSetYAML(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := st.Client().ServiceSetYAML("wordpress", `"wordpress": {"blog-title": "foo"}`)
	if err != nil {
		return func() {}, err
	}
	return resetBlogTitle(c, st), nil
}

func opClientServiceGet(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := st.Client().ServiceGet("wordpress")
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientServiceExpose(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := st.Client().ServiceExpose("wordpress")
	if err != nil {
		return func() {}, err
	}
	return func() {
		svc, err := mst.Service("wordpress")
		c.Assert(err, jc.ErrorIsNil)
		svc.ClearExposed()
	}, nil
}

func opClientServiceUnexpose(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := st.Client().ServiceUnexpose("wordpress")
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientResolved(c *gc.C, st api.Connection, _ *state.State) (func(), error) {
	err := st.Client().Resolved("wordpress/1", false)
	// There are several scenarios in which this test is called, one is
	// that the user is not authorized.  In that case we want to exit now,
	// letting the error percolate out so the caller knows that the
	// permission error was correctly generated.
	if err != nil && params.IsCodeUnauthorized(err) {
		return func() {}, err
	}
	// Otherwise, the user was authorized, but we expect an error anyway
	// because the unit is not in an error state when we tried to resolve
	// the error.  Therefore, since it is complaining it means that the
	// call to Resolved worked, so we're happy.
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `unit "wordpress/1" is not in an error state`)
	return func() {}, nil
}

func opClientGetAnnotations(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	ann, err := st.Client().GetAnnotations("service-wordpress")
	if err != nil {
		return func() {}, err
	}
	c.Assert(ann, gc.DeepEquals, make(map[string]string))
	return func() {}, nil
}

func opClientSetAnnotations(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
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

func opClientServiceDeploy(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := st.Client().ServiceDeploy("mad:bad/url-1", "x", 1, "", constraints.Value{}, "")
	if err.Error() == `charm URL has invalid schema: "mad:bad/url-1"` {
		err = nil
	}
	return func() {}, err
}

func opClientServiceDeployWithNetworks(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := st.Client().ServiceDeployWithNetworks("mad:bad/url-1", "x", 1, "", constraints.Value{}, "", nil)
	if err.Error() == `charm URL has invalid schema: "mad:bad/url-1"` {
		err = nil
	}
	return func() {}, err
}

func opClientServiceUpdate(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	args := params.ServiceUpdate{
		ServiceName:     "no-such-charm",
		CharmUrl:        "cs:quantal/wordpress-42",
		ForceCharmUrl:   true,
		SettingsStrings: map[string]string{"blog-title": "foo"},
		SettingsYAML:    `"wordpress": {"blog-title": "foo"}`,
	}
	err := st.Client().ServiceUpdate(args)
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientServiceSetCharm(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := st.Client().ServiceSetCharm("nosuch", "local:quantal/wordpress", false)
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientAddServiceUnits(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := st.Client().AddServiceUnits("nosuch", 1, "")
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyServiceUnits(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := st.Client().DestroyServiceUnits("wordpress/99")
	if err != nil && strings.HasPrefix(err.Error(), "no units were destroyed") {
		err = nil
	}
	return func() {}, err
}

func opClientServiceDestroy(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := st.Client().ServiceDestroy("non-existent")
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientGetServiceConstraints(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := st.Client().GetServiceConstraints("wordpress")
	return func() {}, err
}

func opClientSetServiceConstraints(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	nullConstraints := constraints.Value{}
	err := st.Client().SetServiceConstraints("wordpress", nullConstraints)
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientSetEnvironmentConstraints(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	nullConstraints := constraints.Value{}
	err := st.Client().SetEnvironmentConstraints(nullConstraints)
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientEnvironmentGet(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := st.Client().EnvironmentGet()
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientEnvironmentSet(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	args := map[string]interface{}{"some-key": "some-value"}
	err := st.Client().EnvironmentSet(args)
	if err != nil {
		return func() {}, err
	}
	return func() {
		args["some-key"] = nil
		st.Client().EnvironmentSet(args)
	}, nil
}

func opClientSetEnvironAgentVersion(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	attrs, err := st.Client().EnvironmentGet()
	if err != nil {
		return func() {}, err
	}
	err = st.Client().SetEnvironAgentVersion(version.Current.Number)
	if err != nil {
		return func() {}, err
	}

	return func() {
		oldAgentVersion, found := attrs["agent-version"]
		if found {
			versionString := oldAgentVersion.(string)
			st.Client().SetEnvironAgentVersion(version.MustParse(versionString))
		}
	}, nil
}

func opClientWatchAll(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	watcher, err := st.Client().WatchAll()
	if err == nil {
		watcher.Stop()
	}
	return func() {}, err
}
