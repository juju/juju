// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package jsonschema adds juju-specific metadata to jsonschema.
package jsonschema

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"regexp"

	// Schema *is* the actual package name, this just makes it clearer.
	schema "github.com/lestrrat/go-jsschema"
	"github.com/lestrrat/go-jsschema/validator"
	"gopkg.in/yaml.v2"

	"github.com/juju/utils"
)

// FromJSON returns a schema created from the json value in r.
func FromJSON(r io.Reader) (*Schema, error) {
	s := &Schema{}
	if err := json.NewDecoder(r).Decode(s); err != nil {
		return nil, err
	}
	return s, nil
}

// FromYAML returns a schema created from the yaml value in r.
func FromYAML(r io.Reader) (*Schema, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var v map[interface{}]interface{}
	if err := yaml.Unmarshal(b, &v); err != nil {
		return nil, err
	}

	// yaml serialization outputs map[interface{}]interface{} instead of
	// map[string]interface{} for some reason, so we have to fix that.
	val, err := utils.ConformYAML(v)
	if err != nil {
		return nil, err
	}
	return FromGo(val)
}

// FromGo extracts the jsonschema represented by v.
func FromGo(v interface{}) (*Schema, error) {
	// We have to run this through marshal/unmarshal, since schema.Extract only
	// works for it's special format, which doesn't match up with a more
	// expected format for json-in-go.
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	s := &Schema{}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	return s, nil
}

// Schema represents a fully defined jsonschema plus some metadata for the
// purposes of UX generation.  See http://jsonschema.org for details.
type Schema struct {
	ID          string             `json:"id,omitempty"`
	Title       string             `json:"title,omitempty"`
	Description string             `json:"description,omitempty"`
	Default     interface{}        `json:"default,omitempty"`
	Type        []Type             `json:"type,omitempty"`
	SchemaRef   string             `json:"$schema,omitempty"`
	Definitions map[string]*Schema `json:"definitions,omitempty"`
	Reference   string             `json:"$ref,omitempty"`
	Format      Format             `json:"format,omitempty"`

	// NumericValidations
	MultipleOf       *float64 `json:"multipleOf,omitempty"`
	Minimum          *float64 `json:"minimum,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	ExclusiveMinimum *bool    `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum *bool    `json:"exclusiveMaximum,omitempty"`

	// StringValidation
	MaxLength *int           `json:"maxLength,omitempty"`
	MinLength *int           `json:"minLength,omitempty"`
	Pattern   *regexp.Regexp `json:"pattern,omitempty"`

	// ArrayValidations
	AdditionalItems *Schema   `json:"additionalItems,omitempty"`
	Items           *ItemSpec `json:"items,omitempty"`
	MinItems        *int      `json:"minItems,omitempty"`
	MaxItems        *int      `json:"maxItems,omitempty"`
	UniqueItems     *bool     `json:"uniqueItems,omitempty"`

	// ObjectValidations
	MaxProperties        *int                       `json:"maxProperties,omitempty"`
	MinProperties        *int                       `json:"minProperties,omitempty"`
	Required             []string                   `json:"required,omitempty"`
	Dependencies         DependencyMap              `json:"dependencies,omitempty"`
	Properties           map[string]*Schema         `json:"properties,omitempty"`
	AdditionalProperties *Schema                    `json:"additionalProperties,omitempty"`
	PatternProperties    map[*regexp.Regexp]*Schema `json:"patternProperties,omitempty"`

	Enum  []interface{} `json:"enum,omitempty"`
	AllOf []*Schema     `json:"allOf,omitempty"`
	AnyOf []*Schema     `json:"anyOf,omitempty"`
	OneOf []*Schema     `json:"oneOf,omitempty"`
	Not   *Schema       `json:"not,omitempty"`

	// Juju-specific properties.  If you add properties to this list, you0
	// *must* add conversion logic in toExtras.

	// Immutable specifies whether the attribute cannot
	// be changed once set.
	Immutable bool `json:"immutable,omitempty"`

	// Secret specifies whether the attribute should be
	// considered secret.
	Secret bool `json:"secret,omitempty"`

	// EnvVars holds environment variables that will be used to obtain the
	// default value if it isn't specified, they are checked from highest to
	// lowest priority.
	EnvVars []string `json:"env-vars,omitempty"`

	// Example holds an example value for the attribute
	// that can be used to produce a plausible-looking
	// entry for the attribute without necessarily using
	// it as a default value.
	//
	// TODO if the example holds some special values, use
	// it as a template to generate initial random values
	// (for example for admin-password) ?
	Example interface{} `json:"example,omitempty"`

	// Order is the order in which properties should be requested of the user
	// during an interactive session.
	Order []string `json:"order,omitempty"`

	// Singular contains the singular version of the human-friendly name of this
	// property.
	Singular string `json:"singular,omitempty"`

	// Plural contains the plural version of the human-friendly name of this
	// property.
	Plural string `json:"plural,omitempty"`

	// PromptDefault contains the default value the user can accept during
	// interactive add-cloud.
	PromptDefault interface{} `json:"prompt-default,omitempty"`

	// PathFor should contain the name of another property in this schema. If a
	// value for that property does not exist, and this property's value is set,
	// the value from this property is interpreted as a filepath, and the
	// contents of that filepath are used as the value of the given property.
	// This is useful for properties with large values, such as encryption keys.
	PathFor string `json:"path-for,omitempty"`
}

