package state_test

import (
	. "launchpad.net/gocheck"
)

type annotator interface {
	Annotation(key string) string
	Annotations() map[string]string
	Refresh() error
	SetAnnotation(key, value string) error
}

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
		err:   `invalid key "invalid.key"`,
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
		about:    "test removing a non existent annotation",
		input:    map[string]string{"mykey": ""},
		expected: map[string]string{},
	},
}

func testAnnotator(c *C, getEntity func() (annotator, error)) {
loop:
	for i, t := range annotatorTests {
		c.Logf("test %d. %s", i, t.about)
		entity, err := getEntity()
		c.Assert(err, IsNil)
		for key, value := range t.initial {
			err := entity.SetAnnotation(key, value)
			c.Assert(err, IsNil)
		}
		for key, value := range t.input {
			err := entity.SetAnnotation(key, value)
			if t.err != "" {
				c.Assert(err, ErrorMatches, t.err)
				continue loop
			}
		}
		c.Assert(err, IsNil)
		// Retrieving single values works as expected.
		for key, value := range t.input {
			c.Assert(entity.Annotation(key), Equals, value)
		}
		// The value stored in the annotator changed.
		c.Assert(entity.Annotations(), DeepEquals, t.expected)
		err = entity.Refresh()
		c.Assert(err, IsNil)
		// The value stored in MongoDB changed.
		c.Assert(entity.Annotations(), DeepEquals, t.expected)
		// Clean up existing annotations.
		for key := range t.expected {
			err = entity.SetAnnotation(key, "")
		}
	}
}
