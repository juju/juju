package charm_test

import (
	"bytes"
	"io"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/schema"
	"launchpad.net/juju-core/testing"
	"os"
	"path/filepath"
)

func repoMeta(name string) io.Reader {
	charmDir := testing.Charms.DirPath(name)
	file, err := os.Open(filepath.Join(charmDir, "metadata.yaml"))
	if err != nil {
		panic(err)
	}
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		panic(err)
	}
	return bytes.NewBuffer(data)
}

type MetaSuite struct{}

var _ = Suite(&MetaSuite{})

func (s *MetaSuite) TestReadMeta(c *C) {
	meta, err := charm.ReadMeta(repoMeta("dummy"))
	c.Assert(err, IsNil)
	c.Assert(meta.Name, Equals, "dummy")
	c.Assert(meta.Summary, Equals, "That's a dummy charm.")
	c.Assert(meta.Description, Equals,
		"This is a longer description which\npotentially contains multiple lines.\n")
	c.Assert(meta.OldRevision, Equals, 0)
	c.Assert(meta.Subordinate, Equals, false)
}

func (s *MetaSuite) TestSubordinate(c *C) {
	meta, err := charm.ReadMeta(repoMeta("logging"))
	c.Assert(err, IsNil)
	c.Assert(meta.Subordinate, Equals, true)
}

func (s *MetaSuite) TestSubordinateWithoutContainerRelation(c *C) {
	r := repoMeta("dummy")
	hackYaml := ReadYaml(r)
	hackYaml["subordinate"] = true
	_, err := charm.ReadMeta(hackYaml.Reader())
	c.Assert(err, ErrorMatches, "subordinate charm \"dummy\" lacks requires relation with container scope")
}

func (s *MetaSuite) TestScopeConstraint(c *C) {
	meta, err := charm.ReadMeta(repoMeta("logging"))
	c.Assert(err, IsNil)
	c.Assert(meta.Provides["logging-client"].Scope, Equals, charm.ScopeGlobal)
	c.Assert(meta.Requires["logging-directory"].Scope, Equals, charm.ScopeContainer)
	c.Assert(meta.Subordinate, Equals, true)
}

func (s *MetaSuite) TestParseMetaRelations(c *C) {
	meta, err := charm.ReadMeta(repoMeta("mysql"))
	c.Assert(err, IsNil)
	c.Assert(meta.Provides["server"], Equals, charm.Relation{Interface: "mysql", Scope: charm.ScopeGlobal})
	c.Assert(meta.Requires, IsNil)
	c.Assert(meta.Peers, IsNil)

	meta, err = charm.ReadMeta(repoMeta("riak"))
	c.Assert(err, IsNil)
	c.Assert(meta.Provides["endpoint"], Equals, charm.Relation{Interface: "http", Scope: charm.ScopeGlobal})
	c.Assert(meta.Provides["admin"], Equals, charm.Relation{Interface: "http", Scope: charm.ScopeGlobal})
	c.Assert(meta.Peers["ring"], Equals, charm.Relation{Interface: "riak", Limit: 1, Scope: charm.ScopeGlobal})
	c.Assert(meta.Requires, IsNil)

	meta, err = charm.ReadMeta(repoMeta("wordpress"))
	c.Assert(err, IsNil)
	c.Assert(meta.Provides["url"], Equals, charm.Relation{Interface: "http", Scope: charm.ScopeGlobal})
	c.Assert(meta.Requires["db"], Equals, charm.Relation{Interface: "mysql", Limit: 1, Scope: charm.ScopeGlobal})
	c.Assert(meta.Requires["cache"], Equals, charm.Relation{Interface: "varnish", Limit: 2, Optional: true, Scope: charm.ScopeGlobal})
	c.Assert(meta.Peers, IsNil)

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
func (s *MetaSuite) TestIfaceExpander(c *C) {
	e := charm.IfaceExpander(nil)

	path := []string{"<pa", "th>"}

	// Shorthand is properly rewritten
	v, err := e.Coerce("http", path)
	c.Assert(err, IsNil)
	c.Assert(v, DeepEquals, schema.MapType{"interface": "http", "limit": nil, "optional": false, "scope": charm.ScopeGlobal})

	// Defaults are properly applied
	v, err = e.Coerce(schema.MapType{"interface": "http"}, path)
	c.Assert(err, IsNil)
	c.Assert(v, DeepEquals, schema.MapType{"interface": "http", "limit": nil, "optional": false, "scope": charm.ScopeGlobal})

	v, err = e.Coerce(schema.MapType{"interface": "http", "limit": 2}, path)
	c.Assert(err, IsNil)
	c.Assert(v, DeepEquals, schema.MapType{"interface": "http", "limit": int64(2), "optional": false, "scope": charm.ScopeGlobal})

	v, err = e.Coerce(schema.MapType{"interface": "http", "optional": true}, path)
	c.Assert(err, IsNil)
	c.Assert(v, DeepEquals, schema.MapType{"interface": "http", "limit": nil, "optional": true, "scope": charm.ScopeGlobal})

	// Invalid data raises an error.
	v, err = e.Coerce(42, path)
	c.Assert(err, ErrorMatches, "<path>: expected map, got 42")

	v, err = e.Coerce(schema.MapType{"interface": "http", "optional": nil}, path)
	c.Assert(err, ErrorMatches, "<path>.optional: expected bool, got nothing")

	v, err = e.Coerce(schema.MapType{"interface": "http", "limit": "none, really"}, path)
	c.Assert(err, ErrorMatches, "<path>.limit: unsupported value")

	// Can change default limit
	e = charm.IfaceExpander(1)
	v, err = e.Coerce(schema.MapType{"interface": "http"}, path)
	c.Assert(err, IsNil)
	c.Assert(v, DeepEquals, schema.MapType{"interface": "http", "limit": int64(1), "optional": false, "scope": charm.ScopeGlobal})
}
