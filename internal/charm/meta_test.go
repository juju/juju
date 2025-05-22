// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
)

func repoMeta(c *tc.C, name string) io.Reader {
	charmDir := charmDirPath(c, name)
	file, err := os.Open(filepath.Join(charmDir, "metadata.yaml"))
	c.Assert(err, tc.IsNil)
	defer file.Close()
	data, err := io.ReadAll(file)
	c.Assert(err, tc.IsNil)
	return bytes.NewReader(data)
}

type MetaSuite struct{}

func TestMetaSuite(t *stdtesting.T) {
	tc.Run(t, &MetaSuite{})
}

func (s *MetaSuite) TestReadMetaVersion1(c *tc.C) {
	meta, err := charm.ReadMeta(repoMeta(c, "dummy"))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Name, tc.Equals, "dummy")
	c.Assert(meta.Summary, tc.Equals, "That's a dummy charm.")
	c.Assert(meta.Description, tc.Equals,
		"This is a longer description which\npotentially contains multiple lines.\n")
	c.Assert(meta.Subordinate, tc.Equals, false)
}

func (s *MetaSuite) TestReadMetaVersion2(c *tc.C) {
	// This checks that we can accept a charm with the
	// obsolete "format" field, even though we ignore it.
	meta, err := charm.ReadMeta(repoMeta(c, "format2"))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Name, tc.Equals, "format2")
	c.Assert(meta.Categories, tc.HasLen, 0)
	c.Assert(meta.Terms, tc.HasLen, 0)
}

func (s *MetaSuite) TestValidTermFormat(c *tc.C) {
	valid := []string{
		"foobar",
		"foobar/27",
		"foo/003",
		"owner/foobar/27",
		"owner/foobar",
		"owner/foo-bar",
		"own-er/foobar",
		"ibm/j9-jvm/2",
		"cs:foobar/27",
		"cs:foobar",
	}

	invalid := []string{
		"/",
		"/1",
		"//",
		"//2",
		"27",
		"owner/foo/foobar",
		"@les/term/1",
		"own_er/foobar",
	}

	for i, s := range valid {
		c.Logf("valid test %d: %s", i, s)
		meta := charm.Meta{Terms: []string{s}}
		err := meta.Check(charm.FormatV2, charm.SelectionManifest)
		c.Check(err, tc.ErrorIsNil)
	}

	for i, s := range invalid {
		c.Logf("invalid test %d: %s", i, s)
		meta := charm.Meta{Terms: []string{s}}
		err := meta.Check(charm.FormatV2, charm.SelectionManifest)
		c.Check(err, tc.NotNil)
	}
}

func (s *MetaSuite) TestTermStringRoundTrip(c *tc.C) {
	terms := []string{
		"foobar",
		"foobar/27",
		"owner/foobar/27",
		"owner/foobar",
		"owner/foo-bar",
		"own-er/foobar",
		"ibm/j9-jvm/2",
		"cs:foobar/27",
	}
	for i, term := range terms {
		c.Logf("test %d: %s", i, term)
		id, err := charm.ParseTerm(term)
		c.Check(err, tc.IsNil)
		c.Check(id.String(), tc.Equals, term)
	}
}

func (s *MetaSuite) TestCheckTerms(c *tc.C) {
	tests := []struct {
		about       string
		terms       []string
		expectError string
	}{{
		about: "valid terms",
		terms: []string{"term/1", "term/2", "term-without-revision", "tt/2"},
	}, {
		about:       "revision not a number",
		terms:       []string{"term/1", "term/a"},
		expectError: `wrong term name format "a"`,
	}, {
		about:       "negative revision",
		terms:       []string{"term/-1"},
		expectError: "negative term revision",
	}, {
		about:       "wrong format",
		terms:       []string{"term/1", "foobar/term/abc/1"},
		expectError: `unknown term id format "foobar/term/abc/1"`,
	}, {
		about: "term with owner",
		terms: []string{"term/1", "term/abc/1"},
	}, {
		about: "term with owner no rev",
		terms: []string{"term/1", "term/abc"},
	}, {
		about:       "term may not contain spaces",
		terms:       []string{"term/1", "term about a term"},
		expectError: `wrong term name format "term about a term"`,
	}, {
		about:       "term name must start with lowercase letter",
		terms:       []string{"Term/1"},
		expectError: `wrong term name format "Term"`,
	}, {
		about:       "term name cannot contain capital letters",
		terms:       []string{"owner/foO-Bar"},
		expectError: `wrong term name format "foO-Bar"`,
	}, {
		about:       "term name cannot contain underscores, that's what dashes are for",
		terms:       []string{"owner/foo_bar"},
		expectError: `wrong term name format "foo_bar"`,
	}, {
		about:       "term name can't end with a dash",
		terms:       []string{"o-/1"},
		expectError: `wrong term name format "o-"`,
	}, {
		about:       "term name can't contain consecutive dashes",
		terms:       []string{"o-oo--ooo---o/1"},
		expectError: `wrong term name format "o-oo--ooo---o"`,
	}, {
		about:       "term name more than a single char",
		terms:       []string{"z/1"},
		expectError: `wrong term name format "z"`,
	}, {
		about:       "term name match the regexp",
		terms:       []string{"term_123-23aAf/1"},
		expectError: `wrong term name format "term_123-23aAf"`,
	},
	}
	for i, test := range tests {
		c.Logf("running test %v: %v", i, test.about)
		meta := charm.Meta{Terms: test.terms}
		err := meta.Check(charm.FormatV2, charm.SelectionManifest)
		if test.expectError == "" {
			c.Check(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.ErrorMatches, test.expectError)
		}
	}
}

