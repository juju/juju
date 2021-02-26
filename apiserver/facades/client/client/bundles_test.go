// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

// This test is here only to make sure that the endpoint is still provided by
// the Client facade. For full coverage, see tests in the Bundle facade.
func (s *serverSuite) TestGetBundleChangesSuccess(c *gc.C) {
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
                        bitcoinminer: 2,nvidia.com/gpu  // device is not supported here because GetBundleChanges is deprecated
                haproxy:
                    charm: cs:trusty/haproxy-42
            relations:
                - - django:web
                  - haproxy:web
        `,
	}
	r, err := s.client.GetBundleChanges(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Changes, jc.DeepEquals, []*params.BundleChange{{
		Id:     "addCharm-0",
		Method: "addCharm",
		Args:   []interface{}{"django", "", ""},
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
			map[string]string{},
			map[string]int{},
			0,
			"",
		},
		Requires: []string{"addCharm-0"},
	}, {
		Id:     "addCharm-2",
		Method: "addCharm",
		Args:   []interface{}{"cs:trusty/haproxy-42", "trusty", ""},
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
			map[string]int{},
			0,
			"",
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
