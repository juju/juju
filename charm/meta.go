// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"launchpad.net/goyaml"

	"launchpad.net/juju-core/charm/hooks"
	"launchpad.net/juju-core/schema"
)

// RelationScope describes the scope of a relation.
type RelationScope string

// Note that schema doesn't support custom string types,
// so when we use these values in a schema.Checker,
// we must store them as strings, not RelationScopes.

const (
	ScopeGlobal    RelationScope = "global"
	ScopeContainer RelationScope = "container"
)

// RelationRole defines the role of a relation.
type RelationRole string

const (
	RoleProvider RelationRole = "provider"
	RoleRequirer RelationRole = "requirer"
	RolePeer     RelationRole = "peer"
)

// Relation represents a single relation defined in the charm
// metadata.yaml file.
type Relation struct {
	Name      string
	Role      RelationRole
	Interface string
	Optional  bool
	Limit     int
	Scope     RelationScope
}

// ImplementedBy returns whether the relation is implemented by the supplied charm.
func (r Relation) ImplementedBy(ch Charm) bool {
	if r.IsImplicit() {
		return true
	}
	var m map[string]Relation
	switch r.Role {
	case RoleProvider:
		m = ch.Meta().Provides
	case RoleRequirer:
		m = ch.Meta().Requires
	case RolePeer:
		m = ch.Meta().Peers
	default:
		panic(fmt.Errorf("unknown relation role %q", r.Role))
	}
	rel, found := m[r.Name]
	if !found {
		return false
	}
	if rel.Interface == r.Interface {
		switch r.Scope {
		case ScopeGlobal:
			return rel.Scope != ScopeContainer
		case ScopeContainer:
			return true
		default:
			panic(fmt.Errorf("unknown relation scope %q", r.Scope))
		}
	}
	return false
}

// IsImplicit returns whether the relation is supplied by juju itself,
// rather than by a charm.
func (r Relation) IsImplicit() bool {
	return (r.Name == "juju-info" &&
		r.Interface == "juju-info" &&
		r.Role == RoleProvider)
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
	Categories  []string            `bson:",omitempty"`
	Series      string              `bson:",omitempty"`
}

func generateRelationHooks(relName string, allHooks map[string]bool) {
	for _, hookName := range hooks.RelationHooks() {
		allHooks[fmt.Sprintf("%s-%s", relName, hookName)] = true
	}
}

// Hooks returns a map of all possible valid hooks, taking relations
// into account. It's a map to enable fast lookups, and the value is
// always true.
func (m Meta) Hooks() map[string]bool {
	allHooks := make(map[string]bool)
	// Unit hooks
	for _, hookName := range hooks.UnitHooks() {
		allHooks[string(hookName)] = true
	}
	// Relation hooks
	for hookName := range m.Provides {
		generateRelationHooks(hookName, allHooks)
	}
	for hookName := range m.Requires {
		generateRelationHooks(hookName, allHooks)
	}
	for hookName := range m.Peers {
		generateRelationHooks(hookName, allHooks)
	}
	return allHooks
}

func parseCategories(categories interface{}) []string {
	if categories == nil {
		return nil
	}
	slice := categories.([]interface{})
	result := make([]string, 0, len(slice))
	for _, cat := range slice {
		result = append(result, cat.(string))
	}
	return result
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
	meta.Provides = parseRelations(m["provides"], RoleProvider)
	meta.Requires = parseRelations(m["requires"], RoleRequirer)
	meta.Peers = parseRelations(m["peers"], RolePeer)
	meta.Format = int(m["format"].(int64))
	meta.Categories = parseCategories(m["categories"])
	if subordinate := m["subordinate"]; subordinate != nil {
		meta.Subordinate = subordinate.(bool)
	}
	if rev := m["revision"]; rev != nil {
		// Obsolete
		meta.OldRevision = int(m["revision"].(int64))
	}
	if series, ok := m["series"]; ok && series != nil {
		meta.Series = series.(string)
	}
	if err := meta.Check(); err != nil {
		return nil, err
	}
	return meta, nil
}

// Check checks that the metadata is well-formed.
func (meta Meta) Check() error {
	// Check for duplicate or forbidden relation names or interfaces.
	names := map[string]bool{}
	checkRelations := func(src map[string]Relation, role RelationRole) error {
		for name, rel := range src {
			if rel.Name != name {
				return fmt.Errorf("charm %q has mismatched relation name %q; expected %q", meta.Name, rel.Name, name)
			}
			if rel.Role != role {
				return fmt.Errorf("charm %q has mismatched role %q; expected %q", meta.Name, rel.Role, role)
			}
			// Container-scoped require relations on subordinates are allowed
			// to use the otherwise-reserved juju-* namespace.
			if !meta.Subordinate || role != RoleRequirer || rel.Scope != ScopeContainer {
				if reservedName(name) {
					return fmt.Errorf("charm %q using a reserved relation name: %q", meta.Name, name)
				}
			}
			if role != RoleRequirer {
				if reservedName(rel.Interface) {
					return fmt.Errorf("charm %q relation %q using a reserved interface: %q", meta.Name, name, rel.Interface)
				}
			}
			if names[name] {
				return fmt.Errorf("charm %q using a duplicated relation name: %q", meta.Name, name)
			}
			names[name] = true
		}
		return nil
	}
	if err := checkRelations(meta.Provides, RoleProvider); err != nil {
		return err
	}
	if err := checkRelations(meta.Requires, RoleRequirer); err != nil {
		return err
	}
	if err := checkRelations(meta.Peers, RolePeer); err != nil {
		return err
	}

	// Subordinate charms must have at least one relation that
	// has container scope, otherwise they can't relate to the
	// principal.
	if meta.Subordinate {
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
			return fmt.Errorf("subordinate charm %q lacks \"requires\" relation with container scope", meta.Name)
		}
	}

	if meta.Series != "" {
		if !IsValidSeries(meta.Series) {
			return fmt.Errorf("charm %q declares invalid series: %q", meta.Name, meta.Series)
		}
	}

	return nil
}

func reservedName(name string) bool {
	return name == "juju" || strings.HasPrefix(name, "juju-")
}

func parseRelations(relations interface{}, role RelationRole) map[string]Relation {
	if relations == nil {
		return nil
	}
	result := make(map[string]Relation)
	for name, rel := range relations.(map[string]interface{}) {
		relMap := rel.(map[string]interface{})
		relation := Relation{
			Name:      name,
			Role:      role,
			Interface: relMap["interface"].(string),
			Optional:  relMap["optional"].(bool),
		}
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
		"categories":  schema.List(schema.String()),
		"series":      schema.String(),
	},
	schema.Defaults{
		"provides":    schema.Omit,
		"requires":    schema.Omit,
		"peers":       schema.Omit,
		"revision":    schema.Omit,
		"format":      1,
		"subordinate": schema.Omit,
		"categories":  schema.Omit,
		"series":      schema.Omit,
	},
)