// toExtras converts the juju-specific metadata fields on Schema into values to
// be put into the Extras map on jsschema.Schema. 	The keys here *must* be kept
// in sync with the json keys listed in the Schema struct.
func toExtras(s *Schema) map[string]interface{} {
	extras := make(map[string]interface{})
	if s.Immutable {
		extras["immutable"] = s.Immutable
	}
	if s.Secret {
		extras["secret"] = s.Secret
	}
	if len(s.EnvVars) > 0 {
		extras["env-vars"] = s.EnvVars
	}
	if s.Example != nil {
		extras["example"] = s.Example
	}
	if len(s.Order) > 0 {
		extras["order"] = s.Order
	}
	if s.Singular != "" {
		extras["singular"] = s.Singular
	}
	if s.PromptDefault != nil {
		extras["prompt-default"] = s.PromptDefault
	}
	if s.Plural != "" {
		extras["plural"] = s.Plural
	}

	if s.PathFor != "" {
		extras["path-for"] = s.PathFor
	}
	return extras
}

// MarshalJSON implements the json.Marshaler.
func (s *Schema) MarshalJSON() ([]byte, error) {
	internal, err := toInternal(s, make(map[*Schema]*schema.Schema))
	if err != nil {
		return nil, err
	}
	return internal.MarshalJSON()
}

// UnmarshalJSON implements the json.Marshaler.
func (s *Schema) UnmarshalJSON(data []byte) error {
	internal := schema.New()
	if err := internal.UnmarshalJSON(data); err != nil {
		return err
	}
	ext, err := fromInternal(internal, make(map[*schema.Schema]*Schema))
	if err != nil {
		return err
	}
	*s = *ext
	return nil
}

// Validate validates the given value based on the jsonschema in s.  Values are
// expected to be map[string]interface{} for object types, strings for string
// type, int for integer type, float64 or integer for number type, or an array
// of one of the previous types.
func (s *Schema) Validate(x interface{}) error {
	internal, err := toInternal(s, make(map[*Schema]*schema.Schema))
	if err != nil {
		return err
	}
	v := validator.New(internal)
	return v.Validate(x)
}

// InsertDefaults takes a target map and inserts any missing default values
// as specified in the properties map, according to JSON-Schema.
func (s *Schema) InsertDefaults(into map[string]interface{}) {
	if into == nil {
		return
	}
	for property, schema := range s.Properties {
		if v, ok := into[property]; ok {
			// If there is already a value in the target map for this key, don't
			// overwrite it.
			// If it's a map, set defaults on it.
			if innerMap, ok := v.(map[string]interface{}); ok {
				schema.InsertDefaults(innerMap)
			}
			continue
		}

		if schema.Default != nil {
			// Most basic case: we have a default value. Done for this key.
			into[property] = schema.Default
			continue
		}

		if len(schema.Properties) > 0 {
			m := make(map[string]interface{})
			schema.InsertDefaults(m)
			if len(m) > 0 {
				into[property] = m
			}
		}
	}
}

// Type defines the standard jsonschema value types.IntegerType
type Type int

