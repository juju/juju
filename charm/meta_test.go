// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/testing"
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

func (s *MetaSuite) TestReadMetaVersion1(c *C) {
	meta, err := charm.ReadMeta(repoMeta("dummy"))
	c.Assert(err, IsNil)
	c.Assert(meta.Name, Equals, "dummy")
	c.Assert(meta.Summary, Equals, "That's a dummy charm.")
	c.Assert(meta.Description, Equals,
		"This is a longer description which\npotentially contains multiple lines.\n")
	c.Assert(meta.Format, Equals, 1)
	c.Assert(meta.OldRevision, Equals, 0)
	c.Assert(meta.Subordinate, Equals, false)
}

func (s *MetaSuite) TestReadMetaVersion2(c *C) {
	meta, err := charm.ReadMeta(repoMeta("format2"))
	c.Assert(err, IsNil)
	c.Assert(meta.Name, Equals, "format2")
	c.Assert(meta.Format, Equals, 2)
	c.Assert(meta.Categories, HasLen, 0)
}

func (s *MetaSuite) TestReadCategory(c *C) {
	meta, err := charm.ReadMeta(repoMeta("category"))
	c.Assert(err, IsNil)
	c.Assert(meta.Categories, DeepEquals, []string{"database"})
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
	c.Assert(err, ErrorMatches, "subordinate charm \"dummy\" lacks \"requires\" relation with container scope")
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
	c.Assert(meta.Provides["server"], Equals, charm.Relation{
		Name:      "server",
		Role:      charm.RoleProvider,
		Interface: "mysql",
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Requires, IsNil)
	c.Assert(meta.Peers, IsNil)

	meta, err = charm.ReadMeta(repoMeta("riak"))
	c.Assert(err, IsNil)
	c.Assert(meta.Provides["endpoint"], Equals, charm.Relation{
		Name:      "endpoint",
		Role:      charm.RoleProvider,
		Interface: "http",
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Provides["admin"], Equals, charm.Relation{
		Name:      "admin",
		Role:      charm.RoleProvider,
		Interface: "http",
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Peers["ring"], Equals, charm.Relation{
		Name:      "ring",
		Role:      charm.RolePeer,
		Interface: "riak",
		Limit:     1,
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Requires, IsNil)

	meta, err = charm.ReadMeta(repoMeta("terracotta"))
	c.Assert(err, IsNil)
	c.Assert(meta.Provides["dso"], Equals, charm.Relation{
		Name:      "dso",
		Role:      charm.RoleProvider,
		Interface: "terracotta",
		Optional:  true,
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Peers["server-array"], Equals, charm.Relation{
		Name:      "server-array",
		Role:      charm.RolePeer,
		Interface: "terracotta-server",
		Limit:     1,
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Requires, IsNil)

	meta, err = charm.ReadMeta(repoMeta("wordpress"))
	c.Assert(err, IsNil)
	c.Assert(meta.Provides["url"], Equals, charm.Relation{
		Name:      "url",
		Role:      charm.RoleProvider,
		Interface: "http",
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Requires["db"], Equals, charm.Relation{
		Name:      "db",
		Role:      charm.RoleRequirer,
		Interface: "mysql",
		Limit:     1,
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Requires["cache"], Equals, charm.Relation{
		Name:      "cache",
		Role:      charm.RoleRequirer,
		Interface: "varnish",
		Limit:     2,
		Optional:  true,
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Peers, IsNil)
}

var relationsConstraintsTests = []struct {
	rels string
	err  string
}{
	{
		"provides:\n  foo: ping\nrequires:\n  foo: pong",
		`charm "a" using a duplicated relation name: "foo"`,
	}, {
		"requires:\n  foo: ping\npeers:\n  foo: pong",
		`charm "a" using a duplicated relation name: "foo"`,
	}, {
		"peers:\n  foo: ping\nprovides:\n  foo: pong",
		`charm "a" using a duplicated relation name: "foo"`,
	}, {
		"provides:\n  juju: blob",
		`charm "a" using a reserved relation name: "juju"`,
	}, {
		"requires:\n  juju: blob",
		`charm "a" using a reserved relation name: "juju"`,
	}, {
		"peers:\n  juju: blob",
		`charm "a" using a reserved relation name: "juju"`,
	}, {
		"provides:\n  juju-snap: blub",
		`charm "a" using a reserved relation name: "juju-snap"`,
	}, {
		"requires:\n  juju-crackle: blub",
		`charm "a" using a reserved relation name: "juju-crackle"`,
	}, {
		"peers:\n  juju-pop: blub",
		`charm "a" using a reserved relation name: "juju-pop"`,
	}, {
		"provides:\n  innocuous: juju",
		`charm "a" relation "innocuous" using a reserved interface: "juju"`,
	}, {
		"peers:\n  innocuous: juju",
		`charm "a" relation "innocuous" using a reserved interface: "juju"`,
	}, {
		"provides:\n  innocuous: juju-snap",
		`charm "a" relation "innocuous" using a reserved interface: "juju-snap"`,
	}, {
		"peers:\n  innocuous: juju-snap",
		`charm "a" relation "innocuous" using a reserved interface: "juju-snap"`,
	},
}

func (s *MetaSuite) TestRelationsConstraints(c *C) {
	check := func(s, e string) {
		meta, err := charm.ReadMeta(strings.NewReader(s))
		if e != "" {
			c.Assert(err, ErrorMatches, e)
			c.Assert(meta, IsNil)
		} else {
			c.Assert(err, IsNil)
			c.Assert(meta, NotNil)
		}
	}
	prefix := "name: a\nsummary: b\ndescription: c\n"
	for i, t := range relationsConstraintsTests {
		c.Logf("test %d", i)
		check(prefix+t.rels, t.err)
		check(prefix+"subordinate: true\n"+t.rels, t.err)
	}
	// The juju-* namespace is accessible to container-scoped require
	// relations on subordinate charms.
	check(prefix+`
subordinate: true
requires:
  juju-info:
    interface: juju-info
    scope: container`, "")
	// The juju-* interfaces are allowed on any require relation.
	check(prefix+`
requires:
  innocuous: juju-info`, "")
}

func (s *MetaSuite) TestCheckMismatchedRelationName(c *C) {
	// This  Check case cannot be covered by the above
	// TestRelationsConstraints tests.
	meta := charm.Meta{
		Name: "foo",
		Provides: map[string]charm.Relation{
			"foo": {
				Name:      "foo",
				Role:      charm.RolePeer,
				Interface: "x",
				Limit:     1,
				Scope:     charm.ScopeGlobal,
			},
		},
	}
	err := meta.Check()
	c.Assert(err, ErrorMatches, `charm "foo" has mismatched role "peer"; expected "provider"`)
}

func (s *MetaSuite) TestCheckMismatchedRole(c *C) {
	// This  Check case cannot be covered by the above
	// TestRelationsConstraints tests.
	meta := charm.Meta{
		Name: "foo",
		Provides: map[string]charm.Relation{
			"foo": {
				Role:      charm.RolePeer,
				Interface: "foo",
				Limit:     1,
				Scope:     charm.ScopeGlobal,
			},
		},
	}
	err := meta.Check()
	c.Assert(err, ErrorMatches, `charm "foo" has mismatched relation name ""; expected "foo"`)
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
	c.Assert(v, DeepEquals, map[string]interface{}{"interface": "http", "limit": nil, "optional": false, "scope": string(charm.ScopeGlobal)})

	// Defaults are properly applied
	v, err = e.Coerce(map[string]interface{}{"interface": "http"}, path)
	c.Assert(err, IsNil)
	c.Assert(v, DeepEquals, map[string]interface{}{"interface": "http", "limit": nil, "optional": false, "scope": string(charm.ScopeGlobal)})

	v, err = e.Coerce(map[string]interface{}{"interface": "http", "limit": 2}, path)
	c.Assert(err, IsNil)
	c.Assert(v, DeepEquals, map[string]interface{}{"interface": "http", "limit": int64(2), "optional": false, "scope": string(charm.ScopeGlobal)})

	v, err = e.Coerce(map[string]interface{}{"interface": "http", "optional": true}, path)
	c.Assert(err, IsNil)
	c.Assert(v, DeepEquals, map[string]interface{}{"interface": "http", "limit": nil, "optional": true, "scope": string(charm.ScopeGlobal)})

	// Invalid data raises an error.
	v, err = e.Coerce(42, path)
	c.Assert(err, ErrorMatches, "<path>: expected map, got 42")

	v, err = e.Coerce(map[string]interface{}{"interface": "http", "optional": nil}, path)
	c.Assert(err, ErrorMatches, "<path>.optional: expected bool, got nothing")

	v, err = e.Coerce(map[string]interface{}{"interface": "http", "limit": "none, really"}, path)
	c.Assert(err, ErrorMatches, "<path>.limit: unexpected value.*")

	// Can change default limit
	e = charm.IfaceExpander(1)
	v, err = e.Coerce(map[string]interface{}{"interface": "http"}, path)
	c.Assert(err, IsNil)
	c.Assert(v, DeepEquals, map[string]interface{}{"interface": "http", "limit": int64(1), "optional": false, "scope": string(charm.ScopeGlobal)})
}

func (s *MetaSuite) TestMetaHooks(c *C) {
	meta, err := charm.ReadMeta(repoMeta("wordpress"))
	c.Assert(err, IsNil)
	hooks := meta.Hooks()
	expectedHooks := map[string]bool{
		"install":                       true,
		"start":                         true,
		"config-changed":                true,
		"upgrade-charm":                 true,
		"stop":                          true,
		"cache-relation-joined":         true,
		"cache-relation-changed":        true,
		"cache-relation-departed":       true,
		"cache-relation-broken":         true,
		"db-relation-joined":            true,
		"db-relation-changed":           true,
		"db-relation-departed":          true,
		"db-relation-broken":            true,
		"logging-dir-relation-joined":   true,
		"logging-dir-relation-changed":  true,
		"logging-dir-relation-departed": true,
		"logging-dir-relation-broken":   true,
		"url-relation-joined":           true,
		"url-relation-changed":          true,
		"url-relation-departed":         true,
		"url-relation-broken":           true,
	}
	c.Assert(hooks, DeepEquals, expectedHooks)
}

func (s *MetaSuite) TestCodecRoundTripEmpty(c *C) {
	for i, codec := range codecs {
		c.Logf("codec %d", i)
		empty_input := charm.Meta{}
		data, err := codec.Marshal(empty_input)
		c.Assert(err, IsNil)
		var empty_output charm.Meta
		err = codec.Unmarshal(data, &empty_output)
		c.Assert(err, IsNil)
		c.Assert(empty_input, DeepEquals, empty_output)
	}
}

func (s *MetaSuite) TestCodecRoundTrip(c *C) {
	var input = charm.Meta{
		Name:        "Foo",
		Summary:     "Bar",
		Description: "Baz",
		Subordinate: true,
		Provides: map[string]charm.Relation{
			"qux": {
				Interface: "quxx",
				Optional:  true,
				Limit:     42,
				Scope:     "quxxx",
			},
		},
		Requires: map[string]charm.Relation{
			"qux": {
				Interface: "quxx",
				Optional:  true,
				Limit:     42,
				Scope:     "quxxx",
			},
		},
		Peers: map[string]charm.Relation{
			"qux": {
				Interface: "quxx",
				Optional:  true,
				Limit:     42,
				Scope:     "quxxx",
			},
		},
		Categories:  []string{"quxxxx", "quxxxxx"},
		Format:      10,
		OldRevision: 11,
	}
	for i, codec := range codecs {
		c.Logf("codec %d", i)
		data, err := codec.Marshal(input)
		c.Assert(err, IsNil)
		var output charm.Meta
		err = codec.Unmarshal(data, &output)
		c.Assert(err, IsNil)
		c.Assert(input, DeepEquals, output)
	}
}
