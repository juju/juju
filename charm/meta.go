package charm

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/juju/schema"
)

const (
	ScopeGlobal    = "global"
	ScopeContainer = "container"
)

// Relation represents a single relation defined in the charm
// metadata.yaml file.
type Relation struct {
	Interface string
	Optional  bool
	Limit     int
	Scope     string
}

// Meta represents all the known content that may be defined
// within a charm's metadata.yaml file.
type Meta struct {
	Name        string
	Summary     string
	Description string
	Provides    map[string]Relation
	Requires    map[string]Relation
	Peers       map[string]Relation
	OldRevision int // Obsolete
	Subordinate bool
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
	m := v.(schema.MapType)
	meta = &Meta{}
	meta.Name = m["name"].(string)
	// Schema decodes as int64, but the int range should be good
	// enough for revisions.
	meta.Summary = m["summary"].(string)
	meta.Description = m["description"].(string)
	meta.Provides = parseRelations(m["provides"])
	meta.Requires = parseRelations(m["requires"])
	meta.Peers = parseRelations(m["peers"])
	if rev := m["revision"]; rev != nil {
		// Obsolete
		meta.OldRevision = int(m["revision"].(int64))
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

func parseRelations(relations interface{}) map[string]Relation {
	if relations == nil {
		return nil
	}
	result := make(map[string]Relation)
	for name, rel := range relations.(schema.MapType) {
		relMap := rel.(schema.MapType)
		relation := Relation{}
		relation.Interface = relMap["interface"].(string)
		relation.Optional = relMap["optional"].(bool)
		if scope := relMap["scope"]; scope != nil {
			relation.Scope = scope.(string)
		}
		if relMap["limit"] != nil {
			// Schema defaults to int64, but we know
			// the int range should be more than enough.
			relation.Limit = int(relMap["limit"].(int64))
		}
		result[name.(string)] = relation
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
	mapC    = schema.Map(schema.String(), schema.Any())
)

func (c ifaceExpC) Coerce(v interface{}, path []string) (newv interface{}, err error) {
	s, err := stringC.Coerce(v, path)
	if err == nil {
		newv = schema.MapType{
			"interface": s,
			"limit":     c.limit,
			"optional":  false,
			"scope":     ScopeGlobal,
		}
		return
	}

	// Optional values are context-sensitive and/or have
	// defaults, which is different than what KeyDict can
	// readily support. So just do it here first, then
	// coerce to the real schema.
	v, err = mapC.Coerce(v, path)
	if err != nil {
		return
	}
	m := v.(schema.MapType)
	if _, ok := m["limit"]; !ok {
		m["limit"] = c.limit
	}
	if _, ok := m["optional"]; !ok {
		m["optional"] = false
	}
	if _, ok := m["scope"]; !ok {
		m["scope"] = ScopeGlobal
	}
	return ifaceSchema.Coerce(m, path)
}

var ifaceSchema = schema.FieldMap(
	schema.Fields{
		"interface": schema.String(),
		"limit":     schema.OneOf(schema.Const(nil), schema.Int()),
		"scope":     schema.OneOf(schema.Const(ScopeGlobal), schema.Const(ScopeContainer)),
		"optional":  schema.Bool(),
	},
	schema.Optional{"scope"},
)

var charmSchema = schema.FieldMap(
	schema.Fields{
		"name":        schema.String(),
		"summary":     schema.String(),
		"description": schema.String(),
		"peers":       schema.Map(schema.String(), ifaceExpander(1)),
		"provides":    schema.Map(schema.String(), ifaceExpander(nil)),
		"requires":    schema.Map(schema.String(), ifaceExpander(1)),
		"revision":    schema.Int(), // Obsolete
		"subordinate": schema.Bool(),
	},
	schema.Optional{"provides", "requires", "peers", "revision", "subordinate"},
)
