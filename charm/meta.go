package charm

import (
	"io"
	"io/ioutil"
	"launchpad.net/juju/go/schema"
	"launchpad.net/goyaml"
	"os"
)

// Relation represents a single relation defined in the charm
// metadata.yaml file.
type Relation struct {
	Interface string
	Optional  bool
	Limit     int
}

// Meta represents all the known content that may be defined
// within a charm's metadata.yaml file.
type Meta struct {
	Name        string
	Revision    int
	Summary     string
	Description string
	Provides    map[string]Relation
	Requires    map[string]Relation
	Peers       map[string]Relation
}

// ReadMeta reads the content of a metadata.yaml file and returns
// its representation.
func ReadMeta(r io.Reader) (meta *Meta, err os.Error) {
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
		return nil, os.NewError("metadata: " + err.String())
	}
	m := v.(schema.MapType)
	meta = &Meta{}
	meta.Name = m["name"].(string)
	// Schema decodes as int64, but the int range should be good
	// enough for revisions.
	meta.Revision = int(m["revision"].(int64))
	meta.Summary = m["summary"].(string)
	meta.Description = m["description"].(string)
	meta.Provides = parseRelations(m["provides"])
	meta.Requires = parseRelations(m["requires"])
	meta.Peers = parseRelations(m["peers"])
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

func (c ifaceExpC) Coerce(v interface{}, path []string) (newv interface{}, err os.Error) {
	s, err := stringC.Coerce(v, path)
	if err == nil {
		newv = schema.MapType{
			"interface": s,
			"limit":     c.limit,
			"optional":  false,
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
	return ifaceSchema.Coerce(m, path)
}

var ifaceSchema = schema.FieldMap(schema.Fields{
	"interface": schema.String(),
	"limit":     schema.OneOf(schema.Const(nil), schema.Int()),
	"optional":  schema.Bool(),
}, nil)

var charmSchema = schema.FieldMap(
	schema.Fields{
		"name":        schema.String(),
		"revision":    schema.Int(),
		"summary":     schema.String(),
		"description": schema.String(),
		"peers":       schema.Map(schema.String(), ifaceExpander(1)),
		"provides":    schema.Map(schema.String(), ifaceExpander(nil)),
		"requires":    schema.Map(schema.String(), ifaceExpander(1)),
	},
	schema.Optional{"provides", "requires", "peers"},
)
