// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package format

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"text/template"

	"github.com/juju/juju/generate/schemagen/gen"
	"github.com/juju/juju/generate/schemagen/jsonschema-gen"
)

func Format(format string, schema []gen.FacadeSchema) ([]byte, error) {
	switch format {
	case "json":
		return json.MarshalIndent(schema, "", "    ")
	case "grpc":
		return grpcFormatter(schema)
	default:
		return nil, fmt.Errorf("unknown format %q", format)
	}
}

const (
	emptyRequest  = "google.protobuf.Empty"
	emptyResponse = "google.protobuf.Empty"
)

func grpcFormatter(schema []gen.FacadeSchema) ([]byte, error) {
	t := template.New("service")
	t = t.Funcs(template.FuncMap{
		"comment":   comment,
		"lowercase": strings.ToLower,
		"optional":  optional,
		"repeated":  repeated,
		"field":     fieldName,
		"type":      parseType,
	})
	tmpl, err := t.Parse(serviceTemplate)
	if err != nil {
		return nil, err
	}

	witnessedDefinitions := make(map[string]struct{})

	services := make([]Service, 0)
	for _, s := range schema {
		methods := make([]Method, 0)

		for name, prop := range s.Schema.Properties {
			if prop.Type != "object" {
				continue
			}

			request := nameOrDefault(prop.Properties, "Params", emptyRequest)
			response := nameOrDefault(prop.Properties, "Result", emptyResponse)

			methods = append(methods, Method{
				MethodName:   name,
				RequestName:  request,
				ResponseName: response,
				Description:  prop.Description,
			})
		}

		definitions := make([]Definition, 0)
		for name, props := range s.Schema.Definitions {
			if props.Type != "object" {
				continue
			}

			if _, ok := witnessedDefinitions[name]; ok {
				continue
			}

			properties := make([]Property, 0)
			for propName, prop := range props.Properties {
				optional := !contains(props.Required, propName)

				typeName, repeated := parseTypeName(prop)

				properties = append(properties, Property{
					Name:     propName,
					Type:     typeName,
					Optional: optional,
					Repeated: repeated,
				})
			}

			sort.Slice(properties, func(i, j int) bool {
				return properties[i].Name < properties[j].Name
			})

			for i := range properties {
				properties[i].Index = i + 1
			}

			definitions = append(definitions, Definition{
				DefinitionName: name,
				Properties:     properties,
			})
			witnessedDefinitions[name] = struct{}{}
		}

		services = append(services, Service{
			ServiceName: s.Name,
			Methods:     methods,
			Definitions: definitions,
		})
	}

	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, services); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func nameOrDefault(m map[string]*jsonschema.Type, key, def string) string {
	if len(m) == 0 {
		return def
	}

	value, ok := m[key]
	if !ok || value.Ref == "" {
		return def
	}

	return defName(value.Ref)
}

func defName(s string) string {
	// Strip the leading "#/definitions/" from the ref.
	parts := strings.Split(s, "/")
	return parts[len(parts)-1]
}

func comment(s string, ident int) string {
	scanner := bufio.NewScanner(bytes.NewBufferString(s))

	var multiline bool
	var builder strings.Builder
	for scanner.Scan() {
		pad := ""
		if multiline {
			pad = strings.Repeat("\n\t", ident)
		}
		fmt.Fprintf(&builder, "%s// %s", pad, scanner.Text())
		multiline = true
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	return builder.String()
}

func optional(p Property) string {
	if p.Optional {
		return "optional "
	}
	return ""
}

func repeated(p Property) string {
	if p.Repeated {
		return "repeated "
	}
	return ""
}

func contains(list []string, item string) bool {
	for _, i := range list {
		if i == item {
			return true
		}
	}
	return false
}

func fieldName(s string) string {
	return strings.ReplaceAll(s, "-", "_")
}

func parseType(s string) string {
	switch s {
	case "integer":
		return "int"
	case "boolean":
		return "bool"
	case "":
		return "google.protobuf.Empty"
	default:
		return s
	}
}

func parseTypeName(prop *jsonschema.Type) (string, bool) {
	if prop.Type == "array" {
		if prop.Ref != "" {
			return defName(prop.Ref), true
		}
		if prop.Items.Type != "" {
			return prop.Items.Type, true
		}
		if prop.Items.Ref != "" {
			return defName(prop.Items.Ref), true
		}
		return fmt.Sprintf("!!%s", prop.Type), true
	}

	if prop.Ref != "" {
		return defName(prop.Ref), false
	}

	return prop.Type, false
}

const serviceTemplate = `
syntax = "proto3";

package juju.api.v1;

import "google/api/annotations.proto";
import "google/protobuf/empty.proto";

{{ range $srv := . }}
service {{ $srv.ServiceName }} {
	{{ range $meth := $srv.Methods }}
	{{ comment $meth.Description 1 }}
	rpc {{ $meth.MethodName }}({{ $meth.RequestName }}) returns ({{ $meth.ResponseName }}) {
		option (google.api.http) = {
			get: "/api/{{ $srv.ServiceName | lowercase }}/{{ $meth.MethodName | lowercase }}"
		};
	}
	{{ end }}
}

{{ range $def := $srv.Definitions }}
message {{ $def.DefinitionName }} {
	{{ range $prop := $def.Properties }}
	{{ optional $prop }}{{ repeated $prop }}{{ $prop.Type | type }} {{ $prop.Name | field }} = {{ $prop.Index }};
	{{ end }}
}
{{ end }}
{{ end }}
`

type Service struct {
	ServiceName string
	Methods     []Method
	Definitions []Definition
}

type Method struct {
	MethodName   string
	RequestName  string
	ResponseName string
	Description  string
}

type Definition struct {
	DefinitionName string
	Properties     []Property
}

type Property struct {
	Name     string
	Type     string
	Index    int
	Optional bool
	Repeated bool
}
