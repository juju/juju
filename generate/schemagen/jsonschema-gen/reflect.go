// Package jsonschema uses reflection to generate JSON Schemas from Go types [1].
//
// If json tags are present on struct fields, they will be used to infer
// property names and if a property is required (omitempty is present).
//
// [1] http://json-schema.org/latest/json-schema-validation.html
package jsonschema

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/juju/juju/internal/rpcreflect"
)

var (
	timeType = reflect.TypeOf(time.Time{})
	ipType   = reflect.TypeOf(net.IP{})
	urlType  = reflect.TypeOf(url.URL{})
	objType  = reflect.TypeOf(rpcreflect.ObjType{})
)

type Type struct {
	Type                 string           `json:"type,omitempty"`
	Format               string           `json:"format,omitempty"`
	Items                *Type            `json:"items,omitempty"`
	Properties           map[string]*Type `json:"properties,omitempty"`
	PatternProperties    map[string]*Type `json:"patternProperties,omitempty"`
	AdditionalProperties json.RawMessage  `json:"additionalProperties,omitempty"`
	Ref                  string           `json:"$ref,omitempty"`
	Required             []string         `json:"required,omitempty"`
	MaxLength            int              `json:"maxLength,omitempty"`
	MinLength            int              `json:"minLength,omitempty"`
	Pattern              string           `json:"pattern,omitempty"`
	Enum                 []interface{}    `json:"enum,omitempty"`
	Default              interface{}      `json:"default,omitempty"`
	Title                string           `json:"title,omitempty"`
	Description          string           `json:"description,omitempty"`
}

type Schema struct {
	*Type
	Definitions Definitions `json:"definitions,omitempty"`
}

// Reflect a Schema from a value.
func Reflect(v interface{}) *Schema {
	return ReflectFromType(reflect.TypeOf(v))
}

func ReflectFromType(t reflect.Type) *Schema {
	definitions := Definitions{}
	s := &Schema{
		Type:        reflectTypeToSchema(definitions, t),
		Definitions: definitions,
	}
	return s
}

// rpcreflect is itself a description so we provide additional handling here
func ReflectFromObjType(objtype *rpcreflect.ObjType) *Schema {
	definitions := Definitions{}
	s := &Schema{
		Definitions: definitions,
	}

	methodNames := objtype.MethodNames()
	props := make(map[string]*Type, len(methodNames))
	for _, n := range methodNames {
		method, err := objtype.Method(n)
		if err == nil {
			callmap := make(map[string]*Type)
			if method.Params != nil {
				callmap["Params"] = reflectTypeToSchema(definitions, method.Params)
			}
			if method.Result != nil {
				callmap["Result"] = reflectTypeToSchema(definitions, method.Result)
			}

			props[n] = &Type{
				Type:       "object",
				Properties: callmap,
			}
		}
	}

	s.Type = &Type{
		Type:       "object",
		Properties: props,
	}
	return s
}

type Definitions map[string]*Type

func reflectTypeToSchema(definitions Definitions, t reflect.Type) *Type {
	switch t.Kind() {
	case reflect.Struct:
		switch t {
		case timeType:
			return &Type{Type: "string", Format: "date-time"}

		case ipType:
			return &Type{Type: "string", Format: "ipv4"}

		case urlType:
			return &Type{Type: "string", Format: "uri"}

		default:
			if _, ok := definitions[t.Name()]; ok {
				return &Type{Ref: "#/definitions/" + t.Name()}
			}

			return reflectStruct(definitions, t)
		}

	case reflect.Map:
		rt := &Type{
			Type: "object",
			PatternProperties: map[string]*Type{
				".*": reflectTypeToSchema(definitions, t.Elem()),
			},
		}
		delete(rt.PatternProperties, "additionalProperties")
		return rt

	case reflect.Array, reflect.Slice:
		return &Type{
			Type:  "array",
			Items: reflectTypeToSchema(definitions, t.Elem()),
		}

	case reflect.Interface:
		return &Type{
			Type:                 "object",
			AdditionalProperties: []byte("true"),
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Type{Type: "integer"}

	case reflect.Float32, reflect.Float64:
		return &Type{Type: "number"}

	case reflect.Bool:
		return &Type{Type: "boolean"}

	case reflect.String:
		return &Type{Type: "string"}

	case reflect.Ptr:
		return reflectTypeToSchema(definitions, t.Elem())
	}

	fmt.Fprintf(os.Stderr, "Unsupported Type %s", t.String())
	return nil
}

func reflectStruct(definitions Definitions, t reflect.Type) *Type {
	st := &Type{
		Type:                 "object",
		Properties:           map[string]*Type{},
		AdditionalProperties: []byte("false"),
	}
	definitions[t.Name()] = st
	reflectStructFields(st, definitions, t)

	return &Type{Ref: "#/definitions/" + t.Name()}
}

func reflectStructFields(st *Type, definitions Definitions, t reflect.Type) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		name, required, embedded := reflectFieldName(f)
		if name == "" {
			continue
		}

		// anonymous and exported type should be processed recursively
		// current type should inherit properties of anonymous one
		if embedded {
			reflectStructFields(st, definitions, f.Type)
		}

		st.Properties[name] = reflectTypeToSchema(definitions, f.Type)
		if required {
			st.Required = append(st.Required, name)
		}
	}
}

func reflectFieldName(f reflect.StructField) (string, bool, bool) {
	if f.PkgPath != "" { // unexported field, ignore it
		return "", false, false
	}
	parts := strings.Split(f.Tag.Get("json"), ",")
	if parts[0] == "-" {
		return "", false, false
	}

	name := f.Name
	required := true

	if parts[0] != "" {
		name = parts[0]
	}

	if len(parts) > 1 && parts[1] == "omitempty" {
		required = false
	}
	embedded := len(parts) == 1 && parts[0] == "" && f.Type.Kind() == reflect.Struct && f.Name == f.Type.Name() && f.Anonymous
	return name, required, embedded
}
