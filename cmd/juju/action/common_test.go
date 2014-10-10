// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This is necessary since it must test a recursive unexported function,
// i.e., the function cannot be exported via a var
package action

import gc "gopkg.in/check.v1"

type CommonSuite struct{}

var _ = gc.Suite(&CommonSuite{})

func (s *CommonSuite) TestConform(c *gc.C) {
	var goodInterfaceTests = []struct {
		description       string
		inputInterface    map[string]interface{}
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
		cleansedInterfaceMap, err := conform(test.inputInterface)
		if test.expectedError == "" {
			c.Assert(err, gc.IsNil)
			c.Check(cleansedInterfaceMap, gc.DeepEquals, test.expectedInterface)
		} else {
			c.Check(err, gc.ErrorMatches, test.expectedError)
		}
	}
}

func (s *CommonSuite) TestTabbedString(c *gc.C) {
	tests := []struct {
		should    string
		given     [][]string
		sep       string
		expected  string
		errString string
	}{{
		should: "be empty with no args",
		given:  [][]string{},
	}, {
		should:   "properly tab things out",
		given:    [][]string{{"cat", "meow"}, {"dog", "woof"}},
		sep:      " -- ",
		expected: "cat\t -- meow\ndog\t -- woof",
	}, {
		should: "work for bigger strings",
		given: [][]string{
			{"something awfully long", "words"},
			{"short", "more words"},
		},
		sep: " -- ",
		expected: "something awfully long\t -- words\n" +
			"short\t\t\t -- more words",
	}, {
		should:   "work with different first and sep",
		given:    [][]string{{"a", "b"}},
		sep:      ",",
		expected: "a\t,b",
	}, {
		should:    "error on too many",
		given:     [][]string{{"a", "b", "c"}},
		sep:       " -- ",
		errString: `row must have only two items, got \[\]string{"a", "b", "c"}`,
	}, {
		should:    "error on too few",
		given:     [][]string{{"a"}},
		sep:       " -- ",
		errString: `row must have only two items, got \[\]string{"a"}`,
	}}

	for i, t := range tests {
		c.Logf("test %d: should %s", i, t.should)
		obtained, err := tabbedString(t.given, t.sep)
		if t.errString != "" {
			c.Check(err, gc.ErrorMatches, t.errString)
		} else {
			c.Assert(err, gc.IsNil)
			c.Check(obtained, gc.Equals, t.expected)
		}
	}
}
