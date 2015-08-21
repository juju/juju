// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"github.com/juju/bundlechanges"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type bundlesSuite struct {
	baseSuite
}

var _ = gc.Suite(&bundlesSuite{})

func (s *bundlesSuite) TestGetBundleChangesBundleContentError(c *gc.C) {
	changes, errors, err := s.APIState.Client().GetBundleChanges(":")
	c.Assert(err, gc.ErrorMatches, `cannot read bundle YAML: cannot unmarshal bundle data: YAML error: did not find expected key`)
	c.Assert(changes, gc.IsNil)
	c.Assert(errors, gc.IsNil)
}

func (s *bundlesSuite) TestGetBundleChangesBundleVerificationErrors(c *gc.C) {
	yaml := `
        services:
            django:
                charm: django
                to: [1]
            haproxy:
                charm: 42
                num_units: -1
    `
	changes, errors, err := s.APIState.Client().GetBundleChanges(yaml)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, gc.IsNil)
	c.Assert(errors, jc.SameContents, []string{
		`placement "1" refers to a machine not defined in this bundle`,
		`too many units specified in unit placement for service "django"`,
		`invalid charm URL in service "haproxy": charm URL has invalid charm name: "42"`,
		`negative number of units specified on service "haproxy"`,
	})
}

func (s *bundlesSuite) TestGetBundleChangesSuccess(c *gc.C) {
	yaml := `
        services:
            django:
                charm: django
                options:
                    debug: true
            haproxy:
                charm: cs:trusty/haproxy-42
        relations:
            - - django:web
              - haproxy:web
    `
	changes, errors, err := s.APIState.Client().GetBundleChanges(yaml)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(changes, jc.DeepEquals, []*bundlechanges.Change{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Args:   []interface{}{"django"},
	}, {
		Id:       "addService-1",
		Method:   "deploy",
		Args:     []interface{}{"django", "django", map[string]interface{}{"debug": true}},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Args:   []interface{}{"cs:trusty/haproxy-42"},
	}, {
		Id:       "addService-3",
		Method:   "deploy",
		Args:     []interface{}{"cs:trusty/haproxy-42", "haproxy", map[string]interface{}{}},
		Requires: []string{"addCharm-2"},
	}, {
		Id:       "addRelation-4",
		Method:   "addRelation",
		Args:     []interface{}{"$addService-1:web", "$addService-3:web"},
		Requires: []string{"addService-1", "addService-3"},
	}})
	c.Assert(errors, gc.IsNil)
}