func (s *MetaSuite) TestParseTerms(c *tc.C) {
	tests := []struct {
		about       string
		term        string
		expectError string
		expectTerm  charm.TermsId
	}{{
		about:      "valid term",
		term:       "term/1",
		expectTerm: charm.TermsId{"", "", "term", 1},
	}, {
		about:      "valid term no revision",
		term:       "term",
		expectTerm: charm.TermsId{"", "", "term", 0},
	}, {
		about:       "revision not a number",
		term:        "term/a",
		expectError: `wrong term name format "a"`,
	}, {
		about:       "negative revision",
		term:        "term/-1",
		expectError: "negative term revision",
	}, {
		about:       "bad revision",
		term:        "owner/term/12a",
		expectError: `invalid revision number "12a" strconv.Atoi: parsing "12a": invalid syntax`,
	}, {
		about:       "wrong format",
		term:        "foobar/term/abc/1",
		expectError: `unknown term id format "foobar/term/abc/1"`,
	}, {
		about:      "term with owner",
		term:       "term/abc/1",
		expectTerm: charm.TermsId{"", "term", "abc", 1},
	}, {
		about:      "term with owner no rev",
		term:       "term/abc",
		expectTerm: charm.TermsId{"", "term", "abc", 0},
	}, {
		about:       "term may not contain spaces",
		term:        "term about a term",
		expectError: `wrong term name format "term about a term"`,
	}, {
		about:       "term name must not start with a number",
		term:        "1Term/1",
		expectError: `wrong term name format "1Term"`,
	}, {
		about:      "full term with tenant",
		term:       "tenant:owner/term/1",
		expectTerm: charm.TermsId{"tenant", "owner", "term", 1},
	}, {
		about:       "bad tenant",
		term:        "tenant::owner/term/1",
		expectError: `wrong owner format ":owner"`,
	}, {
		about:      "ownerless term with tenant",
		term:       "tenant:term/1",
		expectTerm: charm.TermsId{"tenant", "", "term", 1},
	}, {
		about:      "ownerless revisionless term with tenant",
		term:       "tenant:term",
		expectTerm: charm.TermsId{"tenant", "", "term", 0},
	}, {
		about:      "owner/term with tenant",
		term:       "tenant:owner/term",
		expectTerm: charm.TermsId{"tenant", "owner", "term", 0},
	}, {
		about:      "term with tenant",
		term:       "tenant:term",
		expectTerm: charm.TermsId{"tenant", "", "term", 0},
	}}
	for i, test := range tests {
		c.Logf("running test %v: %v", i, test.about)
		term, err := charm.ParseTerm(test.term)
		if test.expectError == "" {
			c.Check(err, tc.ErrorIsNil)
			c.Check(term, tc.DeepEquals, &test.expectTerm)
		} else {
			c.Check(err, tc.ErrorMatches, test.expectError)
			c.Check(term, tc.IsNil)
		}
	}
}

func (s *MetaSuite) TestReadCategory(c *tc.C) {
	meta, err := charm.ReadMeta(repoMeta(c, "category"))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Categories, tc.DeepEquals, []string{"database"})
}

func (s *MetaSuite) TestReadTerms(c *tc.C) {
	meta, err := charm.ReadMeta(repoMeta(c, "terms"))
	c.Assert(err, tc.ErrorIsNil)
	err = meta.Check(charm.FormatV2, charm.SelectionManifest)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(meta.Terms, tc.DeepEquals, []string{"term1/1", "term2", "owner/term3/1"})
}

var metaDataWithInvalidTermsId = `
name: terms
summary: "Sample charm with terms and conditions"
description: |
        That's a boring charm that requires certain terms.
terms: ["!!!/abc"]
`

func (s *MetaSuite) TestCheckReadInvalidTerms(c *tc.C) {
	reader := strings.NewReader(metaDataWithInvalidTermsId)
	meta, err := charm.ReadMeta(reader)
	c.Assert(err, tc.ErrorIsNil)
	err = meta.Check(charm.FormatV2, charm.SelectionManifest)
	c.Assert(err, tc.ErrorMatches, `wrong owner format "!!!"`)
}

func (s *MetaSuite) TestReadTags(c *tc.C) {
	meta, err := charm.ReadMeta(repoMeta(c, "category"))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Tags, tc.DeepEquals, []string{"openstack", "storage"})
}

func (s *MetaSuite) TestSubordinate(c *tc.C) {
	meta, err := charm.ReadMeta(repoMeta(c, "logging"))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Subordinate, tc.Equals, true)
}

func (s *MetaSuite) TestCheckSubordinateWithoutContainerRelation(c *tc.C) {
	r := repoMeta(c, "dummy")
	hackYaml := ReadYaml(r)
	hackYaml["subordinate"] = true
	meta, err := charm.ReadMeta(hackYaml.Reader())
	c.Assert(err, tc.ErrorIsNil)
	err = meta.Check(charm.FormatV2, charm.SelectionManifest)
	c.Assert(err, tc.ErrorMatches, "subordinate charm \"dummy\" lacks \"requires\" relation with container scope")
}

func (s *MetaSuite) TestScopeConstraint(c *tc.C) {
	meta, err := charm.ReadMeta(repoMeta(c, "logging"))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Provides["logging-client"].Scope, tc.Equals, charm.ScopeGlobal)
	c.Assert(meta.Requires["logging-directory"].Scope, tc.Equals, charm.ScopeContainer)
	c.Assert(meta.Subordinate, tc.Equals, true)
}