// Standard jsonschema types.
const (
	UnspecifiedType Type = iota
	NullType
	IntegerType
	StringType
	ObjectType
	ArrayType
	BooleanType
	NumberType
)

// Format defines well-known jsonschema formats for strings.
type Format string

// Standard jsonschema formats.
const (
	FormatDateTime Format = "date-time"
	FormatEmail    Format = "email"
	FormatHostname Format = "hostname"
	FormatIPv4     Format = "ipv4"
	FormatIPv6     Format = "ipv6"
	FormatURI      Format = "uri"
)

// DependencyMap contains the dependencies defined within this schema.
// for a given dependency name, you can have either a schema or a
// list of property names
type DependencyMap struct {
	Names   map[string][]string
	Schemas map[string]*Schema
}

// ItemSpec contains the schemas for any items in an array type.
type ItemSpec struct {
	TupleMode bool
	Schemas   []*Schema
}

// Float is a helper function for use in struct literals.
func Float(f float64) *float64 {
	return &f
}

// Int is a helper function for use in struct literals.
func Int(i int) *int {
	return &i
}

// Bool is a helper function for use in struct literals.
func Bool(b bool) *bool {
	return &b
}

func fromInternalSchemaList(in schema.SchemaList, cache map[*schema.Schema]*Schema) ([]*Schema, error) {
	if in == nil {
		return nil, nil
	}
	list := make([]*Schema, len(in))
	for i, in := range in {
		out, err := fromInternal(in, cache)
		if err != nil {
			return nil, err
		}
		list[i] = out
	}
	return list, nil
}

func fromInternalSchemaMap(in map[string]*schema.Schema, cache map[*schema.Schema]*Schema) (map[string]*Schema, error) {
	if in == nil {
		return nil, nil
	}
	m := make(map[string]*Schema)
	for k, in := range in {
		out, err := fromInternal(in, cache)
		if err != nil {
			return nil, err
		}
		m[k] = out
	}
	return m, nil
}

func fromInternal(in *schema.Schema, cache map[*schema.Schema]*Schema) (*Schema, error) {
	if in == nil {
		return nil, nil
	}
	if out, ok := cache[in]; ok {
		return out, nil
	}
	out := &Schema{}
	cache[in] = out

	var definitions map[string]*Schema
	if len(in.Definitions) > 0 {
		definitions = make(map[string]*Schema)
		for k, in := range in.Definitions {
			out, err := fromInternal(in, cache)
			if err != nil {
				return nil, err
			}
			definitions[k] = out
		}
	}

	var additionalItems *Schema
	if in.AdditionalItems != nil {
		out, err := fromInternal(in.AdditionalItems.Schema, cache)
		if err != nil {
			return nil, err
		}
		additionalItems = out
	}

	var items *ItemSpec
	if in.Items != nil {
		schemas, err := fromInternalSchemaList(in.Items.Schemas, cache)
		if err != nil {
			return nil, err
		}
		items = &ItemSpec{
			TupleMode: in.Items.TupleMode,
			Schemas:   schemas,
		}
	}

	schemas, err := fromInternalSchemaMap(in.Dependencies.Schemas, cache)
	if err != nil {
		return nil, err
	}
	dependencies := DependencyMap{
		Names:   in.Dependencies.Names,
		Schemas: schemas,
	}

	properties, err := fromInternalSchemaMap(in.Properties, cache)
	if err != nil {
		return nil, err
	}

	var additionalProperties *Schema
	if in.AdditionalProperties != nil {
		out, err := fromInternal(in.AdditionalProperties.Schema, cache)
		if err != nil {
			return nil, err
		}
		additionalProperties = out
	}

	var patternProperties map[*regexp.Regexp]*Schema
	if in.PatternProperties != nil {
		patternProperties = make(map[*regexp.Regexp]*Schema)
		for re, in := range in.PatternProperties {
			out, err := fromInternal(in, cache)
			if err != nil {
				return nil, err
			}
			patternProperties[re] = out
		}
	}

	allOf, err := fromInternalSchemaList(in.AllOf, cache)
	if err != nil {
		return nil, err
	}

	anyOf, err := fromInternalSchemaList(in.AnyOf, cache)
	if err != nil {
		return nil, err
	}

	oneOf, err := fromInternalSchemaList(in.OneOf, cache)
	if err != nil {
		return nil, err
	}

	not, err := fromInternal(in.Not, cache)
	if err != nil {
		return nil, err
	}

	*out = Schema{
		ID:          in.ID,
		Title:       in.Title,
		Description: in.Description,
		Default:     in.Default,
		Type:        fromPrimitiveTypes(in.Type),
		SchemaRef:   in.SchemaRef,
		Definitions: definitions,
		Reference:   in.Reference,
		Format:      Format(in.Format),

		MultipleOf:       toFloat(in.MultipleOf),
		Minimum:          toFloat(in.Minimum),
		Maximum:          toFloat(in.Maximum),
		ExclusiveMinimum: toBool(in.ExclusiveMinimum),
		ExclusiveMaximum: toBool(in.ExclusiveMaximum),

		MaxLength: toInt(in.MaxLength),
		MinLength: toInt(in.MinLength),
		Pattern:   in.Pattern,

		AdditionalItems: additionalItems,
		Items:           items,
		MinItems:        toInt(in.MinItems),
		MaxItems:        toInt(in.MaxItems),
		UniqueItems:     toBool(in.UniqueItems),

		MaxProperties:        toInt(in.MaxProperties),
		MinProperties:        toInt(in.MinProperties),
		Required:             in.Required,
		Dependencies:         dependencies,
		Properties:           properties,
		AdditionalProperties: additionalProperties,
		PatternProperties:    patternProperties,

		Enum:  in.Enum,
		AllOf: allOf,
		AnyOf: anyOf,
		OneOf: oneOf,
		Not:   not,
	}

	// Extract the Juju-specific properties that exist in Extras.  Yes, we're
	// round-tripping through json, yes this is kinda slow, but it's really only
	// a few tens of milliseconds, and this isn't ever done in a tight loop.  By
	// doing it this way, we don't have to hand-write the marshalling code for
	// all our custom propreties, so the struct definition is the single source
	// of truth.

	b, err := json.Marshal(in.Extras)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, noCustomUnmarshal(out)); err != nil {
		return nil, err
	}

	return out, nil
}

