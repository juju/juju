package formula_test

import (
	"io/ioutil"
	"testing"
	. "launchpad.net/gocheck"
	"launchpad.net/ensemble/go/formula"
	"launchpad.net/ensemble/go/schema"
	"launchpad.net/goyaml"
	"path/filepath"
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

const dummyMeta = "testrepo/dummy/metadata.yaml"

func (s *S) TestReadMeta(c *C) {
	meta, err := formula.ReadMeta(dummyMeta)
	c.Assert(err, IsNil)
	c.Assert(meta.Name, Equals, "dummy")
	c.Assert(meta.Revision, Equals, 1)
	c.Assert(meta.Summary, Equals, "That's a dummy formula.")
	c.Assert(meta.Description, Equals,
		"This is a longer description which\npotentially contains multiple lines.\n")
}

func (s *S) TestParseMeta(c *C) {
	data, err := ioutil.ReadFile(dummyMeta)
	c.Assert(err, IsNil)

	meta, err := formula.ParseMeta(data)
	c.Assert(err, IsNil)
	c.Assert(meta.Name, Equals, "dummy")
	c.Assert(meta.Revision, Equals, 1)
	c.Assert(meta.Summary, Equals, "That's a dummy formula.")
	c.Assert(meta.Description, Equals,
		"This is a longer description which\npotentially contains multiple lines.\n")
}

func (s *S) TestMetaHeader(c *C) {
	yaml := ReadYaml(dummyMeta)
	yaml["ensemble"] = "foo"
	data := DumpYaml(yaml)

	_, err := formula.ParseMeta(data)
	c.Assert(err, Matches, `ensemble: expected "formula", got "foo"`)
}

func (s *S) TestMetaErrorWithPath(c *C) {
	yaml := ReadYaml(dummyMeta)
	yaml["ensemble"] = "foo"
	data := DumpYaml(yaml)

	path := filepath.Join(c.MkDir(), "mymeta.yaml")

	_, err := formula.ReadMeta(path)
	c.Assert(err, Matches, `.*/.*/mymeta\.yaml.*no such file.*`)

	err = ioutil.WriteFile(path, data, 0644)
	c.Assert(err, IsNil)

	_, err = formula.ReadMeta(path)
	c.Assert(err, Matches, `/.*/mymeta\.yaml: ensemble: expected "formula", got "foo"`)
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


func ReadYaml(path string) map[interface{}]interface{} {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	m := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, m)
	if err != nil {
		panic(err)
	}
	return m
}

func DumpYaml(v interface{}) []byte {
	data, err := goyaml.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
