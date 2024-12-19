// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package form_test

import (
	"bytes"
	"os"
	"strings"

	"github.com/juju/schema"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/juju/juju/internal/environschema"
	"github.com/juju/juju/internal/environschema/form"
)

type formSuite struct{}

var _ = gc.Suite(&formSuite{})

var _ form.Filler = form.IOFiller{}

var ioFillerTests = []struct {
	about        string
	form         form.Form
	filler       form.IOFiller
	environment  map[string]string
	expectIO     string
	expectResult map[string]interface{}
	expectError  string
}{{
	about: "no fields, no interaction",
	form: form.Form{
		Title: "something",
	},
	expectIO:     "",
	expectResult: map[string]interface{}{},
}, {
	about: "single field no default",
	form: form.Form{
		Fields: environschema.Fields{
			"A": environschema.Attr{
				Type:        environschema.Tstring,
				Description: "A description",
			},
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|A: »B
	`,
	expectResult: map[string]interface{}{
		"A": "B",
	},
}, {
	about: "single field with default",
	form: form.Form{
		Fields: environschema.Fields{
			"A": environschema.Attr{
				Type:        environschema.Tstring,
				Description: "A description",
				EnvVar:      "A",
			},
		},
	},
	environment: map[string]string{
		"A": "C",
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|A [C]: »B
	`,
	expectResult: map[string]interface{}{
		"A": "B",
	},
}, {
	about: "single field with default no input",
	form: form.Form{
		Fields: environschema.Fields{
			"A": environschema.Attr{
				Type:        environschema.Tstring,
				Description: "A description",
				EnvVar:      "A",
			},
		},
	},
	environment: map[string]string{
		"A": "C",
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|A [C]: »
	`,
	expectResult: map[string]interface{}{
		"A": "C",
	},
}, {
	about: "secret single field with default no input",
	form: form.Form{
		Fields: environschema.Fields{
			"A": environschema.Attr{
				Type:        environschema.Tstring,
				Description: "A description",
				EnvVar:      "A",
				Secret:      true,
			},
		},
	},
	environment: map[string]string{
		"A": "password",
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|A [********]: »
	`,
	expectResult: map[string]interface{}{
		"A": "password",
	},
}, {
	about: "windows line endings",
	form: form.Form{
		Fields: environschema.Fields{
			"A": environschema.Attr{
				Type:        environschema.Tstring,
				Description: "A description",
			},
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|A: »B` + "\r" + `
	`,
	expectResult: map[string]interface{}{
		"A": "B",
	},
}, {
	about: "with title",
	form: form.Form{
		Title: "Test Title",
		Fields: environschema.Fields{
			"A": environschema.Attr{
				Type:        environschema.Tstring,
				Description: "A description",
			},
		},
	},
	expectIO: `
	|Test Title
	|Press return to select a default value, or enter - to omit an entry.
	|A: »hello
	`,
	expectResult: map[string]interface{}{
		"A": "hello",
	},
}, {
	about: "title with prompts",
	form: form.Form{
		Title: "Test Title",
		Fields: environschema.Fields{
			"A": environschema.Attr{
				Type:        environschema.Tstring,
				Description: "A description",
			},
		},
	},
	expectIO: `
	|Test Title
	|Press return to select a default value, or enter - to omit an entry.
	|A: »B
	`,
	expectResult: map[string]interface{}{
		"A": "B",
	},
}, {
	about: "correct ordering",
	form: form.Form{
		Fields: environschema.Fields{
			"a1": environschema.Attr{
				Group:       "A",
				Description: "z a1 description",
				Type:        environschema.Tstring,
			},
			"c1": environschema.Attr{
				Group:       "A",
				Description: "c1 description",
				Type:        environschema.Tstring,
			},
			"b1": environschema.Attr{
				Group:       "A",
				Description: "b1 description",
				Type:        environschema.Tstring,
				Secret:      true,
			},
			"a2": environschema.Attr{
				Group:       "B",
				Description: "a2 description",
				Type:        environschema.Tstring,
			},
			"c2": environschema.Attr{
				Group:       "B",
				Description: "c2 description",
				Type:        environschema.Tstring,
			},
			"b2": environschema.Attr{
				Group:       "B",
				Description: "b2 description",
				Type:        environschema.Tstring,
				Secret:      true,
			},
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a1: »a1
	|c1: »c1
	|b1: »b1
	|a2: »a2
	|c2: »c2
	|b2: »b2
	`,
	expectResult: map[string]interface{}{
		"a1": "a1",
		"b1": "b1",
		"c1": "c1",
		"a2": "a2",
		"b2": "b2",
		"c2": "c2",
	},
}, {
	about: "string type",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tstring,
			},
			"b": environschema.Attr{
				Description: "b description",
				Type:        environschema.Tstring,
				Mandatory:   true,
			},
			"c": environschema.Attr{
				Description: "c description",
				Type:        environschema.Tstring,
			},
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a: »
	|b: »
	|c: »something
	`,
	expectResult: map[string]interface{}{
		"b": "",
		"c": "something",
	},
}, {
	about: "bool type",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tbool,
			},
			"b": environschema.Attr{
				Description: "b description",
				Type:        environschema.Tbool,
			},
			"c": environschema.Attr{
				Description: "c description",
				Type:        environschema.Tbool,
			},
			"d": environschema.Attr{
				Description: "d description",
				Type:        environschema.Tbool,
			},
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a: »true
	|b: »false
	|c: »1
	|d: »0
	`,
	expectResult: map[string]interface{}{
		"a": true,
		"b": false,
		"c": true,
		"d": false,
	},
}, {
	about: "int type",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tint,
			},
			"b": environschema.Attr{
				Description: "b description",
				Type:        environschema.Tint,
			},
			"c": environschema.Attr{
				Description: "c description",
				Type:        environschema.Tint,
			},
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a: »0
	|b: »-1000000
	|c: »1000000
	`,
	expectResult: map[string]interface{}{
		"a": 0,
		"b": -1000000,
		"c": 1000000,
	},
}, {
	about: "attrs type",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tattrs,
			},
			"b": environschema.Attr{
				Description: "b description",
				Type:        environschema.Tattrs,
			},
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a: »x=y z= foo=bar
	|b: »
	`,
	expectResult: map[string]interface{}{
		"a": map[string]string{
			"x":   "y",
			"foo": "bar",
			"z":   "",
		},
	},
}, {
	about: "don't mention hyphen if all entries are mandatory",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tint,
				Mandatory:   true,
			},
			"b": environschema.Attr{
				Description: "b description",
				Type:        environschema.Tstring,
				Mandatory:   true,
			},
		},
	},
	expectIO: `
	|Press return to select a default value.
	|a: »12
	|b: »-
	`,
	expectResult: map[string]interface{}{
		"a": 12,
		"b": "-",
	},
}, {
	about: "too many bad responses",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tint,
				Mandatory:   true,
			},
		},
	},
	expectIO: `
	|Press return to select a default value.
	|a: »one
	|Invalid input: expected number, got string("one")
	|a: »
	|Invalid input: expected number, got string("")
	|a: »three
	|Invalid input: expected number, got string("three")
	`,
	expectError: `cannot complete form: too many invalid inputs`,
}, {
	about: "too many bad responses with maxtries=1",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tint,
			},
		},
	},
	filler: form.IOFiller{
		MaxTries: 1,
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a: »one
	|Invalid input: expected number, got string("one")
	`,
	expectError: `cannot complete form: too many invalid inputs`,
}, {
	about: "bad then good input",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tint,
			},
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a: »one
	|Invalid input: expected number, got string("one")
	|a: »two
	|Invalid input: expected number, got string("two")
	|a: »3
	`,
	expectResult: map[string]interface{}{
		"a": 3,
	},
}, {
	about: "empty value entered for optional attribute with no default",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tstring,
			},
			"b": environschema.Attr{
				Description: "b description",
				Type:        environschema.Tint,
			},
			"c": environschema.Attr{
				Description: "c description",
				Type:        environschema.Tbool,
			},
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a: »
	|b: »
	|c: »
	`,
	expectResult: map[string]interface{}{},
}, {
	about: "unsupported type",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        "bogus",
			},
		},
	},
	expectError: `invalid field a: invalid type "bogus"`,
}, {
	about: "no interaction is done if any field has an invalid type",
	form: form.Form{
		Title: "some title",
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tstring,
			},
			"b": environschema.Attr{
				Description: "b description",
				Type:        "bogus",
			},
		},
	},
	expectError: `invalid field b: invalid type "bogus"`,
}, {
	about: "invalid default value is ignored",
	environment: map[string]string{
		"a": "three",
	},
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tint,
				EnvVars:     []string{"a"},
			},
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|Warning: invalid default value: cannot convert $a: expected number, got string("three")
	|a: »99
	`,
	expectResult: map[string]interface{}{
		"a": 99,
	},
}, {
	about: "entering a hyphen causes an optional value to be omitted",
	environment: map[string]string{
		"a": "29",
	},
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tint,
				EnvVar:      "a",
			},
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a [29]: »-
	|Value a omitted.
	`,
	expectResult: map[string]interface{}{},
}, {
	about: "entering a hyphen causes a mandatory value to be fail when there are other optional values",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tint,
				Mandatory:   true,
			},
			"b": environschema.Attr{
				Description: "b description",
				Type:        environschema.Tint,
			},
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a: »-
	|Cannot omit a because it is mandatory.
	|a: »123
	|b: »99
	`,
	expectResult: map[string]interface{}{
		"a": 123,
		"b": 99,
	},
}, {
	about: "descriptions can be enabled with ShowDescriptions",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "  The a attribute\nis pretty boring.\n\n",
				Type:        environschema.Tstring,
				Mandatory:   true,
			},
			"b": environschema.Attr{
				Type: environschema.Tint,
			},
		},
	},
	filler: form.IOFiller{
		ShowDescriptions: true,
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|
	|The a attribute
	|is pretty boring.
	|a: »-
	|Cannot omit a because it is mandatory.
	|a: »value
	|b: »99
	`,
	expectResult: map[string]interface{}{
		"a": "value",
		"b": 99,
	},
}, {
	about: "custom GetDefault value success",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tstring,
			},
		},
	},
	filler: form.IOFiller{
		GetDefault: func(attr form.NamedAttr, checker schema.Checker) (interface{}, string, error) {
			return "hello", "", nil
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a [hello]: »
	`,
	expectResult: map[string]interface{}{
		"a": "hello",
	},
}, {
	about: "custom GetDefault value error",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tstring,
			},
		},
	},
	filler: form.IOFiller{
		GetDefault: func(attr form.NamedAttr, checker schema.Checker) (interface{}, string, error) {
			return nil, "", errgo.New("some error")
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|Warning: invalid default value: some error
	|a: »value
	`,
	expectResult: map[string]interface{}{
		"a": "value",
	},
}, {
	about: "custom GetDefault value with custom display",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tint,
			},
		},
	},
	filler: form.IOFiller{
		GetDefault: func(attr form.NamedAttr, checker schema.Checker) (interface{}, string, error) {
			return 99, "ninety-nine", nil
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a [ninety-nine]: »
	`,
	expectResult: map[string]interface{}{
		"a": 99,
	},
}, {
	about: "custom GetDefault value with empty display and non-string type",
	form: form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tint,
			},
		},
	},
	filler: form.IOFiller{
		GetDefault: func(attr form.NamedAttr, checker schema.Checker) (interface{}, string, error) {
			return 99, "", nil
		},
	},
	expectIO: `
	|Press return to select a default value, or enter - to omit an entry.
	|a [99]: »
	`,
	expectResult: map[string]interface{}{
		"a": 99,
	},
}}