func (s *MetaSuite) TestParseMetaRelations(c *tc.C) {
	meta, err := charm.ReadMeta(repoMeta(c, "mysql"))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Provides["server"], tc.Equals, charm.Relation{
		Name:      "server",
		Role:      charm.RoleProvider,
		Interface: "mysql",
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Requires, tc.IsNil)
	c.Assert(meta.Peers, tc.IsNil)

	meta, err = charm.ReadMeta(repoMeta(c, "riak"))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Provides["endpoint"], tc.Equals, charm.Relation{
		Name:      "endpoint",
		Role:      charm.RoleProvider,
		Interface: "http",
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Provides["admin"], tc.Equals, charm.Relation{
		Name:      "admin",
		Role:      charm.RoleProvider,
		Interface: "http",
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Peers["ring"], tc.Equals, charm.Relation{
		Name:      "ring",
		Role:      charm.RolePeer,
		Interface: "riak",
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Requires, tc.IsNil)

	meta, err = charm.ReadMeta(repoMeta(c, "terracotta"))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Provides["dso"], tc.Equals, charm.Relation{
		Name:      "dso",
		Role:      charm.RoleProvider,
		Interface: "terracotta",
		Optional:  true,
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Peers["server-array"], tc.Equals, charm.Relation{
		Name:      "server-array",
		Role:      charm.RolePeer,
		Interface: "terracotta-server",
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Requires, tc.IsNil)

	meta, err = charm.ReadMeta(repoMeta(c, "wordpress"))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Provides["url"], tc.Equals, charm.Relation{
		Name:      "url",
		Role:      charm.RoleProvider,
		Interface: "http",
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Requires["db"], tc.Equals, charm.Relation{
		Name:      "db",
		Role:      charm.RoleRequirer,
		Interface: "mysql",
		Limit:     1,
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Requires["cache"], tc.Equals, charm.Relation{
		Name:      "cache",
		Role:      charm.RoleRequirer,
		Interface: "varnish",
		Limit:     2,
		Optional:  true,
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Peers, tc.IsNil)

	meta, err = charm.ReadMeta(repoMeta(c, "monitoring"))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Provides["monitoring-client"], tc.Equals, charm.Relation{
		Name:      "monitoring-client",
		Role:      charm.RoleProvider,
		Interface: "monitoring",
		Scope:     charm.ScopeGlobal,
	})
	c.Assert(meta.Requires["monitoring-port"], tc.Equals, charm.Relation{
		Name:      "monitoring-port",
		Role:      charm.RoleRequirer,
		Interface: "monitoring",
		Scope:     charm.ScopeContainer,
	})
	c.Assert(meta.Requires["info"], tc.Equals, charm.Relation{
		Name:      "info",
		Role:      charm.RoleRequirer,
		Interface: "juju-info",
		Scope:     charm.ScopeContainer,
	})

	c.Assert(meta.Peers, tc.IsNil)
}

func (s *MetaSuite) TestCombinedRelations(c *tc.C) {
	meta, err := charm.ReadMeta(repoMeta(c, "riak"))
	c.Assert(err, tc.IsNil)
	combinedRelations := meta.CombinedRelations()
	expectedLength := len(meta.Provides) + len(meta.Requires) + len(meta.Peers)
	c.Assert(combinedRelations, tc.HasLen, expectedLength)
	c.Assert(combinedRelations, tc.DeepEquals, map[string]charm.Relation{
		"endpoint": {
			Name:      "endpoint",
			Role:      charm.RoleProvider,
			Interface: "http",
			Scope:     charm.ScopeGlobal,
		},
		"admin": {
			Name:      "admin",
			Role:      charm.RoleProvider,
			Interface: "http",
			Scope:     charm.ScopeGlobal,
		},
		"ring": {
			Name:      "ring",
			Role:      charm.RolePeer,
			Interface: "riak",
			Scope:     charm.ScopeGlobal,
		},
	})
}

func (s *MetaSuite) TestParseJujuRelations(c *tc.C) {
	meta, err := charm.ReadMeta(repoMeta(c, "juju-charm"))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Provides["dashboard"], tc.Equals, charm.Relation{
		Name:      "dashboard",
		Role:      charm.RoleProvider,
		Interface: "juju-dashboard",
		Scope:     charm.ScopeGlobal,
	})
}

// dummyMetadata contains a minimally valid charm metadata.yaml
// for testing valid and invalid series.
const dummyMetadata = "name: a\nsummary: b\ndescription: c"

func (s *MetaSuite) TestMinJujuVersion(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(dummyMetadata))
	c.Assert(err, tc.IsNil)
	charmMeta := fmt.Sprintf("%s\nmin-juju-version: ", dummyMetadata)
	vals := []semversion.Number{
		{Major: 1, Minor: 25},
		{Major: 1, Minor: 25, Tag: "alpha"},
		{Major: 1, Minor: 25, Patch: 1},
	}
	for _, ver := range vals {
		val := charmMeta + ver.String()
		meta, err = charm.ReadMeta(strings.NewReader(val))
		c.Assert(err, tc.IsNil)
		c.Assert(meta.MinJujuVersion, tc.Equals, ver)
	}
}

func (s *MetaSuite) TestInvalidMinJujuVersion(c *tc.C) {
	_, err := charm.ReadMeta(strings.NewReader(dummyMetadata + "\nmin-juju-version: invalid-version"))

	c.Check(err, tc.ErrorMatches, `invalid min-juju-version: invalid version "invalid-version"`)
}

func (s *MetaSuite) TestNoMinJujuVersion(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(dummyMetadata))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(meta.MinJujuVersion, tc.Equals, semversion.Zero)
}

func (s *MetaSuite) TestCheckMismatchedExtraBindingName(c *tc.C) {
	meta := charm.Meta{
		Name: "foo",
		ExtraBindings: map[string]charm.ExtraBinding{
			"foo": {Name: "bar"},
		},
	}
	err := meta.Check(charm.FormatV2, charm.SelectionManifest)
	c.Assert(err, tc.ErrorMatches, `charm "foo" has invalid extra bindings: mismatched extra binding name: got "bar", expected "foo"`)
}

func (s *MetaSuite) TestCheckEmptyNameKeyOrEmptyExtraBindingName(c *tc.C) {
	meta := charm.Meta{
		Name:          "foo",
		ExtraBindings: map[string]charm.ExtraBinding{"": {Name: "bar"}},
	}
	err := meta.Check(charm.FormatV2, charm.SelectionManifest)
	expectedError := `charm "foo" has invalid extra bindings: missing binding name`
	c.Assert(err, tc.ErrorMatches, expectedError)

	meta.ExtraBindings = map[string]charm.ExtraBinding{"bar": {Name: ""}}
	err = meta.Check(charm.FormatV2, charm.SelectionManifest)
	c.Assert(err, tc.ErrorMatches, expectedError)
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
func (s *MetaSuite) TestIfaceExpander(c *tc.C) {
	e := charm.IfaceExpander(nil)

	path := []string{"<pa", "th>"}

	// Shorthand is properly rewritten
	v, err := e.Coerce("http", path)
	c.Assert(err, tc.IsNil)
	c.Assert(v, tc.DeepEquals, map[string]interface{}{"interface": "http", "limit": nil, "optional": false, "scope": string(charm.ScopeGlobal)})

	// Defaults are properly applied
	v, err = e.Coerce(map[string]interface{}{"interface": "http"}, path)
	c.Assert(err, tc.IsNil)
	c.Assert(v, tc.DeepEquals, map[string]interface{}{"interface": "http", "limit": nil, "optional": false, "scope": string(charm.ScopeGlobal)})

	v, err = e.Coerce(map[string]interface{}{"interface": "http", "limit": 2}, path)
	c.Assert(err, tc.IsNil)
	c.Assert(v, tc.DeepEquals, map[string]interface{}{"interface": "http", "limit": int64(2), "optional": false, "scope": string(charm.ScopeGlobal)})

	v, err = e.Coerce(map[string]interface{}{"interface": "http", "optional": true}, path)
	c.Assert(err, tc.IsNil)
	c.Assert(v, tc.DeepEquals, map[string]interface{}{"interface": "http", "limit": nil, "optional": true, "scope": string(charm.ScopeGlobal)})

	// Invalid data raises an error.
	_, err = e.Coerce(42, path)
	c.Assert(err, tc.ErrorMatches, `<path>: expected map, got int\(42\)`)

	_, err = e.Coerce(map[string]interface{}{"interface": "http", "optional": nil}, path)
	c.Assert(err, tc.ErrorMatches, "<path>.optional: expected bool, got nothing")

	_, err = e.Coerce(map[string]interface{}{"interface": "http", "limit": "none, really"}, path)
	c.Assert(err, tc.ErrorMatches, "<path>.limit: unexpected value.*")

	// Can change default limit
	e = charm.IfaceExpander(1)
	v, err = e.Coerce(map[string]interface{}{"interface": "http"}, path)
	c.Assert(err, tc.IsNil)
	c.Assert(v, tc.DeepEquals, map[string]interface{}{"interface": "http", "limit": int64(1), "optional": false, "scope": string(charm.ScopeGlobal)})
}

func (s *MetaSuite) TestMetaHooks(c *tc.C) {
	meta, err := charm.ReadMeta(repoMeta(c, "wordpress"))
	c.Assert(err, tc.IsNil)
	hooks := meta.Hooks()
	expectedHooks := map[string]bool{
		"install":                           true,
		"start":                             true,
		"config-changed":                    true,
		"upgrade-charm":                     true,
		"stop":                              true,
		"remove":                            true,
		"leader-elected":                    true,
		"leader-deposed":                    true,
		"update-status":                     true,
		"cache-relation-created":            true,
		"cache-relation-joined":             true,
		"cache-relation-changed":            true,
		"cache-relation-departed":           true,
		"cache-relation-broken":             true,
		"db-relation-created":               true,
		"db-relation-joined":                true,
		"db-relation-changed":               true,
		"db-relation-departed":              true,
		"db-relation-broken":                true,
		"logging-dir-relation-created":      true,
		"logging-dir-relation-joined":       true,
		"logging-dir-relation-changed":      true,
		"logging-dir-relation-departed":     true,
		"logging-dir-relation-broken":       true,
		"monitoring-port-relation-created":  true,
		"monitoring-port-relation-joined":   true,
		"monitoring-port-relation-changed":  true,
		"monitoring-port-relation-departed": true,
		"monitoring-port-relation-broken":   true,
		"url-relation-created":              true,
		"url-relation-joined":               true,
		"url-relation-changed":              true,
		"url-relation-departed":             true,
		"url-relation-broken":               true,
		"secret-changed":                    true,
		"secret-expired":                    true,
		"secret-remove":                     true,
		"secret-rotate":                     true,
	}
	c.Assert(hooks, tc.DeepEquals, expectedHooks)
}

func (s *MetaSuite) TestCodecRoundTripEmpty(c *tc.C) {
	for _, codec := range codecs {
		c.Logf("codec %s", codec.Name)
		empty_input := charm.Meta{}
		data, err := codec.Marshal(empty_input)
		c.Assert(err, tc.IsNil)
		var empty_output charm.Meta
		err = codec.Unmarshal(data, &empty_output)
		c.Assert(err, tc.IsNil)
		c.Assert(empty_input, tc.DeepEquals, empty_output)
	}
}

func (s *MetaSuite) TestCodecRoundTrip(c *tc.C) {
	var input = charm.Meta{
		Name:        "Foo",
		Summary:     "Bar",
		Description: "Baz",
		Subordinate: true,
		Provides: map[string]charm.Relation{
			"qux": {
				Name:      "qux",
				Role:      charm.RoleProvider,
				Interface: "quxx",
				Optional:  true,
				Limit:     42,
				Scope:     charm.ScopeGlobal,
			},
		},
		Requires: map[string]charm.Relation{
			"frob": {
				Name:      "frob",
				Role:      charm.RoleRequirer,
				Interface: "quxx",
				Optional:  true,
				Limit:     42,
				Scope:     charm.ScopeContainer,
			},
		},
		Peers: map[string]charm.Relation{
			"arble": {
				Name:      "arble",
				Role:      charm.RolePeer,
				Interface: "quxx",
				Optional:  true,
				Limit:     42,
				Scope:     charm.ScopeGlobal,
			},
		},
		ExtraBindings: map[string]charm.ExtraBinding{
			"b1": {Name: "b1"},
			"b2": {Name: "b2"},
		},
		Categories: []string{"quxxxx", "quxxxxx"},
		Tags:       []string{"openstack", "storage"},
		Terms:      []string{"test-term/1", "test-term/2"},
	}
	for _, codec := range codecs {
		c.Logf("codec %s", codec.Name)
		data, err := codec.Marshal(input)
		c.Assert(err, tc.IsNil)
		var output charm.Meta
		err = codec.Unmarshal(data, &output)
		c.Assert(err, tc.IsNil)
		c.Assert(output, tc.DeepEquals, input, tc.Commentf("data: %q", data))
	}
}

func (s *MetaSuite) TestCodecRoundTripKubernetes(c *tc.C) {
	var input = charm.Meta{
		Name:        "Foo",
		Summary:     "Bar",
		Description: "Baz",
		Subordinate: true,
		Provides: map[string]charm.Relation{
			"qux": {
				Name:      "qux",
				Role:      charm.RoleProvider,
				Interface: "quxx",
				Optional:  true,
				Limit:     42,
				Scope:     charm.ScopeGlobal,
			},
		},
		Requires: map[string]charm.Relation{
			"frob": {
				Name:      "frob",
				Role:      charm.RoleRequirer,
				Interface: "quxx",
				Optional:  true,
				Limit:     42,
				Scope:     charm.ScopeContainer,
			},
		},
		Peers: map[string]charm.Relation{
			"arble": {
				Name:      "arble",
				Role:      charm.RolePeer,
				Interface: "quxx",
				Optional:  true,
				Limit:     42,
				Scope:     charm.ScopeGlobal,
			},
		},
		ExtraBindings: map[string]charm.ExtraBinding{
			"b1": {Name: "b1"},
			"b2": {Name: "b2"},
		},
		Categories: []string{"quxxxx", "quxxxxx"},
		Tags:       []string{"openstack", "storage"},
		Terms:      []string{"test-term/1", "test-term/2"},
		Containers: map[string]charm.Container{
			"test": {
				Mounts: []charm.Mount{{
					Storage:  "test",
					Location: "/wow/",
				}},
				Resource: "test",
			},
		},
		Resources: map[string]resource.Meta{
			"test": {
				Name: "test",
				Type: resource.TypeContainerImage,
			},
			"test2": {
				Name: "test2",
				Type: resource.TypeContainerImage,
			},
		},
		Storage: map[string]charm.Storage{
			"test": {
				Name:     "test",
				Type:     charm.StorageFilesystem,
				CountMin: 1,
				CountMax: 1,
			},
		},
	}
	for _, codec := range codecs {
		c.Logf("codec %s", codec.Name)
		data, err := codec.Marshal(input)
		c.Assert(err, tc.IsNil)
		var output charm.Meta
		err = codec.Unmarshal(data, &output)
		c.Assert(err, tc.IsNil)
		c.Assert(output, tc.DeepEquals, input, tc.Commentf("data: %q", data))
	}
}

var implementedByTests = []struct {
	ifce     string
	name     string
	role     charm.RelationRole
	scope    charm.RelationScope
	match    bool
	implicit bool
}{
	{"ifce-pro", "pro", charm.RoleProvider, charm.ScopeGlobal, true, false},
	{"blah", "pro", charm.RoleProvider, charm.ScopeGlobal, false, false},
	{"ifce-pro", "blah", charm.RoleProvider, charm.ScopeGlobal, false, false},
	{"ifce-pro", "pro", charm.RoleRequirer, charm.ScopeGlobal, false, false},
	{"ifce-pro", "pro", charm.RoleProvider, charm.ScopeContainer, true, false},

	{"juju-info", "juju-info", charm.RoleProvider, charm.ScopeGlobal, true, true},
	{"blah", "juju-info", charm.RoleProvider, charm.ScopeGlobal, false, false},
	{"juju-info", "blah", charm.RoleProvider, charm.ScopeGlobal, false, false},
	{"juju-info", "juju-info", charm.RoleRequirer, charm.ScopeGlobal, false, false},
	{"juju-info", "juju-info", charm.RoleProvider, charm.ScopeContainer, true, true},

	{"ifce-req", "req", charm.RoleRequirer, charm.ScopeGlobal, true, false},
	{"blah", "req", charm.RoleRequirer, charm.ScopeGlobal, false, false},
	{"ifce-req", "blah", charm.RoleRequirer, charm.ScopeGlobal, false, false},
	{"ifce-req", "req", charm.RolePeer, charm.ScopeGlobal, false, false},
	{"ifce-req", "req", charm.RoleRequirer, charm.ScopeContainer, true, false},

	{"juju-info", "info", charm.RoleRequirer, charm.ScopeContainer, true, false},
	{"blah", "info", charm.RoleRequirer, charm.ScopeContainer, false, false},
	{"juju-info", "blah", charm.RoleRequirer, charm.ScopeContainer, false, false},
	{"juju-info", "info", charm.RolePeer, charm.ScopeContainer, false, false},
	{"juju-info", "info", charm.RoleRequirer, charm.ScopeGlobal, false, false},

	{"ifce-peer", "peer", charm.RolePeer, charm.ScopeGlobal, true, false},
	{"blah", "peer", charm.RolePeer, charm.ScopeGlobal, false, false},
	{"ifce-peer", "blah", charm.RolePeer, charm.ScopeGlobal, false, false},
	{"ifce-peer", "peer", charm.RoleProvider, charm.ScopeGlobal, false, false},
	{"ifce-peer", "peer", charm.RolePeer, charm.ScopeContainer, true, false},
}

func (s *MetaSuite) TestImplementedBy(c *tc.C) {
	for i, t := range implementedByTests {
		c.Logf("test %d", i)
		r := charm.Relation{
			Interface: t.ifce,
			Name:      t.name,
			Role:      t.role,
			Scope:     t.scope,
		}
		c.Assert(r.ImplementedBy(dummyMeta), tc.Equals, t.match)
		c.Assert(r.IsImplicit(), tc.Equals, t.implicit)
	}
}

var metaYAMLMarshalTests = []struct {
	about string
	yaml  string
}{{
	about: "minimal charm",
	yaml: `
name: minimal
description: d
summary: s
`,
}, {
	about: "charm with lots of stuff",
	yaml: `
name: big
description: d
summary: s
subordinate: true
provides:
    provideSimple: someinterface
    provideLessSimple:
        interface: anotherinterface
        optional: true
        scope: container
        limit: 3
requires:
    requireSimple: someinterface
    requireLessSimple:
        interface: anotherinterface
        optional: true
        scope: container
        limit: 3
peers:
    peerSimple: someinterface
    peerLessSimple:
        interface: peery
        optional: true
extra-bindings:
    extraBar:
    extraFoo1:
categories: [c1, c1]
tags: [t1, t2]
series:
    - someseries
resources:
    foo:
        description: 'a description'
        filename: 'x.zip'
    bar:
        filename: 'y.tgz'
        type: file
`,
}, {
	about: "minimal charm with nested assumes block",
	yaml: `
name: minimal-with-assumes
description: d
summary: s
assumes:
- chips
- any-of:
  - guacamole
  - salsa
  - any-of:
    - good-weather
    - great-music
- all-of:
  - table
  - lazy-suzan
`,
}}

func (s *MetaSuite) TestYAMLMarshal(c *tc.C) {
	for i, test := range metaYAMLMarshalTests {
		c.Logf("test %d: %s", i, test.about)
		ch, err := charm.ReadMeta(strings.NewReader(test.yaml))
		c.Assert(err, tc.IsNil)
		gotYAML, err := yaml.Marshal(ch)
		c.Assert(err, tc.IsNil)
		gotCh, err := charm.ReadMeta(bytes.NewReader(gotYAML))
		c.Assert(err, tc.IsNil)
		c.Assert(gotCh, tc.DeepEquals, ch)
	}
}

func (s *MetaSuite) TestYAMLMarshalSimpleRelationOrExtraBinding(c *tc.C) {
	// Check that a simple relation / extra-binding gets marshaled as a string.
	chYAML := `
name: minimal
description: d
summary: s
provides:
    server: http
requires:
    client: http
peers:
     me: http
extra-bindings:
     foo:
`
	ch, err := charm.ReadMeta(strings.NewReader(chYAML))
	c.Assert(err, tc.IsNil)
	gotYAML, err := yaml.Marshal(ch)
	c.Assert(err, tc.IsNil)

	var x interface{}
	err = yaml.Unmarshal(gotYAML, &x)
	c.Assert(err, tc.IsNil)
	c.Assert(x, tc.DeepEquals, map[interface{}]interface{}{
		"name":        "minimal",
		"description": "d",
		"summary":     "s",
		"provides": map[interface{}]interface{}{
			"server": "http",
		},
		"requires": map[interface{}]interface{}{
			"client": "http",
		},
		"peers": map[interface{}]interface{}{
			"me": "http",
		},
		"extra-bindings": map[interface{}]interface{}{
			"foo": nil,
		},
	})
}

func (s *MetaSuite) TestDevices(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
devices:
    bitcoin-miner1:
        description: a big gpu device
        type: gpu
        countmin: 1
        countmax: 1
    bitcoin-miner2:
        description: a nvdia gpu device
        type: nvidia.com/gpu
        countmin: 1
        countmax: 2
    bitcoin-miner3:
        description: an amd gpu device
        type: amd.com/gpu
        countmin: 1
        countmax: 2
`))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Devices, tc.DeepEquals, map[string]charm.Device{
		"bitcoin-miner1": {
			Name:        "bitcoin-miner1",
			Description: "a big gpu device",
			Type:        "gpu",
			CountMin:    1,
			CountMax:    1,
		},
		"bitcoin-miner2": {
			Name:        "bitcoin-miner2",
			Description: "a nvdia gpu device",
			Type:        "nvidia.com/gpu",
			CountMin:    1,
			CountMax:    2,
		},
		"bitcoin-miner3": {
			Name:        "bitcoin-miner3",
			Description: "an amd gpu device",
			Type:        "amd.com/gpu",
			CountMin:    1,
			CountMax:    2,
		},
	}, tc.Commentf("meta: %+v", meta))
}

func (s *MetaSuite) TestDevicesDefaultLimitAndRequest(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
devices:
    bitcoin-miner:
        description: a big gpu device
        type: gpu
`))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Devices, tc.DeepEquals, map[string]charm.Device{
		"bitcoin-miner": {
			Name:        "bitcoin-miner",
			Description: "a big gpu device",
			Type:        "gpu",
			CountMin:    1,
			CountMax:    1,
		},
	}, tc.Commentf("meta: %+v", meta))
}