// noCustomUnmarshal strips off the custom json Unmarshal function so we can use
// json.Unmarshal to populate our juju metadata fields from schema.Extras.
type noCustomUnmarshal *Schema

func fromPrimitiveTypes(p schema.PrimitiveTypes) []Type {
	if p == nil {
		return nil
	}
	t := make([]Type, len(p))
	for i, typ := range p {
		t[i] = Type(typ)
	}
	return t
}

func toPrimitiveTypes(t []Type) schema.PrimitiveTypes {
	if t == nil {
		return nil
	}
	p := make(schema.PrimitiveTypes, len(t))
	for i, typ := range t {
		p[i] = schema.PrimitiveType(typ)
	}
	return p
}

func toFloat(n schema.Number) *float64 {
	if !n.Initialized {
		return nil
	}
	return &n.Val
}

func fromFloat(n *float64) schema.Number {
	if n == nil {
		return schema.Number{}
	}
	return schema.Number{Initialized: true, Val: *n}
}

func toInt(n schema.Integer) *int {
	if !n.Initialized {
		return nil
	}
	return &n.Val
}

func fromInt(n *int) schema.Integer {
	if n == nil {
		return schema.Integer{}
	}
	return schema.Integer{Initialized: true, Val: *n}
}

func toBool(b schema.Bool) *bool {
	if !b.Initialized {
		return nil
	}
	return &b.Val
}

func fromBool(b *bool) schema.Bool {
	if b == nil {
		return schema.Bool{}
	}
	return schema.Bool{Initialized: true, Val: *b}
}

func toInternalSchemaList(in []*Schema, cache map[*Schema]*schema.Schema) (schema.SchemaList, error) {
	list := make(schema.SchemaList, len(in))
	for i, in := range in {
		out, err := toInternal(in, cache)
		if err != nil {
			return nil, err
		}
		list[i] = out
	}
	return list, nil
}

func toInternalSchemaMap(in map[string]*Schema, cache map[*Schema]*schema.Schema) (map[string]*schema.Schema, error) {
	m := make(map[string]*schema.Schema)
	for k, in := range in {
		out, err := toInternal(in, cache)
		if err != nil {
			return nil, err
		}
		m[k] = out
	}
	return m, nil
}