func (formSuite) TestIOFiller(c *gc.C) {
	for i, test := range ioFillerTests {
		c.Logf("%d. %s", i, test.about)
		for k, v := range test.environment {
			err := os.Setenv(k, v)
			c.Assert(err, jc.ErrorIsNil)
		}
		ioChecker := newInteractionChecker(c, "»", strings.TrimPrefix(unbeautify(test.expectIO), "\n"))
		ioFiller := test.filler
		ioFiller.In = ioChecker
		ioFiller.Out = ioChecker
		result, err := ioFiller.Fill(test.form)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(result, gc.IsNil)
		} else {
			ioChecker.Close()
			c.Assert(err, gc.IsNil)
			c.Assert(result, gc.DeepEquals, test.expectResult)
		}
		for k := range test.environment {
			err = os.Unsetenv(k)
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

func (formSuite) TestIOFillerReadError(c *gc.C) {
	r := errorReader{}
	var out bytes.Buffer
	ioFiller := form.IOFiller{
		In:  r,
		Out: &out,
	}
	result, err := ioFiller.Fill(form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tstring,
			},
		},
	})
	c.Check(out.String(), gc.Equals, "Press return to select a default value, or enter - to omit an entry.\na: ")
	c.Assert(err, gc.ErrorMatches, `cannot complete form: cannot read input: some read error`)
	c.Assert(result, gc.IsNil)
	// Verify that the cause is masked. Maybe it shouldn't
	// be, but test the code as it is.
	c.Assert(errgo.Cause(err), gc.Not(gc.Equals), errRead)
}

