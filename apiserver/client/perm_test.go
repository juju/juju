// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/annotations"
	"github.com/juju/juju/api/service"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/rpc"
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
		about: "Service.Set",
		op:    opClientServiceSet,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Service.Get",
		op:    opClientServiceGet,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.Resolved",
		op:    opClientResolved,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Service.Expose",
		op:    opClientServiceExpose,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Service.Unexpose",
		op:    opClientServiceUnexpose,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Service.Update",
		op:    opClientServiceUpdate,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Service.SetCharm",
		op:    opClientServiceSetCharm,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Annotations.GetAnnotations",
		op:    opClientGetAnnotations,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Annotations.SetAnnotations",
		op:    opClientSetAnnotations,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Service.AddUnits",
		op:    opClientAddServiceUnits,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Service.DestroyUnits",
		op:    opClientDestroyServiceUnits,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Service.Destroy",
		op:    opClientServiceDestroy,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Service.GetConstraints",
		op:    opClientGetServiceConstraints,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Service.SetConstraints",
		op:    opClientSetServiceConstraints,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.SetModelConstraints",
		op:    opClientSetEnvironmentConstraints,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ModelGet",
		op:    opClientEnvironmentGet,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ModelSet",
		op:    opClientEnvironmentSet,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.SetModelAgentVersion",
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
		about: "Service.AddRelation",
		op:    opClientAddRelation,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Service.DestroyRelation",
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
				c.Check(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
					Message: "permission denied",
					Code:    "unauthorized access",
				})
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
	_, err := service.NewClient(st).AddRelation("nosuch1", "nosuch2")
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyRelation(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := service.NewClient(st).DestroyRelation("nosuch1", "nosuch2")
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
		err := service.NewClient(st).Set("wordpress", map[string]string{
			"blog-title": "",
		})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func opClientServiceSet(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := service.NewClient(st).Set("wordpress", map[string]string{
		"blog-title": "foo",
	})
	if err != nil {
		return func() {}, err
	}
	return resetBlogTitle(c, st), nil
}

func opClientServiceGet(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := service.NewClient(st).Get("wordpress")
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientServiceExpose(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := service.NewClient(st).Expose("wordpress")
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
	err := service.NewClient(st).Unexpose("wordpress")
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
	ann, err := annotations.NewClient(st).Get([]string{"service-wordpress"})
	if err != nil {
		return func() {}, err
	}
	c.Assert(ann, gc.DeepEquals, []params.AnnotationsGetResult{{
		EntityTag:   "service-wordpress",
		Annotations: map[string]string{},
	}})
	return func() {}, nil
}

func opClientSetAnnotations(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	pairs := map[string]string{"key1": "value1", "key2": "value2"}
	setParams := map[string]map[string]string{
		"service-wordpress": pairs,
	}
	_, err := annotations.NewClient(st).Set(setParams)
	if err != nil {
		return func() {}, err
	}
	return func() {
		pairs := map[string]string{"key1": "", "key2": ""}
		setParams := map[string]map[string]string{
			"service-wordpress": pairs,
		}
		annotations.NewClient(st).Set(setParams)
	}, nil
}

func opClientServiceUpdate(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	args := params.ServiceUpdate{
		ServiceName:     "no-such-charm",
		CharmUrl:        "cs:quantal/wordpress-42",
		ForceCharmUrl:   true,
		SettingsStrings: map[string]string{"blog-title": "foo"},
		SettingsYAML:    `"wordpress": {"blog-title": "foo"}`,
	}
	err := service.NewClient(st).Update(args)
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientServiceSetCharm(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	cfg := service.SetCharmConfig{
		ServiceName: "nosuch",
		CharmUrl:    "local:quantal/wordpress",
	}
	err := service.NewClient(st).SetCharm(cfg)
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientAddServiceUnits(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := service.NewClient(st).AddUnits("nosuch", 1, nil)
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyServiceUnits(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := service.NewClient(st).DestroyUnits("wordpress/99")
	if err != nil && strings.HasPrefix(err.Error(), "no units were destroyed") {
		err = nil
	}
	return func() {}, err
}

func opClientServiceDestroy(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := service.NewClient(st).Destroy("non-existent")
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientGetServiceConstraints(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := service.NewClient(st).GetConstraints("wordpress")
	return func() {}, err
}

func opClientSetServiceConstraints(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	nullConstraints := constraints.Value{}
	err := service.NewClient(st).SetConstraints("wordpress", nullConstraints)
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientSetEnvironmentConstraints(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	nullConstraints := constraints.Value{}
	err := st.Client().SetModelConstraints(nullConstraints)
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientEnvironmentGet(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := st.Client().ModelGet()
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientEnvironmentSet(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	args := map[string]interface{}{"some-key": "some-value"}
	err := st.Client().ModelSet(args)
	if err != nil {
		return func() {}, err
	}
	return func() {
		args["some-key"] = nil
		st.Client().ModelSet(args)
	}, nil
}

func opClientSetEnvironAgentVersion(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	attrs, err := st.Client().ModelGet()
	if err != nil {
		return func() {}, err
	}
	err = st.Client().SetModelAgentVersion(version.Current)
	if err != nil {
		return func() {}, err
	}

	return func() {
		oldAgentVersion, found := attrs["agent-version"]
		if found {
			versionString := oldAgentVersion.(string)
			st.Client().SetModelAgentVersion(version.MustParse(versionString))
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