type testErrorPayload struct {
	desc string
	yaml string
	err  string
}

func testErrors(c *tc.C, prefix string, tests []testErrorPayload) {
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.desc)
		c.Logf("\n%s\n", prefix+test.yaml)
		_, err := charm.ReadMeta(strings.NewReader(prefix + test.yaml))
		c.Assert(err, tc.ErrorMatches, test.err)
	}
}

func testCheckErrors(c *tc.C, prefix string, tests []testErrorPayload) {
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.desc)
		c.Logf("\n%s\n", prefix+test.yaml)
		meta, err := charm.ReadMeta(strings.NewReader(prefix + test.yaml))
		c.Assert(err, tc.ErrorIsNil)
		err = meta.Check(charm.FormatV2, charm.SelectionManifest)
		c.Assert(err, tc.ErrorMatches, test.err)
	}
}

func (s *MetaSuite) TestDevicesErrors(c *tc.C) {
	prefix := `
name: a
summary: b
description: c
devices:
    bad-nvidia-gpu:
`[1:]

	tests := []testErrorPayload{{
		desc: "invalid device type",
		yaml: "        countmin: 0",
		err:  "metadata: devices.bad-nvidia-gpu.type: expected string, got nothing",
	}, {
		desc: "countmax has to be greater than 0",
		yaml: "        countmax: -1\n        description: a big gpu device\n        type: gpu",
		err:  "metadata: invalid device count -1",
	}, {
		desc: "countmin has to be greater than 0",
		yaml: "        countmin: -1\n        description: a big gpu device\n        type: gpu",
		err:  "metadata: invalid device count -1",
	}}

	testErrors(c, prefix, tests)

}