func (formSuite) TestIOFillerUnexpectedEOF(c *gc.C) {
	r := strings.NewReader("a")
	var out bytes.Buffer
	ioFiller := form.IOFiller{
		In:  r,
		Out: &out,
	}
	result, err := ioFiller.Fill(form.Form{
		Fields: environschema.Fields{
			"a": environschema.Attr{
				Description: "a description",
				Type:        environschema.Tstring,
			},
		},
	})
	c.Check(out.String(), gc.Equals, "Press return to select a default value, or enter - to omit an entry.\na: ")
	c.Assert(err, gc.ErrorMatches, `cannot complete form: cannot read input: unexpected EOF`)
	c.Assert(result, gc.IsNil)
}

func (formSuite) TestSortedFields(c *gc.C) {
	fields := environschema.Fields{
		"a1": environschema.Attr{
			Group:       "A",
			Description: "a1 description",
			Type:        environschema.Tstring,
		},
		"c1": environschema.Attr{
			Group:       "A",
			Description: "c1 description",
			Type:        environschema.Tstring,
		},
		"b1": environschema.Attr{
			Group:       "A",
			Description: "b1 description",
			Type:        environschema.Tstring,
			Secret:      true,
		},
		"a2": environschema.Attr{
			Group:       "B",
			Description: "a2 description",
			Type:        environschema.Tstring,
		},
		"c2": environschema.Attr{
			Group:       "B",
			Description: "c2 description",
			Type:        environschema.Tstring,
		},
		"b2": environschema.Attr{
			Group:       "B",
			Description: "b2 description",
			Type:        environschema.Tstring,
			Secret:      true,
		},
	}
	c.Assert(form.SortedFields(fields), gc.DeepEquals, []form.NamedAttr{{
		Name: "a1",
		Attr: environschema.Attr{
			Group:       "A",
			Description: "a1 description",
			Type:        environschema.Tstring,
		}}, {
		Name: "c1",
		Attr: environschema.Attr{
			Group:       "A",
			Description: "c1 description",
			Type:        environschema.Tstring,
		}}, {
		Name: "b1",
		Attr: environschema.Attr{
			Group:       "A",
			Description: "b1 description",
			Type:        environschema.Tstring,
			Secret:      true,
		}}, {
		Name: "a2",
		Attr: environschema.Attr{
			Group:       "B",
			Description: "a2 description",
			Type:        environschema.Tstring,
		}}, {
		Name: "c2",
		Attr: environschema.Attr{
			Group:       "B",
			Description: "c2 description",
			Type:        environschema.Tstring,
		}}, {
		Name: "b2",
		Attr: environschema.Attr{
			Group:       "B",
			Description: "b2 description",
			Type:        environschema.Tstring,
			Secret:      true,
		},
	}})
}

