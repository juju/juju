// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/query"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type applicationScopeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&applicationScopeSuite{})

func (s *applicationScopeSuite) TestGetIdentValue(c *gc.C) {
	tests := []struct {
		Field           string
		ApplicationInfo *params.ApplicationInfo
		Expected        query.Ord
	}{{
		Field:           "name",
		ApplicationInfo: &params.ApplicationInfo{Name: "application name"},
		Expected:        query.NewString("application name"),
	}, {
		Field:           "life",
		ApplicationInfo: &params.ApplicationInfo{Life: life.Alive},
		Expected:        query.NewString("alive"),
	}, {
		Field:           "charm-url",
		ApplicationInfo: &params.ApplicationInfo{CharmURL: "cs:charm"},
		Expected:        query.NewString("cs:charm"),
	}, {
		Field:           "subordinate",
		ApplicationInfo: &params.ApplicationInfo{Subordinate: true},
		Expected:        query.NewBool(true),
	}, {
		Field: "status",
		ApplicationInfo: &params.ApplicationInfo{Status: params.StatusInfo{
			Current: status.Active,
		}},
		Expected: query.NewString("active"),
	}, {
		Field:           "workload-version",
		ApplicationInfo: &params.ApplicationInfo{WorkloadVersion: "1.2.3"},
		Expected:        query.NewString("1.2.3"),
	}}
	for i, test := range tests {
		c.Logf("%d: GetIdentValue %q", i, test.Field)
		scope := ApplicationScope{
			ApplicationInfo: test.ApplicationInfo,
		}
		result, err := scope.GetIdentValue(test.Field)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, gc.DeepEquals, test.Expected)
	}
}

func (s *applicationScopeSuite) TestGetIdentValueError(c *gc.C) {
	scope := ApplicationScope{
		ApplicationInfo: &params.ApplicationInfo{},
	}
	result, err := scope.GetIdentValue("bad")
	c.Assert(err, gc.ErrorMatches, `Runtime Error: identifier "bad" not found on ApplicationInfo: invalid identifer`)
	c.Assert(result, gc.IsNil)
}