func (s *MetaSuite) TestCheckDevicesErrors(c *tc.C) {
	prefix := `
name: a
summary: b
description: c
devices:
    bad-nvidia-gpu:
`[1:]

	tests := []testErrorPayload{{
		desc: "countmax can not be smaller than countmin",
		yaml: "        countmin: 2\n        countmax: 1\n        description: a big gpu device\n        type: gpu",
		err:  "charm \"a\" device \"bad-nvidia-gpu\": maximum count 1 can not be smaller than minimum count 2",
	}}

	testCheckErrors(c, prefix, tests)

}

func (s *MetaSuite) TestStorage(c *tc.C) {
	// "type" is the only required attribute for storage.
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
storage:
    store0:
        description: woo tee bix
        type: block
    store1:
        type: filesystem
`))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Storage, tc.DeepEquals, map[string]charm.Storage{
		"store0": {
			Name:        "store0",
			Description: "woo tee bix",
			Type:        charm.StorageBlock,
			CountMin:    1, // singleton
			CountMax:    1,
		},
		"store1": {
			Name:     "store1",
			Type:     charm.StorageFilesystem,
			CountMin: 1, // singleton
			CountMax: 1,
		},
	})
}

func (s *MetaSuite) TestStorageErrors(c *tc.C) {
	prefix := `
name: a
summary: b
description: c
storage:
 store-bad:
`[1:]

	tests := []testErrorPayload{{
		desc: "type is required",
		yaml: "  required: false",
		err:  "metadata: storage.store-bad.type: unexpected value <nil>",
	}, {
		desc: "range must be an integer, or integer range (1)",
		yaml: "  type: filesystem\n  multiple:\n   range: woat",
		err:  `metadata: storage.store-bad.multiple.range: value "woat" does not match 'm', 'm-n', or 'm\+'`,
	}, {
		desc: "range must be an integer, or integer range (2)",
		yaml: "  type: filesystem\n  multiple:\n   range: 0-abc",
		err:  `metadata: storage.store-bad.multiple.range: value "0-abc" does not match 'm', 'm-n', or 'm\+'`,
	}, {
		desc: "range must be non-negative",
		yaml: "  type: filesystem\n  multiple:\n    range: -1",
		err:  `metadata: storage.store-bad.multiple.range: invalid count -1`,
	}, {
		desc: "range must be positive",
		yaml: "  type: filesystem\n  multiple:\n    range: 0",
		err:  `metadata: storage.store-bad.multiple.range: invalid count 0`,
	}, {
		desc: "minimum size must parse correctly",
		yaml: "  type: block\n  minimum-size: foo",
		err:  `metadata: expected a non-negative number, got "foo"`,
	}, {
		desc: "minimum size must have valid suffix",
		yaml: "  type: block\n  minimum-size: 10Q",
		err:  `metadata: invalid multiplier suffix "Q", expected one of MGTPEZY`,
	}, {
		desc: "properties must contain valid values",
		yaml: "  type: block\n  properties: [transient, foo]",
		err:  `metadata: .* unexpected value "foo"`,
	}}

	testErrors(c, prefix, tests)
}

func (s *MetaSuite) TestCheckStorageErrors(c *tc.C) {
	prefix := `
name: a
summary: b
description: c
storage:
 store-bad:
`[1:]

	tests := []testErrorPayload{{
		desc: "location cannot be specified for block type storage",
		yaml: "  type: block\n  location: /dev/sdc",
		err:  `charm "a" storage "store-bad": location may not be specified for "type: block"`,
	}}

	testCheckErrors(c, prefix, tests)
}

func (s *MetaSuite) TestStorageCount(c *tc.C) {
	testStorageCount := func(count string, min, max int) {
		meta, err := charm.ReadMeta(strings.NewReader(fmt.Sprintf(`
name: a
summary: b
description: c
storage:
    store0:
        type: filesystem
        multiple:
            range: %s
`, count)))
		c.Assert(err, tc.IsNil)
		store := meta.Storage["store0"]
		c.Assert(store, tc.NotNil)
		c.Assert(store.CountMin, tc.Equals, min)
		c.Assert(store.CountMax, tc.Equals, max)
	}
	testStorageCount("1", 1, 1)
	testStorageCount("0-1", 0, 1)
	testStorageCount("1-1", 1, 1)
	testStorageCount("1+", 1, -1)
	// n- is equivalent to n+
	testStorageCount("1-", 1, -1)
}

func (s *MetaSuite) TestStorageLocation(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
storage:
    store0:
        type: filesystem
        location: /var/lib/things
`))
	c.Assert(err, tc.IsNil)
	store := meta.Storage["store0"]
	c.Assert(store, tc.NotNil)
	c.Assert(store.Location, tc.Equals, "/var/lib/things")
}

