// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/common"
)

type ConformSuite struct{}

var _ = gc.Suite(&ConformSuite{})

func (s *ConformSuite) TestConformYAML(c *gc.C) {
	var goodInterfaceTests = []struct {
		description       string
		inputInterface    interface{}
		expectedInterface map[string]interface{}
		expectedError     string
	}{{
		description: "An interface requiring no changes.",
		inputInterface: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": map[string]interface{}{
				"foo1": "val1",
				"foo2": "val2"}},
		expectedInterface: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": map[string]interface{}{
				"foo1": "val1",
				"foo2": "val2"}},
	}, {
		description: "Substitute a single inner map[i]i.",
		inputInterface: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": map[interface{}]interface{}{
				"foo1": "val1",
				"foo2": "val2"}},
		expectedInterface: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": map[string]interface{}{
				"foo1": "val1",
				"foo2": "val2"}},
	}, {
		description: "Substitute nested inner map[i]i.",
		inputInterface: map[string]interface{}{
			"key1a": "val1a",
			"key2a": "val2a",
			"key3a": map[interface{}]interface{}{
				"key1b": "val1b",
				"key2b": map[interface{}]interface{}{
					"key1c": "val1c"}}},
		expectedInterface: map[string]interface{}{
			"key1a": "val1a",
			"key2a": "val2a",
			"key3a": map[string]interface{}{
				"key1b": "val1b",
				"key2b": map[string]interface{}{
					"key1c": "val1c"}}},
	}, {
		description: "Substitute nested map[i]i within []i.",
		inputInterface: map[string]interface{}{
			"key1a": "val1a",
			"key2a": []interface{}{5, "foo", map[string]interface{}{
				"key1b": "val1b",
				"key2b": map[interface{}]interface{}{
					"key1c": "val1c"}}}},
		expectedInterface: map[string]interface{}{
			"key1a": "val1a",
			"key2a": []interface{}{5, "foo", map[string]interface{}{
				"key1b": "val1b",
				"key2b": map[string]interface{}{
					"key1c": "val1c"}}}},
	}, {
		description: "An inner map[interface{}]interface{} with an int key.",
		inputInterface: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": map[interface{}]interface{}{
				"foo1": "val1",
				5:      "val2"}},
		expectedError: "map keyed with non-string value",
	}, {
		description: "An inner []interface{} containing a map[i]i with an int key.",
		inputInterface: map[string]interface{}{
			"key1a": "val1b",
			"key2a": "val2b",
			"key3a": []interface{}{"foo1", 5, map[interface{}]interface{}{
				"key1b": "val1b",
				"key2b": map[interface{}]interface{}{
					"key1c": "val1c",
					5:       "val2c"}}}},
		expectedError: "map keyed with non-string value",
	}}

	for i, test := range goodInterfaceTests {
		c.Logf("test %d: %s", i, test.description)
		input := test.inputInterface
		cleansedInterfaceMap, err := common.ConformYAML(input)
		if test.expectedError == "" {
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
			c.Check(cleansedInterfaceMap, gc.DeepEquals, test.expectedInterface)
		} else {
			c.Check(err, gc.ErrorMatches, test.expectedError)
		}
	}
}

type userFriendlyDurationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&userFriendlyDurationSuite{})

func (*userFriendlyDurationSuite) TestFormat(c *gc.C) {
	// lp:1558657
	now := time.Now()
	for _, test := range []struct {
		other    time.Time
		expected string
	}{
		{
			other:    now,
			expected: "just now",
		}, {
			other:    now.Add(-1 * time.Second),
			expected: "just now",
		}, {
			other:    now.Add(-2 * time.Second),
			expected: "2 seconds ago",
		}, {
			other:    now.Add(-59 * time.Second),
			expected: "59 seconds ago",
		}, {
			other:    now.Add(-60 * time.Second),
			expected: "1 minute ago",
		}, {
			other:    now.Add(-61 * time.Second),
			expected: "1 minute ago",
		}, {
			other:    now.Add(-2 * time.Minute),
			expected: "2 minutes ago",
		}, {
			other:    now.Add(-59 * time.Minute),
			expected: "59 minutes ago",
		}, {
			other:    now.Add(-60 * time.Minute),
			expected: "1 hour ago",
		}, {
			other:    now.Add(-61 * time.Minute),
			expected: "1 hour ago",
		}, {
			other:    now.Add(-2 * time.Hour),
			expected: "2 hours ago",
		}, {
			other:    now.Add(-23 * time.Hour),
			expected: "23 hours ago",
		}, {
			other:    now.Add(-24 * time.Hour),
			expected: now.Add(-24 * time.Hour).Format("2006-01-02"),
		}, {
			other:    now.Add(-96 * time.Hour),
			expected: now.Add(-96 * time.Hour).Format("2006-01-02"),
		},
	} {
		obtained := common.UserFriendlyDuration(test.other, now)
		c.Check(obtained, gc.Equals, test.expected)
	}
}
