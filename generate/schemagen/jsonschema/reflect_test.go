// Copyright (C) 2014 Alec Thomas
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
// of the Software, and to permit persons to whom the Software is furnished to do
// so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package jsonschema

import (
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type GrandfatherType struct {
	FamilyName string `json:"family_name"`
}

type SomeBaseType struct {
	SomeBaseProperty        int             `json:"some_base_property"`
	somePrivateBaseProperty string          `json:"i_am_private"`
	SomeIgnoredBaseProperty string          `json:"-"`
	Grandfather             GrandfatherType `json:"grand"`

	SomeUntaggedBaseProperty           bool
	someUnexportedUntaggedBaseProperty bool
}

type nonExported struct {
	PublicNonExported  int
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

type JsonSchemaSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&JsonSchemaSuite{})

// TestSchemaGeneration checks if schema generated correctly:
// - fields marked with "-" are ignored
// - non-exported fields are ignored
// - anonymous fields are expanded
func (s *JsonSchemaSuite) TestSchemaGeneration(c *gc.C) {
	reflection := Reflect(&TestUser{})

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

	props := reflection.Definitions["TestUser"].Properties
	for defKey, prop := range props {
		typeOrRef, ok := expectedProperties[defKey]
		if !ok {
			c.Fatalf("unexpected property '%s'", defKey)
		}
		if prop.Type != "" && prop.Type != typeOrRef {
			c.Fatalf("expected property type '%s', got '%s' for property '%s'", typeOrRef, prop.Type, defKey)
		} else if prop.Ref != "" && prop.Ref != typeOrRef {
			c.Fatalf("expected reference to '%s', got '%s' for property '%s'", typeOrRef, prop.Ref, defKey)
		}
	}

	for defKey := range expectedProperties {
		if _, ok := props[defKey]; !ok {
			c.Fatalf("expected property missing '%s'", defKey)
		}
	}
}