func (s *MetaSuite) TestStorageMinimumSize(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
storage:
    store0:
        type: filesystem
        minimum-size: 10G
`))
	c.Assert(err, tc.IsNil)
	store := meta.Storage["store0"]
	c.Assert(store, tc.NotNil)
	c.Assert(store.MinimumSize, tc.Equals, uint64(10*1024))
}

func (s *MetaSuite) TestStorageProperties(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
storage:
    store0:
        type: filesystem
        properties: [transient]
`))
	c.Assert(err, tc.IsNil)
	store := meta.Storage["store0"]
	c.Assert(store, tc.NotNil)
	c.Assert(store.Properties, tc.SameContents, []string{"transient"})
}

func (s *MetaSuite) TestExtraBindings(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
extra-bindings:
    endpoint-1:
    foo:
    bar-42:
`))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.ExtraBindings, tc.DeepEquals, map[string]charm.ExtraBinding{
		"endpoint-1": {
			Name: "endpoint-1",
		},
		"foo": {
			Name: "foo",
		},
		"bar-42": {
			Name: "bar-42",
		},
	})
}

func (s *MetaSuite) TestExtraBindingsEmptyMapError(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
extra-bindings:
`))
	c.Assert(err, tc.ErrorMatches, "metadata: extra-bindings: expected map, got nothing")
	c.Assert(meta, tc.IsNil)
}

