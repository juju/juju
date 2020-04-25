// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"strings"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/annotations"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/apiserver/facades/client/client"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
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

func (s *permSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	client.SkipReplicaCheck(s)
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
		about: "Application.Set",
		op:    opClientServiceSet,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.Get",
		op:    opClientServiceGet,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.Resolved",
		op:    opClientResolved,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.Expose",
		op:    opClientServiceExpose,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.Unexpose",
		op:    opClientServiceUnexpose,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.Update",
		op:    opClientServiceUpdate,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.SetCharm",
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
		about: "Application.AddUnits",
		op:    opClientAddServiceUnits,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.DestroyUnits",
		op:    opClientDestroyServiceUnits,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.DestroyUnit",
		op:    opClientDestroyUnit,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.Destroy",
		op:    opClientServiceDestroy,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.DestroyApplication",
		op:    opClientDestroyApplication,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.GetConstraints",
		op:    opClientGetServiceConstraints,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.SetConstraints",
		op:    opClientSetServiceConstraints,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.SetModelConstraints",
		op:    opClientSetEnvironmentConstraints,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ModelGet",
		op:    opClientModelGet,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.ModelSet",
		op:    opClientModelSet,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.SetModelAgentVersion",
		op:    opClientSetModelAgentVersion,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Client.WatchAll",
		op:    opClientWatchAll,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.AddRelation",
		op:    opClientAddRelation,
		allow: []names.Tag{userAdmin, userOther},
	}, {
		about: "Application.DestroyRelation",
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

func opClientAddRelation(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := application.NewClient(st).AddRelation([]string{"nosuch1", "nosuch2"}, nil)
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyRelation(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := application.NewClient(st).DestroyRelation((*bool)(nil), (*time.Duration)(nil), "nosuch1", "nosuch2")
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
	clearSinceTimes(scenarioStatus)
	clearContollerTimestamp(status)
	c.Assert(status, jc.DeepEquals, scenarioStatus)
	return func() {}, nil
}

func resetBlogTitle(c *gc.C, st api.Connection) func() {
	return func() {
		err := application.NewClient(st).Set("wordpress", map[string]string{
			"blog-title": "",
		})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func opClientServiceSet(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := application.NewClient(st).Set("wordpress", map[string]string{
		"blog-title": "foo",
	})
	if err != nil {
		return func() {}, err
	}
	return resetBlogTitle(c, st), nil
}

func opClientServiceGet(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := application.NewClient(st).Get(model.GenerationMaster, "wordpress")
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientServiceExpose(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := application.NewClient(st).Expose("wordpress")
	if err != nil {
		return func() {}, err
	}
	return func() {
		svc, err := mst.Application("wordpress")
		c.Assert(err, jc.ErrorIsNil)
		err = svc.ClearExposed()
		c.Assert(err, jc.ErrorIsNil)
	}, nil
}

func opClientServiceUnexpose(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := application.NewClient(st).Unexpose("wordpress")
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
	ann, err := annotations.NewClient(st).Get([]string{"application-wordpress"})
	if err != nil {
		return func() {}, err
	}
	c.Assert(ann, gc.DeepEquals, []params.AnnotationsGetResult{{
		EntityTag:   "application-wordpress",
		Annotations: map[string]string{},
	}})
	return func() {}, nil
}

func opClientSetAnnotations(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	pairs := map[string]string{"key1": "value1", "key2": "value2"}
	setParams := map[string]map[string]string{
		"application-wordpress": pairs,
	}
	_, err := annotations.NewClient(st).Set(setParams)
	if err != nil {
		return func() {}, err
	}
	return func() {
		pairs := map[string]string{"key1": "", "key2": ""}
		setParams := map[string]map[string]string{
			"application-wordpress": pairs,
		}
		_, err := annotations.NewClient(st).Set(setParams)
		c.Assert(err, jc.ErrorIsNil)
	}, nil
}

func opClientServiceUpdate(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	args := params.ApplicationUpdate{
		ApplicationName: "no-such-charm",
		CharmURL:        "cs:quantal/wordpress-42",
		ForceCharmURL:   true,
		SettingsStrings: map[string]string{"blog-title": "foo"},
		SettingsYAML:    `"wordpress": {"blog-title": "foo"}`,
	}
	err := application.NewClient(st).Update(args)
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientServiceSetCharm(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	cfg := application.SetCharmConfig{
		ApplicationName: "nosuch",
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("local:quantal/wordpress"),
		},
	}
	err := application.NewClient(st).SetCharm(model.GenerationMaster, cfg)
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientAddServiceUnits(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := application.NewClient(st).AddUnits(application.AddUnitsParams{
		ApplicationName: "nosuch",
		NumUnits:        1,
	})
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyServiceUnits(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := application.NewClient(st).DestroyUnitsDeprecated("wordpress/99")
	if err != nil && strings.HasPrefix(err.Error(), "no units were destroyed") {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyUnit(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := application.NewClient(st).DestroyUnits(application.DestroyUnitsParams{
		Units: []string{"wordpress/99"},
	})
	return func() {}, err
}

func opClientServiceDestroy(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	err := application.NewClient(st).DestroyDeprecated("non-existent")
	if params.IsCodeNotFound(err) {
		err = nil
	}
	return func() {}, err
}

func opClientDestroyApplication(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := application.NewClient(st).DestroyApplications(application.DestroyApplicationsParams{
		Applications: []string{"non-existent"},
	})
	return func() {}, err
}

func opClientGetServiceConstraints(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := application.NewClient(st).GetConstraints("wordpress")
	return func() {}, err
}

func opClientSetServiceConstraints(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	nullConstraints := constraints.Value{}
	err := application.NewClient(st).SetConstraints("wordpress", nullConstraints)
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

func opClientModelGet(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	_, err := modelconfig.NewClient(st).ModelGet()
	if err != nil {
		return func() {}, err
	}
	return func() {}, nil
}

func opClientModelSet(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	args := map[string]interface{}{"some-key": "some-value"}
	err := modelconfig.NewClient(st).ModelSet(args)
	if err != nil {
		return func() {}, err
	}
	return func() {
		args["some-key"] = nil
		modelconfig.NewClient(st).ModelSet(args)
	}, nil
}

func opClientSetModelAgentVersion(c *gc.C, st api.Connection, mst *state.State) (func(), error) {
	attrs, err := modelconfig.NewClient(st).ModelGet()
	if err != nil {
		return func() {}, err
	}
	ver := version.Number{Major: 1, Minor: 2, Patch: 3}
	err = st.Client().SetModelAgentVersion(ver, false)
	if err != nil {
		return func() {}, err
	}

	return func() {
		oldAgentVersion, found := attrs["agent-version"]
		if found {
			versionString := oldAgentVersion.(string)
			st.Client().SetModelAgentVersion(version.MustParse(versionString), false)
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
