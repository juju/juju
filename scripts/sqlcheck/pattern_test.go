// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type patternSuite struct {
	coretesting.BaseSuite
}

func TestPatternSuite(t *testing.T) {
	tc.Run(t, &patternSuite{})
}

func (s *patternSuite) TestMatchPath(c *tc.C) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{{
		pattern: "domain/*/state/**",
		path:    "domain/cloud/state/state.go",
		want:    true,
	}, {
		pattern: "domain/*/state/**",
		path:    "domain/cloud/state/testing/cloud.go",
		want:    true,
	}, {
		pattern: "domain/*/state/**",
		path:    "domain/cloud/service/service.go",
		want:    false,
	}, {
		pattern: "**/*_test.go",
		path:    "domain/cloud/state/state_test.go",
		want:    true,
	}, {
		pattern: "**/*_test.go",
		path:    "domain/cloud/state/state.go",
		want:    false,
	}, {
		pattern: "domain/secret/**",
		path:    "domain/secret/state/state.go",
		want:    true,
	}, {
		pattern: "domain/secret/**",
		path:    "domain/secret/state.go",
		want:    true,
	}, {
		pattern: "**",
		path:    "anything/at/all.go",
		want:    true,
	}, {
		pattern: "domain/cloud/state/state.go",
		path:    "domain/cloud/state/state.go",
		want:    true,
	}, {
		pattern: "domain/*/state/state.go",
		path:    "domain/cloud/state/state.go",
		want:    true,
	}, {
		pattern: "domain/*/state/state.go",
		path:    "domain/a/b/state/state.go",
		want:    false,
	}, {
		pattern: "domain/**/state/*.go",
		path:    "domain/a/b/state/state.go",
		want:    true,
	}}
	for _, test := range tests {
		c.Logf("pattern %q path %q", test.pattern, test.path)
		c.Check(matchPath(test.pattern, test.path), tc.Equals, test.want)
	}
}

func (s *patternSuite) TestParseRules(c *tc.C) {
	c.Check(parseRules(""), tc.IsNil)
	c.Check(parseRules("   "), tc.IsNil)
	c.Check(parseRules(","), tc.IsNil)

	rules := parseRules("LT01, cp02 ,AL03")
	c.Check(rules, tc.DeepEquals, map[string]bool{
		"LT01": true,
		"CP02": true,
		"AL03": true,
	})
}

func (s *patternSuite) TestJoinRulesDeterministic(c *tc.C) {
	c.Check(joinRules(map[string]bool{"CP01": true, "LT01": true}), tc.Equals, "CP01,LT01")
}