func (s *MetaSuite) TestExtraBindingsNonEmptyValueError(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
extra-bindings:
    foo: 42
`))
	c.Assert(err, tc.ErrorMatches, `metadata: extra-bindings.foo: expected empty value, got int\(42\)`)
	c.Assert(meta, tc.IsNil)
}

func (s *MetaSuite) TestExtraBindingsEmptyNameError(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
extra-bindings:
    "":
`))
	c.Assert(err, tc.ErrorMatches, `metadata: extra-bindings: expected non-empty binding name, got string\(""\)`)
	c.Assert(meta, tc.IsNil)
}

func (s *MetaSuite) TestResources(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
resources:
    resource-name:
        type: file
        filename: filename.tgz
        description: "One line that is useful when operators need to push it."
    other-resource:
        type: file
        filename: other.zip
    image-resource:
         type: oci-image
         description: "An image"
`))
	c.Assert(err, tc.IsNil)

	c.Check(meta.Resources, tc.DeepEquals, map[string]resource.Meta{
		"resource-name": {
			Name:        "resource-name",
			Type:        resource.TypeFile,
			Path:        "filename.tgz",
			Description: "One line that is useful when operators need to push it.",
		},
		"other-resource": {
			Name: "other-resource",
			Type: resource.TypeFile,
			Path: "other.zip",
		},
		"image-resource": {
			Name:        "image-resource",
			Type:        resource.TypeContainerImage,
			Description: "An image",
		},
	})
}

func (s *MetaSuite) TestParseResourceMetaOkay(c *tc.C) {
	name := "my-resource"
	data := map[string]interface{}{
		"type":        "file",
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	}
	res, err := charm.ParseResourceMeta(name, data)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res, tc.DeepEquals, resource.Meta{
		Name:        "my-resource",
		Type:        resource.TypeFile,
		Path:        "filename.tgz",
		Description: "One line that is useful when operators need to push it.",
	})
}

func (s *MetaSuite) TestParseResourceMetaMissingName(c *tc.C) {
	name := ""
	data := map[string]interface{}{
		"type":        "file",
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	}
	res, err := charm.ParseResourceMeta(name, data)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res, tc.DeepEquals, resource.Meta{
		Name:        "",
		Type:        resource.TypeFile,
		Path:        "filename.tgz",
		Description: "One line that is useful when operators need to push it.",
	})
}

func (s *MetaSuite) TestParseResourceMetaMissingType(c *tc.C) {
	name := "my-resource"
	data := map[string]interface{}{
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	}
	res, err := charm.ParseResourceMeta(name, data)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res, tc.DeepEquals, resource.Meta{
		Name: "my-resource",
		// Type is the zero value.
		Path:        "filename.tgz",
		Description: "One line that is useful when operators need to push it.",
	})
}

func (s *MetaSuite) TestParseResourceMetaEmptyType(c *tc.C) {
	name := "my-resource"
	data := map[string]interface{}{
		"type":        "",
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	}
	_, err := charm.ParseResourceMeta(name, data)

	c.Check(err, tc.ErrorMatches, `unsupported resource type .*`)
}

func (s *MetaSuite) TestParseResourceMetaUnknownType(c *tc.C) {
	name := "my-resource"
	data := map[string]interface{}{
		"type":        "spam",
		"filename":    "filename.tgz",
		"description": "One line that is useful when operators need to push it.",
	}
	_, err := charm.ParseResourceMeta(name, data)

	c.Check(err, tc.ErrorMatches, `unsupported resource type .*`)
}

func (s *MetaSuite) TestParseResourceMetaMissingPath(c *tc.C) {
	name := "my-resource"
	data := map[string]interface{}{
		"type":        "file",
		"description": "One line that is useful when operators need to push it.",
	}
	res, err := charm.ParseResourceMeta(name, data)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res, tc.DeepEquals, resource.Meta{
		Name:        "my-resource",
		Type:        resource.TypeFile,
		Path:        "",
		Description: "One line that is useful when operators need to push it.",
	})
}

func (s *MetaSuite) TestParseResourceMetaMissingComment(c *tc.C) {
	name := "my-resource"
	data := map[string]interface{}{
		"type":     "file",
		"filename": "filename.tgz",
	}
	res, err := charm.ParseResourceMeta(name, data)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res, tc.DeepEquals, resource.Meta{
		Name:        "my-resource",
		Type:        resource.TypeFile,
		Path:        "filename.tgz",
		Description: "",
	})
}

func (s *MetaSuite) TestParseResourceMetaEmpty(c *tc.C) {
	name := "my-resource"
	data := make(map[string]interface{})
	res, err := charm.ParseResourceMeta(name, data)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res, tc.DeepEquals, resource.Meta{
		Name: "my-resource",
	})
}

func (s *MetaSuite) TestParseResourceMetaNil(c *tc.C) {
	name := "my-resource"
	var data map[string]interface{}
	res, err := charm.ParseResourceMeta(name, data)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res, tc.DeepEquals, resource.Meta{
		Name: "my-resource",
	})
}

func (s *MetaSuite) TestContainers(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
containers:
  foo:
    resource: test-os
    mounts:
      - storage: a
        location: /b/
    uid: 10
    gid: 10
resources:
  test-os:
    type: oci-image
storage:
  a:
    type: filesystem
`))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.Containers, tc.DeepEquals, map[string]charm.Container{
		"foo": {
			Resource: "test-os",
			Mounts: []charm.Mount{{
				Storage:  "a",
				Location: "/b/",
			}},
			Uid: intPtr(10),
			Gid: intPtr(10),
		},
	})
}

