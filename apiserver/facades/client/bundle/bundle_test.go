// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/description"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type bundleSuite struct {
	testing.JujuConnSuite
	api  *bundle.BundleAPI
	auth facade.Authorizer
}

var _ = gc.Suite(&bundleSuite{})

// bundleSuiteContext implements the facade.Context interface.
type bundleSuiteContext struct{ bs *bundleSuite }

func (ctx *bundleSuiteContext) Abort() <-chan struct{}      { return nil }
func (ctx *bundleSuiteContext) Auth() facade.Authorizer     { return ctx.bs.auth }
func (ctx *bundleSuiteContext) Dispose()                    {}
func (ctx *bundleSuiteContext) Resources() facade.Resources { return common.NewResources() }
func (ctx *bundleSuiteContext) State() *state.State         { return ctx.bs.State }
func (ctx *bundleSuiteContext) StatePool() *state.StatePool { return nil }
func (ctx *bundleSuiteContext) ID() string                  { return "" }
func (ctx *bundleSuiteContext) Presence() facade.Presence   { return nil }
func (ctx *bundleSuiteContext) Hub() facade.Hub             { return nil }

func (s *bundleSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.auth = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("who"),
	}
	facade, err := bundle.NewFacade(&bundleSuiteContext{bs: s})
	c.Assert(err, jc.ErrorIsNil)
	s.api = facade
}

func (s *bundleSuite) TestGetChangesBundleContentError(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: ":",
	}
	r, err := s.api.GetChanges(args)
	c.Assert(err, gc.ErrorMatches, `cannot read bundle YAML: cannot unmarshal bundle data: yaml: did not find expected key`)
	c.Assert(r, gc.DeepEquals, params.BundleChangesResults{})
}

func (s *bundleSuite) TestGetChangesBundleVerificationErrors(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    to: [1]
                haproxy:
                    charm: 42
                    num_units: -1
        `,
	}
	r, err := s.api.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`placement "1" refers to a machine not defined in this bundle`,
		`too many units specified in unit placement for application "django"`,
		`invalid charm URL in application "haproxy": cannot parse URL "42": name "42" not valid`,
		`negative number of units specified on application "haproxy"`,
	})
}

func (s *bundleSuite) TestGetChangesBundleConstraintsError(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    num_units: 1
                    constraints: bad=wolf
        `,
	}
	r, err := s.api.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`invalid constraints "bad=wolf" in application "django": unknown constraint "bad"`,
	})
}

func (s *bundleSuite) TestGetChangesBundleStorageError(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    num_units: 1
                    storage:
                        bad: 0,100M
        `,
	}
	r, err := s.api.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`invalid storage "bad" in application "django": cannot parse count: count must be greater than zero, got "0"`,
	})
}

func (s *bundleSuite) TestGetChangesBundleDevicesError(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    num_units: 1
                    devices:
                        bad-gpu: -1,nvidia.com/gpu
        `,
	}
	r, err := s.api.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, gc.IsNil)
	c.Assert(r.Errors, jc.SameContents, []string{
		`invalid device "bad-gpu" in application "django": count must be greater than zero, got "-1"`,
	})
}

func (s *bundleSuite) TestGetChangesSuccess(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    options:
                        debug: true
                    storage:
                        tmpfs: tmpfs,1G
                    devices:
                        bitcoinminer: 2,nvidia.com/gpu
                haproxy:
                    charm: cs:trusty/haproxy-42
            relations:
                - - django:web
                  - haproxy:web
        `,
	}
	r, err := s.api.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, jc.DeepEquals, []*params.BundleChange{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Args:   []interface{}{"django", ""},
	}, {
		Id:     "deploy-1",
		Method: "deploy",
		Args: []interface{}{
			"$addCharm-0",
			"",
			"django",
			map[string]interface{}{"debug": true},
			"",
			map[string]string{"tmpfs": "tmpfs,1G"},
			map[string]string{"bitcoinminer": "2,nvidia.com/gpu"},
			map[string]string{},
			map[string]int{},
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Args:   []interface{}{"cs:trusty/haproxy-42", "trusty"},
	}, {
		Id:     "deploy-3",
		Method: "deploy",
		Args: []interface{}{
			"$addCharm-2",
			"trusty",
			"haproxy",
			map[string]interface{}{},
			"",
			map[string]string{},
			map[string]string{},
			map[string]string{},
			map[string]int{},
		},
		Requires: []string{"addCharm-2"},
	}, {
		Id:       "addRelation-4",
		Method:   "addRelation",
		Args:     []interface{}{"$deploy-1:web", "$deploy-3:web"},
		Requires: []string{"deploy-1", "deploy-3"},
	}})
	c.Assert(r.Errors, gc.IsNil)
}

func (s *bundleSuite) TestGetChangesBundleEndpointBindingsSuccess(c *gc.C) {
	args := params.BundleChangesParams{
		BundleDataYAML: `
            applications:
                django:
                    charm: django
                    num_units: 1
                    bindings:
                        url: public
        `,
	}
	r, err := s.api.GetChanges(args)
	c.Assert(err, jc.ErrorIsNil)

	for _, change := range r.Changes {
		if change.Method == "deploy" {
			c.Assert(change, jc.DeepEquals, &params.BundleChange{
				Id:     "deploy-1",
				Method: "deploy",
				Args: []interface{}{
					"$addCharm-0",
					"",
					"django",
					map[string]interface{}{},
					"",
					map[string]string{},
					map[string]string{},
					map[string]string{"url": "public"},
					map[string]int{},
				},
				Requires: []string{"addCharm-0"},
			})
		}
	}
}

func (s *bundleSuite) TestExportBundleSuccess(c *gc.C) {
	bytes, err := s.api.ExportBundle()
	c.Assert(err, jc.ErrorIsNil)

	// The bytes must be a valid model.
	modelDesc, err := description.Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelDesc.Validate(), jc.ErrorIsNil)
}
