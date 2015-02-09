// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This is necessary since it must test a recursive unexported function,
// i.e., the function cannot be exported via a var
package action

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type CommonSuite struct{}

var _ = gc.Suite(&CommonSuite{})

func (s *CommonSuite) TestConform(c *gc.C) {
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
		cleansedInterfaceMap, err := conform(input)
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

type insertSliceValue struct {
	valuePath []string
	value     interface{}
}

func (s *CommonSuite) TestAddValueToMap(c *gc.C) {
	for i, t := range []struct {
		should       string
		startingMap  map[string]interface{}
		insertSlices []insertSliceValue
		expectedMap  map[string]interface{}
	}{{
		should: "insert a couple of values",
		startingMap: map[string]interface{}{
			"foo": "bar",
			"bar": map[string]interface{}{
				"baz": "bo",
				"bur": "bor",
			},
		},
		insertSlices: []insertSliceValue{
			{
				valuePath: []string{"well", "now"},
				value:     5,
			},
			{
				valuePath: []string{"foo"},
				value:     "kek",
			},
		},
		expectedMap: map[string]interface{}{
			"foo": "kek",
			"bar": map[string]interface{}{
				"baz": "bo",
				"bur": "bor",
			},
			"well": map[string]interface{}{
				"now": 5,
			},
		},
	}} {
		c.Logf("test %d: should %s", i, t.should)
		for _, sVal := range t.insertSlices {
			addValueToMap(sVal.valuePath, sVal.value, t.startingMap)
		}
		// note addValueToMap mutates target.
		c.Check(t.startingMap, jc.DeepEquals, t.expectedMap)
	}
}
