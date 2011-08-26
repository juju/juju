package formula_test

import (
	"testing"
	. "launchpad.net/gocheck"
	"launchpad.net/ensemble/go/formula"
	"launchpad.net/ensemble/go/schema"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct{}

var _ = Suite(&S{})

func (s *S) TestParseId(c *C) {
	namespace, name, rev, err := formula.ParseId("local:mysql-21")
	c.Assert(err, IsNil)
	c.Assert(namespace, Equals, "local")
	c.Assert(name, Equals, "mysql")
	c.Assert(rev, Equals, 21)

	namespace, name, rev, err = formula.ParseId("local:mysql-cluster-21")
	c.Assert(err, IsNil)
	c.Assert(namespace, Equals, "local")
	c.Assert(name, Equals, "mysql-cluster")
	c.Assert(rev, Equals, 21)

	_, _, _, err = formula.ParseId("foo")
	c.Assert(err, Matches, `Missing formula namespace: "foo"`)

	_, _, _, err = formula.ParseId("local:foo-x")
	c.Assert(err, Matches, `Missing formula revision: "local:foo-x"`)
}

// Test rewriting of a given interface specification into long form.
//
// InterfaceExpander uses `coerce` to do one of two things:
//
//   - Rewrite shorthand to the long form used for actual storage
//   - Fills in defaults, including a configurable `limit`
//
// This test ensures test coverage on each of these branches, along
// with ensuring the conversion object properly raises SchemaError
// exceptions on invalid data.
func (s *S) TestIfaceExpander(c *C) {
	e := formula.IfaceExpander(nil)

	path := []string{"<pa", "th>"}

	// Shorthand is properly rewritten
	v, err := e.Coerce("http", path)
	c.Assert(err, IsNil)
	c.Assert(v, Equals, schema.M{"interface": "http", "limit": nil, "optional": false})

	// Defaults are properly applied
	v, err = e.Coerce(schema.M{"interface": "http"}, path)
	c.Assert(err, IsNil)
	c.Assert(v, Equals, schema.M{"interface": "http", "limit": nil, "optional": false})

	v, err = e.Coerce(schema.M{"interface": "http", "limit": 2}, path)
	c.Assert(err, IsNil)
	c.Assert(v, Equals, schema.M{"interface": "http", "limit": int64(2), "optional": false})

	v, err = e.Coerce(schema.M{"interface": "http", "optional": true}, path)
	c.Assert(err, IsNil)
	c.Assert(v, Equals, schema.M{"interface": "http", "limit": nil, "optional": true})

	// Invalid data raises an error.
	v, err = e.Coerce(42, path)
	c.Assert(err, Matches, "<path>: expected map, got 42")

	v, err = e.Coerce(schema.M{"interface": "http", "optional": nil}, path)
	c.Assert(err, Matches, "<path>.optional: expected bool, got nothing")

	v, err = e.Coerce(schema.M{"interface": "http", "limit": "none, really"}, path)
	c.Assert(err, Matches, "<path>.limit: unsupported value")

	// Can change default limit
	e = formula.IfaceExpander(1)
	v, err = e.Coerce(schema.M{"interface": "http"}, path)
	c.Assert(err, IsNil)
	c.Assert(v, Equals, schema.M{"interface": "http", "limit": int64(1), "optional": false})
}