func intPtr(i int) *int {
	return &i
}

func (s *MetaSuite) TestInvalidUid(c *tc.C) {
	_, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
containers:
  foo:
    resource: test-os
    uid: 1000
resources:
  test-os:
    type: oci-image
`))
	c.Assert(err, tc.ErrorMatches, `parsing containers: container "foo" has invalid uid 1000: uid cannot be in reserved range 1000-9999`)
}

func (s *MetaSuite) TestInvalidGid(c *tc.C) {
	_, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
containers:
  foo:
    resource: test-os
    gid: 1000
resources:
  test-os:
    type: oci-image
`))
	c.Assert(err, tc.ErrorMatches, `parsing containers: container "foo" has invalid gid 1000: gid cannot be in reserved range 1000-9999`)
}

func (s *MetaSuite) TestSystemReferencesFileResource(c *tc.C) {
	_, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
containers:
  foo:
    resource: test-os
    mounts:
      - storage: a
        location: /b/
resources:
  test-os:
    type: file
    filename: test.json
storage:
  a:
    type: filesystem
`))
	c.Assert(err, tc.ErrorMatches, `parsing containers: referenced resource "test-os" is not a oci-image`)
}

func (s *MetaSuite) TestSystemReferencedMissingResource(c *tc.C) {
	_, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
containers:
  foo:
    resource: test-os
    mounts:
      - storage: a
        location: /b/
storage:
  a:
    type: filesystem
`))
	c.Assert(err, tc.ErrorMatches, `parsing containers: referenced resource "test-os" not found`)
}

func (s *MetaSuite) TestMountMissingStorage(c *tc.C) {
	_, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
containers:
  foo:
    resource: test-os
    mounts:
      - location: /b/
resources:
  test-os:
    type: oci-image
storage:
  a:
    type: filesystem
`))
	c.Assert(err, tc.ErrorMatches, `parsing containers: container "foo": storage must be specified on mount`)
}

func (s *MetaSuite) TestMountMissingLocation(c *tc.C) {
	_, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
containers:
  foo:
    resource: test-os
    mounts:
      - storage: a
resources:
  test-os:
    type: oci-image
storage:
  a:
    type: filesystem
`))
	c.Assert(err, tc.ErrorMatches, `parsing containers: container "foo": location must be specified on mount`)
}

func (s *MetaSuite) TestMountIncorrectStorage(c *tc.C) {
	_, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
containers:
  foo:
    resource: test-os
    mounts:
      - storage: b
        location: /b/
resources:
  test-os:
    type: oci-image
storage:
  a:
    type: filesystem
`))
	c.Assert(err, tc.ErrorMatches, `parsing containers: container "foo": storage "b" not valid`)
}

func (s *MetaSuite) TestFormatV1AndV2Mixing(c *tc.C) {
	_, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
series:
  - focal
containers:
  foo:
    resource: test-os
    mounts:
      - storage: a
        location: /b/
resources:
  test-os:
    type: oci-image
storage:
  a:
    type: filesystem
`))
	c.Assert(err, tc.ErrorMatches, `ambiguous metadata: keys "series" cannot be used with "containers"`)
}

var dummyMeta = &charm.Meta{
	Provides: map[string]charm.Relation{
		"pro": {Interface: "ifce-pro", Scope: charm.ScopeGlobal},
	},
	Requires: map[string]charm.Relation{
		"req":  {Interface: "ifce-req", Scope: charm.ScopeGlobal},
		"info": {Interface: "juju-info", Scope: charm.ScopeContainer},
	},
	Peers: map[string]charm.Relation{
		"peer": {Interface: "ifce-peer", Scope: charm.ScopeGlobal},
	},
}

type FormatMetaSuite struct{}

func TestFormatMetaSuite(t *stdtesting.T) {
	tc.Run(t, &FormatMetaSuite{})
}

func (FormatMetaSuite) TestCheckV1Fails(c *tc.C) {
	meta := charm.Meta{}
	err := meta.Check(charm.FormatV1)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err, tc.ErrorMatches, "charm metadata without bases in manifest not valid")
}

func (FormatMetaSuite) TestCheckV2(c *tc.C) {
	meta := charm.Meta{}
	err := meta.Check(charm.FormatV2, charm.SelectionManifest, charm.SelectionBases)
	c.Assert(err, tc.ErrorIsNil)
}

func (FormatMetaSuite) TestCheckV2NoReasons(c *tc.C) {
	meta := charm.Meta{}
	err := meta.Check(charm.FormatV2)
	c.Assert(err, tc.ErrorMatches, `metadata v2 without manifest.yaml not valid`)
}

func (FormatMetaSuite) TestCheckV2WithMinJujuVersion(c *tc.C) {
	meta := charm.Meta{
		MinJujuVersion: semversion.MustParse("2.0.0"),
	}
	err := meta.Check(charm.FormatV2, charm.SelectionManifest, charm.SelectionBases)
	c.Assert(err, tc.ErrorMatches, `min-juju-version in metadata v2 not valid`)
}

func (s *MetaSuite) TestCharmUser(c *tc.C) {
	meta, err := charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
charm-user: root
`))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.CharmUser, tc.Equals, charm.RunAsRoot)

	meta, err = charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
charm-user: sudoer
`))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.CharmUser, tc.Equals, charm.RunAsSudoer)

	meta, err = charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
charm-user: non-root
`))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.CharmUser, tc.Equals, charm.RunAsNonRoot)

	meta, err = charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
`))
	c.Assert(err, tc.IsNil)
	c.Assert(meta.CharmUser, tc.Equals, charm.RunAsDefault)

	_, err = charm.ReadMeta(strings.NewReader(`
name: a
summary: b
description: c
charm-user: barry
`))
	c.Assert(err, tc.ErrorMatches, `parsing charm-user: invalid charm-user "barry" expected one of root, sudoer or non-root`)
}
