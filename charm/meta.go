package charm

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/schema"
	"strings"
)

// RelationScope describes the scope of a relation endpoint.
type RelationScope string

// Note that schema doesn't support custom string types,
// so when we use these values in a schema.Checker,
// we must store them as strings, not RelationScopes.

const (
	ScopeGlobal    RelationScope = "global"
	ScopeContainer RelationScope = "container"
)

// Relation represents a single relation defined in the charm
// metadata.yaml file.
type Relation struct {
	Interface string
	Optional  bool
	Limit     int
	Scope     RelationScope
}

// Meta represents all the known content that may be defined
// within a charm's metadata.yaml file.
type Meta struct {
	Name        string
	Summary     string
	Description string
	Subordinate bool
	Provides    map[string]Relation `bson:",omitempty"`
	Requires    map[string]Relation `bson:",omitempty"`
	Peers       map[string]Relation `bson:",omitempty"`
	Format      int                 `bson:",omitempty"`
	OldRevision int                 `bson:",omitempty"` // Obsolete
}

// ReadMeta reads the content of a metadata.yaml file and returns
// its representation.
func ReadMeta(r io.Reader) (meta *Meta, err error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return
	}
	raw := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, raw)
	if err != nil {
		return
	}
	v, err := charmSchema.Coerce(raw, nil)
	if err != nil {
		return nil, errors.New("metadata: " + err.Error())
	}
	m := v.(map[string]interface{})
	meta = &Meta{}
	meta.Name = m["name"].(string)
	// Schema decodes as int64, but the int range should be good
	// enough for revisions.
	meta.Summary = m["summary"].(string)
	meta.Description = m["description"].(string)
	meta.Provides = parseRelations(m["provides"])
	meta.Requires = parseRelations(m["requires"])
	meta.Peers = parseRelations(m["peers"])
	meta.Format = int(m["format"].(int64))
	if rev := m["revision"]; rev != nil {
		// Obsolete
		meta.OldRevision = int(m["revision"].(int64))
	}

	// Check for duplicate or forbidden relation names.
	names := map[string]bool{}
	checkName := func(name string) error {
		if reservedName(name) {
			return fmt.Errorf("charm %q using a reserved relation name: %q", meta.Name, name)
		}
		if names[name] {
			return fmt.Errorf("charm %q using a duplicated relation name: %q", meta.Name, name)
		}
		names[name] = true
		return nil
	}
	for name, rel := range meta.Provides {
		if err := checkName(name); err != nil {
			return nil, err
		}
		if reservedName(rel.Interface) {
			return nil, fmt.Errorf("charm %q relation %q using a reserved provider interface: %q", meta.Name, name, rel.Interface)
		}
	}
	for name := range meta.Requires {
		if err := checkName(name); err != nil {
			return nil, err
		}
	}
	for name := range meta.Peers {
		if err := checkName(name); err != nil {
			return nil, err
		}
	}

	// Subordinate charms must have at least one relation that
	// has container scope, otherwise they can't relate to the
	// principal.
	if subordinate := m["subordinate"]; subordinate != nil {
		valid := false
		if meta.Requires != nil {
			for _, relationData := range meta.Requires {
				if relationData.Scope == ScopeContainer {
					valid = true
					break
				}
			}
		}
		if !valid {
			return nil, fmt.Errorf("subordinate charm %q lacks requires relation with container scope", meta.Name)
		}
		meta.Subordinate = m["subordinate"].(bool)
	}
	return
}

func reservedName(name string) bool {
	return name == "juju" || strings.HasPrefix(name, "juju-")
}

func parseRelations(relations interface{}) map[string]Relation {
	if relations == nil {
		return nil
	}
	result := make(map[string]Relation)
	for name, rel := range relations.(map[string]interface{}) {
		relMap := rel.(map[string]interface{})
		relation := Relation{}
		relation.Interface = relMap["interface"].(string)
		relation.Optional = relMap["optional"].(bool)
		if scope := relMap["scope"]; scope != nil {
			relation.Scope = RelationScope(scope.(string))
		}
		if relMap["limit"] != nil {
			// Schema defaults to int64, but we know
			// the int range should be more than enough.
			relation.Limit = int(relMap["limit"].(int64))
		}
		result[name] = relation
	}
	return result
}

// Schema coercer that expands the interface shorthand notation.
// A consistent format is easier to work with than considering the
// potential difference everywhere.
//
// Supports the following variants::
//
//   provides:
//     server: riak
//     admin: http
//     foobar:
//       interface: blah
//
//   provides:
//     server:
//       interface: mysql
//       limit:
//       optional: false
//
// In all input cases, the output is the fully specified interface
// representation as seen in the mysql interface description above.
func ifaceExpander(limit interface{}) schema.Checker {
	return ifaceExpC{limit}
}

type ifaceExpC struct {
	limit interface{}
}

var (
	stringC = schema.String()
	mapC    = schema.StringMap(schema.Any())
)

func (c ifaceExpC) Coerce(v interface{}, path []string) (newv interface{}, err error) {
	s, err := stringC.Coerce(v, path)
	if err == nil {
		newv = map[string]interface{}{
			"interface": s,
			"limit":     c.limit,
			"optional":  false,
			"scope":     string(ScopeGlobal),
		}
		return
	}

	v, err = mapC.Coerce(v, path)
	if err != nil {
		return
	}
	m := v.(map[string]interface{})
	if _, ok := m["limit"]; !ok {
		m["limit"] = c.limit
	}
	return ifaceSchema.Coerce(m, path)
}

var ifaceSchema = schema.FieldMap(
	schema.Fields{
		"interface": schema.String(),
		"limit":     schema.OneOf(schema.Const(nil), schema.Int()),
		"scope":     schema.OneOf(schema.Const(string(ScopeGlobal)), schema.Const(string(ScopeContainer))),
		"optional":  schema.Bool(),
	},
	schema.Defaults{
		"scope":    string(ScopeGlobal),
		"optional": false,
	},
)

var charmSchema = schema.FieldMap(
	schema.Fields{
		"name":        schema.String(),
		"summary":     schema.String(),
		"description": schema.String(),
		"peers":       schema.StringMap(ifaceExpander(int64(1))),
		"provides":    schema.StringMap(ifaceExpander(nil)),
		"requires":    schema.StringMap(ifaceExpander(int64(1))),
		"revision":    schema.Int(), // Obsolete
		"format":      schema.Int(),
		"subordinate": schema.Bool(),
	},
	schema.Defaults{
		"provides":    schema.Omit,
		"requires":    schema.Omit,
		"peers":       schema.Omit,
		"revision":    schema.Omit,
		"format":      1,
		"subordinate": schema.Omit,
	},
)
