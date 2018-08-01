// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package environschema

import (
	"bytes"
	"fmt"
	"go/doc"
	"io"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v2"
)

// SampleYAML writes YAML output to w, indented by indent spaces
// that holds the attributes in attrs with descriptions found
// in the given fields. An entry for any attribute in fields not
// in attrs will be generated but commented out.
func SampleYAML(w io.Writer, indent int, attrs map[string]interface{}, fields Fields) error {
	indentStr := strings.Repeat(" ", indent)
	orderedFields := make(fieldsByGroup, 0, len(fields))
	for name, f := range fields {
		orderedFields = append(orderedFields, attrWithName{
			name: name,
			Attr: f,
		})
	}
	sort.Sort(orderedFields)
	for i, f := range orderedFields {
		if i > 0 {
			w.Write(nl)
		}
		writeSampleDescription(w, f.Attr, indentStr+"# ")
		val, ok := attrs[f.name]
		if ok {
			fmt.Fprintf(w, "%s:", f.name)
			indentVal(w, val, indentStr)
		} else {
			if f.Example != nil {
				val = f.Example
			} else {
				val = sampleValue(f.Type)
			}
			fmt.Fprintf(w, "# %s:", f.name)
			indentVal(w, val, indentStr+"# ")
		}
	}
	return nil
}

const textWidth = 80

var (
	space = []byte(" ")
	nl    = []byte("\n")
)

// writeSampleDescription writes the given attribute to w
// prefixed by the given indentation string.
func writeSampleDescription(w io.Writer, f Attr, indent string) {
	previousText := false

	// section marks the start of a new section of the comment;
	// sections are separated with empty lines.
	section := func() {
		if previousText {
			fmt.Fprintf(w, "%s\n", strings.TrimRightFunc(indent, unicode.IsSpace))
		}
		previousText = true
	}

	descr := strings.TrimSpace(f.Description)
	if descr != "" {
		section()
		doc.ToText(w, descr, indent, "    ", textWidth-len(indent))
	}
	vars := make([]string, 0, len(f.EnvVars)+1)
	if f.EnvVar != "" {
		vars = append(vars, "$"+f.EnvVar)
	}
	for _, v := range f.EnvVars {
		vars = append(vars, "$"+v)
	}
	if len(vars) > 0 {
		section()
		fmt.Fprintf(w, "%sDefault value taken from %s.\n", indent, wordyList(vars))
	}
	attrText := ""
	switch {
	case f.Secret && f.Immutable:
		attrText = "immutable and considered secret"
	case f.Secret:
		attrText = "considered secret"
	case f.Immutable:
		attrText = "immutable"
	}
	if attrText != "" {
		section()
		fmt.Fprintf(w, "%sThis attribute is %s.\n", indent, attrText)
	}
	section()
}

// emptyLine writes an empty line prefixed with the given
// indent, ensuring that it doesn't have any trailing white space.
func emptyLine(w io.Writer, indent string) {
	fmt.Fprintf(w, "%s\n", strings.TrimRightFunc(indent, unicode.IsSpace))
}

// wordyList formats the given slice in the form "x, y or z".
func wordyList(words []string) string {
	if len(words) == 0 {
		return ""
	}
	if len(words) == 1 {
		return words[0]
	}
	return strings.Join(words[0:len(words)-1], ", ") + " or " + words[len(words)-1]
}

var groupPriority = map[Group]int{
	ProviderGroup: 3,
	AccountGroup:  2,
	EnvironGroup:  1,
}

type attrWithName struct {
	name string
	Attr
}

type fieldsByGroup []attrWithName

func (f fieldsByGroup) Len() int {
	return len(f)
}

func (f fieldsByGroup) Swap(i0, i1 int) {
	f[i0], f[i1] = f[i1], f[i0]
}

func (f fieldsByGroup) Less(i0, i1 int) bool {
	f0, f1 := &f[i0], &f[i1]
	pri0, pri1 := groupPriority[f0.Group], groupPriority[f1.Group]
	if pri0 != pri1 {
		return pri0 > pri1
	}
	return f0.name < f1.name
}

// indentVal writes the given YAML-formatted value x to w and prefixing
// the second and subsequent lines with the given ident.
func indentVal(w io.Writer, x interface{}, indentStr string) {
	data, err := yaml.Marshal(x)
	if err != nil {
		panic(fmt.Errorf("cannot marshal YAML", err))
	}
	if len(data) == 0 {
		panic("YAML cannot marshal to empty string")
	}
	indent := []byte(indentStr + "  ")
	if canUseSameLine(x) {
		w.Write(space)
	} else {
		w.Write(nl)
		w.Write(indent)
	}
	data = bytes.TrimSuffix(data, nl)
	lines := bytes.Split(data, nl)
	for i, line := range lines {
		if i > 0 {
			w.Write(indent)
		}
		w.Write(line)
		w.Write(nl)
	}
}

func canUseSameLine(x interface{}) bool {
	if x == nil {
		return true
	}
	v := reflect.ValueOf(x)
	switch v.Kind() {
	case reflect.Map:
		return v.Len() == 0
	case reflect.Slice:
		return v.Len() == 0
	}
	return true
}

func yamlQuote(s string) string {
	data, _ := yaml.Marshal(s)
	return strings.TrimSpace(string(data))
}

func sampleValue(t FieldType) interface{} {
	switch t {
	case Tstring:
		return ""
	case Tbool:
		return false
	case Tint:
		return 0
	case Tattrs:
		return map[string]string{
			"example": "value",
		}
	default:
		panic(fmt.Errorf("unknown schema type %q", t))
	}
}