func toInternal(in *Schema, cache map[*Schema]*schema.Schema) (*schema.Schema, error) {
	if in == nil {
		return nil, nil
	}
	if out, ok := cache[in]; ok {
		return out, nil
	}
	out := schema.New()
	cache[in] = out

	definitions := make(map[string]*schema.Schema)
	for k, in := range in.Definitions {
		out, err := toInternal(in, cache)
		if err != nil {
			return nil, err
		}
		definitions[k] = out
	}

	var additionalItems *schema.AdditionalItems
	if in.AdditionalItems != nil {
		out, err := toInternal(in.AdditionalItems, cache)
		if err != nil {
			return nil, err
		}
		additionalItems = &schema.AdditionalItems{Schema: out}
	}

	var items *schema.ItemSpec
	if in.Items != nil {
		schemas, err := toInternalSchemaList(in.Items.Schemas, cache)
		if err != nil {
			return nil, err
		}
		items = &schema.ItemSpec{
			TupleMode: in.Items.TupleMode,
			Schemas:   schemas,
		}
	}

	schemas, err := toInternalSchemaMap(in.Dependencies.Schemas, cache)
	if err != nil {
		return nil, err
	}
	dependencies := schema.DependencyMap{
		Names:   in.Dependencies.Names,
		Schemas: schemas,
	}

	properties, err := toInternalSchemaMap(in.Properties, cache)
	if err != nil {
		return nil, err
	}

	var additionalProperties *schema.AdditionalProperties
	if in.AdditionalProperties != nil {
		out, err := toInternal(in.AdditionalProperties, cache)
		if err != nil {
			return nil, err
		}
		additionalProperties = &schema.AdditionalProperties{Schema: out}
	}

	var patternProperties map[*regexp.Regexp]*schema.Schema
	if in.PatternProperties != nil {
		patternProperties = make(map[*regexp.Regexp]*schema.Schema)
		for re, in := range in.PatternProperties {
			out, err := toInternal(in, cache)
			if err != nil {
				return nil, err
			}
			patternProperties[re] = out
		}
	}

	allOf, err := toInternalSchemaList(in.AllOf, cache)
	if err != nil {
		return nil, err
	}

	anyOf, err := toInternalSchemaList(in.AnyOf, cache)
	if err != nil {
		return nil, err
	}

	oneOf, err := toInternalSchemaList(in.OneOf, cache)
	if err != nil {
		return nil, err
	}

	not, err := toInternal(in.Not, cache)
	if err != nil {
		return nil, err
	}

	out.ID = in.ID
	out.Title = in.Title
	out.Description = in.Description
	out.Default = in.Default
	out.Type = toPrimitiveTypes(in.Type)
	out.SchemaRef = in.SchemaRef
	out.Definitions = definitions
	out.Reference = in.Reference
	out.Format = schema.Format(in.Format)

	out.MultipleOf = fromFloat(in.MultipleOf)
	out.Minimum = fromFloat(in.Minimum)
	out.Maximum = fromFloat(in.Maximum)
	out.ExclusiveMinimum = fromBool(in.ExclusiveMinimum)
	out.ExclusiveMaximum = fromBool(in.ExclusiveMaximum)

	out.MaxLength = fromInt(in.MaxLength)
	out.MinLength = fromInt(in.MinLength)
	out.Pattern = in.Pattern

	out.AdditionalItems = additionalItems
	out.Items = items
	out.MinItems = fromInt(in.MinItems)
	out.MaxItems = fromInt(in.MaxItems)
	out.UniqueItems = fromBool(in.UniqueItems)

	out.MaxProperties = fromInt(in.MaxProperties)
	out.MinProperties = fromInt(in.MinProperties)
	out.Required = in.Required
	out.Dependencies = dependencies
	out.Properties = properties
	out.AdditionalProperties = additionalProperties
	out.PatternProperties = patternProperties

	out.Enum = in.Enum
	out.AllOf = allOf
	out.AnyOf = anyOf
	out.OneOf = oneOf
	out.Not = not

	extras := toExtras(in)
	if len(extras) > 0 {
		out.Extras = extras
	}

	return out, nil
}
