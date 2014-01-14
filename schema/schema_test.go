// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema_test

import (
	"math"
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/schema"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type S struct{}

var _ = gc.Suite(&S{})

type Dummy struct{}

func (d *Dummy) Coerce(value interface{}, path []string) (coerced interface{}, err error) {
	return "i-am-dummy", nil
}

var aPath = []string{"<pa", "th>"}

func (s *S) TestConst(c *gc.C) {
	sch := schema.Const("foo")

	out, err := sch.Coerce("foo", aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, "foo")

	out, err = sch.Coerce(42, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: expected "foo", got int\(42\)`)

	out, err = sch.Coerce(nil, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: expected "foo", got nothing`)
}

func (s *S) TestAny(c *gc.C) {
	sch := schema.Any()

	out, err := sch.Coerce("foo", aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, "foo")

	out, err = sch.Coerce(nil, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, nil)
}

func (s *S) TestOneOf(c *gc.C) {
	sch := schema.OneOf(schema.Const("foo"), schema.Const(42))

	out, err := sch.Coerce("foo", aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, "foo")

	out, err = sch.Coerce(42, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, 42)

	out, err = sch.Coerce("bar", aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: unexpected value "bar"`)
}

func (s *S) TestBool(c *gc.C) {
	sch := schema.Bool()

	for _, trueValue := range []interface{}{true, "1", "true", "True", "TRUE"} {
		out, err := sch.Coerce(trueValue, aPath)
		c.Assert(err, gc.IsNil)
		c.Assert(out, gc.Equals, true)
	}

	for _, falseValue := range []interface{}{false, "0", "false", "False", "FALSE"} {
		out, err := sch.Coerce(falseValue, aPath)
		c.Assert(err, gc.IsNil)
		c.Assert(out, gc.Equals, false)
	}

	out, err := sch.Coerce(42, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: expected bool, got int\(42\)`)

	out, err = sch.Coerce(nil, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected bool, got nothing")
}

func (s *S) TestInt(c *gc.C) {
	sch := schema.Int()

	out, err := sch.Coerce(42, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, int64(42))

	out, err = sch.Coerce(int8(42), aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, int64(42))

	out, err = sch.Coerce("42", aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, int64(42))

	out, err = sch.Coerce(true, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: expected int, got bool\(true\)`)

	out, err = sch.Coerce(nil, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected int, got nothing")
}

func (s *S) TestForceInt(c *gc.C) {
	sch := schema.ForceInt()

	out, err := sch.Coerce(42, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, int(42))

	out, err = sch.Coerce("42", aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, int(42))

	out, err = sch.Coerce("42.66", aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, int(42))

	out, err = sch.Coerce(int8(42), aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, int(42))

	out, err = sch.Coerce(float32(42), aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, int(42))

	out, err = sch.Coerce(float64(42), aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, int(42))

	out, err = sch.Coerce(42.66, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, int(42))

	// If an out of range value is provided, that value is truncated,
	// generating unexpected results, but no error is raised.
	out, err = sch.Coerce(float64(math.MaxInt64+1), aPath)
	c.Assert(err, gc.IsNil)

	out, err = sch.Coerce(true, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: expected number, got bool\(true\)`)

	out, err = sch.Coerce(nil, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected number, got nothing")
}

func (s *S) TestFloat(c *gc.C) {
	sch := schema.Float()

	out, err := sch.Coerce(float32(1.0), aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, float64(1.0))

	out, err = sch.Coerce(float64(1.0), aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, float64(1.0))

	out, err = sch.Coerce(true, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: expected float, got bool\(true\)`)

	out, err = sch.Coerce(nil, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected float, got nothing")
}

func (s *S) TestString(c *gc.C) {
	sch := schema.String()

	out, err := sch.Coerce("foo", aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, "foo")

	out, err = sch.Coerce(true, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: expected string, got bool\(true\)`)

	out, err = sch.Coerce(nil, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected string, got nothing")
}

func (s *S) TestSimpleRegexp(c *gc.C) {
	sch := schema.SimpleRegexp()
	out, err := sch.Coerce("[0-9]+", aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.Equals, "[0-9]+")

	out, err = sch.Coerce(1, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: expected regexp string, got int\(1\)`)

	out, err = sch.Coerce("[", aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: expected valid regexp, got string\("\["\)`)

	out, err = sch.Coerce(nil, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: expected regexp string, got nothing`)
}

func (s *S) TestList(c *gc.C) {
	sch := schema.List(schema.Int())
	out, err := sch.Coerce([]int8{1, 2}, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.DeepEquals, []interface{}{int64(1), int64(2)})

	out, err = sch.Coerce(42, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected list, got int\\(42\\)")

	out, err = sch.Coerce(nil, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected list, got nothing")

	out, err = sch.Coerce([]interface{}{1, true}, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>\[1\]: expected int, got bool\(true\)`)
}

func (s *S) TestMap(c *gc.C) {
	sch := schema.Map(schema.String(), schema.Int())
	out, err := sch.Coerce(map[string]interface{}{"a": 1, "b": int8(2)}, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.DeepEquals, map[interface{}]interface{}{"a": int64(1), "b": int64(2)})

	out, err = sch.Coerce(42, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected map, got int\\(42\\)")

	out, err = sch.Coerce(nil, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected map, got nothing")

	out, err = sch.Coerce(map[int]int{1: 1}, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected string, got int\\(1\\)")

	out, err = sch.Coerce(map[string]bool{"a": true}, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>\.a: expected int, got bool\(true\)`)

	// First path entry shouldn't have dots in an error message.
	out, err = sch.Coerce(map[string]bool{"a": true}, nil)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `a: expected int, got bool\(true\)`)
}

func (s *S) TestStringMap(c *gc.C) {
	sch := schema.StringMap(schema.Int())
	out, err := sch.Coerce(map[string]interface{}{"a": 1, "b": int8(2)}, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.DeepEquals, map[string]interface{}{"a": int64(1), "b": int64(2)})

	out, err = sch.Coerce(42, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected map, got int\\(42\\)")

	out, err = sch.Coerce(nil, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected map, got nothing")

	out, err = sch.Coerce(map[int]int{1: 1}, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected string, got int\\(1\\)")

	out, err = sch.Coerce(map[string]bool{"a": true}, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>\.a: expected int, got bool\(true\)`)

	// First path entry shouldn't have dots in an error message.
	out, err = sch.Coerce(map[string]bool{"a": true}, nil)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `a: expected int, got bool\(true\)`)
}

func assertFieldMap(c *gc.C, sch schema.Checker) {
	out, err := sch.Coerce(map[string]interface{}{"a": "A", "b": "B"}, aPath)

	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.DeepEquals, map[string]interface{}{"a": "A", "b": "B", "c": "C"})

	out, err = sch.Coerce(42, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected map, got int\\(42\\)")

	out, err = sch.Coerce(nil, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "<path>: expected map, got nothing")

	out, err = sch.Coerce(map[string]interface{}{"a": "A", "b": "C"}, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>\.b: expected "B", got string\("C"\)`)

	out, err = sch.Coerce(map[string]interface{}{"b": "B"}, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>\.a: expected "A", got nothing`)

	// b is optional
	out, err = sch.Coerce(map[string]interface{}{"a": "A"}, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.DeepEquals, map[string]interface{}{"a": "A", "c": "C"})

	// First path entry shouldn't have dots in an error message.
	out, err = sch.Coerce(map[string]bool{"a": true}, nil)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `a: expected "A", got bool\(true\)`)
}

func (s *S) TestFieldMap(c *gc.C) {
	fields := schema.Fields{
		"a": schema.Const("A"),
		"b": schema.Const("B"),
		"c": schema.Const("C"),
	}
	defaults := schema.Defaults{
		"b": schema.Omit,
		"c": "C",
	}
	sch := schema.FieldMap(fields, defaults)
	assertFieldMap(c, sch)

	out, err := sch.Coerce(map[string]interface{}{"a": "A", "b": "B", "d": "D"}, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.DeepEquals, map[string]interface{}{"a": "A", "b": "B", "c": "C"})

	out, err = sch.Coerce(map[string]interface{}{"a": "A", "d": "D"}, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.DeepEquals, map[string]interface{}{"a": "A", "c": "C"})
}

func (s *S) TestFieldMapDefaultInvalid(c *gc.C) {
	fields := schema.Fields{
		"a": schema.Const("A"),
	}
	defaults := schema.Defaults{
		"a": "B",
	}
	sch := schema.FieldMap(fields, defaults)
	_, err := sch.Coerce(map[string]interface{}{}, aPath)
	c.Assert(err, gc.ErrorMatches, `<path>.a: expected "A", got string\("B"\)`)
}

func (s *S) TestStrictFieldMap(c *gc.C) {
	fields := schema.Fields{
		"a": schema.Const("A"),
		"b": schema.Const("B"),
		"c": schema.Const("C"),
	}
	defaults := schema.Defaults{
		"b": schema.Omit,
		"c": "C",
	}
	sch := schema.StrictFieldMap(fields, defaults)
	assertFieldMap(c, sch)

	out, err := sch.Coerce(map[string]interface{}{"a": "A", "b": "B", "d": "D"}, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: unknown key "d" \(value "D"\)`)
}

func (s *S) TestSchemaMap(c *gc.C) {
	fields1 := schema.FieldMap(schema.Fields{
		"type": schema.Const(1),
		"a":    schema.Const(2),
	}, nil)
	fields2 := schema.FieldMap(schema.Fields{
		"type": schema.Const(3),
		"b":    schema.Const(4),
	}, nil)
	sch := schema.FieldMapSet("type", []schema.Checker{fields1, fields2})

	out, err := sch.Coerce(map[string]int{"type": 1, "a": 2}, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.DeepEquals, map[string]interface{}{"type": 1, "a": 2})

	out, err = sch.Coerce(map[string]int{"type": 3, "b": 4}, aPath)
	c.Assert(err, gc.IsNil)
	c.Assert(out, gc.DeepEquals, map[string]interface{}{"type": 3, "b": 4})

	out, err = sch.Coerce(map[string]int{}, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>\.type: expected supported selector, got nothing`)

	out, err = sch.Coerce(map[string]int{"type": 2}, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>\.type: expected supported selector, got int\(2\)`)

	out, err = sch.Coerce(map[string]int{"type": 3, "b": 5}, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>\.b: expected 4, got int\(5\)`)

	out, err = sch.Coerce(42, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: expected map, got int\(42\)`)

	out, err = sch.Coerce(nil, aPath)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `<path>: expected map, got nothing`)

	// First path entry shouldn't have dots in an error message.
	out, err = sch.Coerce(map[string]int{"a": 1}, nil)
	c.Assert(out, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `type: expected supported selector, got nothing`)
}
