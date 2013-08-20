// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
)

var annotatorTests = []struct {
	about    string
	initial  map[string]string
	input    map[string]string
	expected map[string]string
	err      string
}{
	{
		about:    "test setting an annotation",
		input:    map[string]string{"mykey": "myvalue"},
		expected: map[string]string{"mykey": "myvalue"},
	},
	{
		about:    "test setting multiple annotations",
		input:    map[string]string{"key1": "value1", "key2": "value2"},
		expected: map[string]string{"key1": "value1", "key2": "value2"},
	},
	{
		about:    "test overriding annotations",
		initial:  map[string]string{"mykey": "myvalue"},
		input:    map[string]string{"mykey": "another-value"},
		expected: map[string]string{"mykey": "another-value"},
	},
	{
		about: "test setting an invalid annotation",
		input: map[string]string{"invalid.key": "myvalue"},
		err:   `cannot update annotations on .*: invalid key "invalid.key"`,
	},
	{
		about:    "test returning a non existent annotation",
		expected: map[string]string{},
	},
	{
		about:    "test removing an annotation",
		initial:  map[string]string{"mykey": "myvalue"},
		input:    map[string]string{"mykey": ""},
		expected: map[string]string{},
	},
	{
		about:    "test removing multiple annotations",
		initial:  map[string]string{"key1": "v1", "key2": "v2", "key3": "v3"},
		input:    map[string]string{"key1": "", "key3": ""},
		expected: map[string]string{"key2": "v2"},
	},
	{
		about:    "test removing/adding annotations in the same transaction",
		initial:  map[string]string{"key1": "value1"},
		input:    map[string]string{"key1": "", "key2": "value2"},
		expected: map[string]string{"key2": "value2"},
	},
	{
		about:    "test removing a non existent annotation",
		input:    map[string]string{"mykey": ""},
		expected: map[string]string{},
	},
	{
		about:    "test passing an empty map",
		input:    map[string]string{},
		expected: map[string]string{},
	},
}

func testAnnotator(c *gc.C, getEntity func() (state.Annotator, error)) {
	for i, t := range annotatorTests {
		c.Logf("test %d. %s", i, t.about)
		entity, err := getEntity()
		c.Assert(err, gc.IsNil)
		err = entity.SetAnnotations(t.initial)
		c.Assert(err, gc.IsNil)
		err = entity.SetAnnotations(t.input)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		// Retrieving single values works as expected.
		for key, value := range t.input {
			v, err := entity.Annotation(key)
			c.Assert(err, gc.IsNil)
			c.Assert(v, gc.Equals, value)
		}
		// The value stored in MongoDB changed.
		ann, err := entity.Annotations()
		c.Assert(err, gc.IsNil)
		c.Assert(ann, gc.DeepEquals, t.expected)
		// Clean up existing annotations.
		cleanup := make(map[string]string)
		for key := range t.expected {
			cleanup[key] = ""
		}
		err = entity.SetAnnotations(cleanup)
		c.Assert(err, gc.IsNil)
	}
}
