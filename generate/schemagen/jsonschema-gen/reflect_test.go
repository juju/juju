package jsonschema

import (
	"testing"
	"time"
)

type GrandfatherType struct {
	FamilyName string `json:"family_name"`
}

type SomeBaseType struct {
	SomeBaseProperty int `json:"some_base_property"`
	//nolint:unused
	somePrivateBaseProperty string          `json:"i_am_private"`
	SomeIgnoredBaseProperty string          `json:"-"`
	Grandfather             GrandfatherType `json:"grand"`

	SomeUntaggedBaseProperty bool
	//nolint:unused
	someUnexportedUntaggedBaseProperty bool
}

type nonExported struct {
	PublicNonExported int
	//nolint:unused
	privateNonExported int
}

type TestUser struct {
	SomeBaseType
	nonExported

	ID        int                    `json:"id"`
	Name      string                 `json:"name"`
	Friends   []int                  `json:"friends,omitempty"`
	Tags      map[string]interface{} `json:"tags,omitempty"`
	BirthDate time.Time              `json:"birth_date,omitempty"`

	TestFlag       bool
	IgnoredCounter int `json:"-"`
}

// TestSchemaGeneration checks if schema generated correctly:
// - fields marked with "-" are ignored
// - non-exported fields are ignored
// - anonymous fields are expanded
func TestSchemaGeneration(t *testing.T) {
	s := Reflect(&TestUser{})

	expectedProperties := map[string]string{
		"id":                       "integer",
		"name":                     "string",
		"friends":                  "array",
		"tags":                     "object",
		"birth_date":               "string",
		"TestFlag":                 "boolean",
		"some_base_property":       "integer",
		"grand":                    "#/definitions/GrandfatherType",
		"SomeUntaggedBaseProperty": "boolean",
		"SomeBaseType":             "#/definitions/SomeBaseType",
	}

	props := s.Definitions["TestUser"].Properties
	for defKey, prop := range props {
		typeOrRef, ok := expectedProperties[defKey]
		if !ok {
			t.Fatalf("unexpected property '%s'", defKey)
		}
		if prop.Type != "" && prop.Type != typeOrRef {
			t.Fatalf("expected property type '%s', got '%s' for property '%s'", typeOrRef, prop.Type, defKey)
		} else if prop.Ref != "" && prop.Ref != typeOrRef {
			t.Fatalf("expected reference to '%s', got '%s' for property '%s'", typeOrRef, prop.Ref, defKey)
		}
	}

	for defKey := range expectedProperties {
		if _, ok := props[defKey]; !ok {
			t.Fatalf("expected property missing '%s'", defKey)
		}
	}
}