var errRead = errgo.New("some read error")

type errorReader struct{}

func (r errorReader) Read([]byte) (int, error) {
	return 0, errRead
}

var defaultFromEnvTests = []struct {
	about       string
	environment map[string]string
	attr        environschema.Attr
	expect      interface{}
	expectError string
}{{
	about: "no envvars",
	attr: environschema.Attr{
		EnvVar: "A",
		Type:   environschema.Tstring,
	},
}, {
	about: "matching envvar",
	environment: map[string]string{
		"A": "B",
	},
	attr: environschema.Attr{
		EnvVar: "A",
		Type:   environschema.Tstring,
	},
	expect: "B",
}, {
	about: "matching envvars",
	environment: map[string]string{
		"B": "C",
	},
	attr: environschema.Attr{
		EnvVar:  "A",
		Type:    environschema.Tstring,
		EnvVars: []string{"B"},
	},
	expect: "C",
}, {
	about: "envvar takes priority",
	environment: map[string]string{
		"A": "1",
		"B": "2",
	},
	attr: environschema.Attr{
		EnvVar:  "A",
		Type:    environschema.Tstring,
		EnvVars: []string{"B"},
	},
	expect: "1",
}, {
	about: "cannot coerce",
	environment: map[string]string{
		"A": "B",
	},
	attr: environschema.Attr{
		EnvVar: "A",
		Type:   environschema.Tint,
	},
	expectError: `cannot convert \$A: expected number, got string\("B"\)`,
}}

func (formSuite) TestDefaultFromEnv(c *gc.C) {
	for _, test := range defaultFromEnvTests {
		for k, v := range test.environment {
			err := os.Setenv(k, v)
			c.Assert(err, jc.ErrorIsNil)
		}
		checker, err := test.attr.Checker()
		c.Assert(err, gc.IsNil)
		result, display, err := form.DefaultFromEnv(form.NamedAttr{
			Name: "ignored",
			Attr: test.attr,
		}, checker)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(display, gc.Equals, "")
			c.Assert(result, gc.Equals, nil)
			return
		}
		c.Assert(err, gc.IsNil)
		c.Assert(display, gc.Equals, "")
		c.Assert(result, gc.Equals, test.expect)
		for k := range test.environment {
			err := os.Unsetenv(k)
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

// indentReplacer deletes tabs and | beautifier characters.
var indentReplacer = strings.NewReplacer("\t", "", "|", "")

// unbeautify strips the leading tabs and | characters that
// we use to make the tests look nicer.
func unbeautify(s string) string {
	return indentReplacer.Replace(s)
}
