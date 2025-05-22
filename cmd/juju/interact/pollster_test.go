// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interact

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type PollsterSuite struct {
	testhelpers.IsolationSuite
}

func TestPollsterSuite(t *testing.T) {
	tc.Run(t, &PollsterSuite{})
}

func (s *PollsterSuite) TearDownTest(c *tc.C) {
	s.IsolationSuite.TearDownTest(c)
	os.Unsetenv("SCHEMA_VAR")
	os.Unsetenv("SCHEMA_VAR_TWO")
}

func (s *PollsterSuite) TestSelect(c *tc.C) {
	r := strings.NewReader("macintosh")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	sel, err := p.Select(List{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sel, tc.Equals, "macintosh")

	// Note: please only check the full output here, so that we don't have to
	// edit a million tests if we make minor tweaks to the output.
	c.Assert(w.String(), tc.Equals, `
Apples
  macintosh
  granny smith

Select apple: 
`[1:])
}

func (s *PollsterSuite) TestSelectDefault(c *tc.C) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	sel, err := p.Select(List{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith"},
		Default:  "macintosh",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sel, tc.Equals, "macintosh")
	c.Assert(w.String(), tc.Contains, `Select apple [macintosh]: `)
}

func (s *PollsterSuite) TestSelectDefaultIfOnlyOption(c *tc.C) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	sel, err := p.Select(List{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh"},
		Default:  "macintosh",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sel, tc.Equals, "macintosh")
	c.Assert(w.String(), tc.Contains, `Select apple [macintosh]: `)
}

func (s *PollsterSuite) TestSelectIncorrect(c *tc.C) {
	r := strings.NewReader("mac\nmacintosh")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	sel, err := p.Select(List{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sel, tc.Equals, "macintosh")

	c.Assert(squash(w.String()), tc.Contains, `Invalid apple: "mac"Select apple:`)
}

// squash removes all newlines from the given string so our tests can be more
// resilient in the face of minor tweaks to spacing.
func squash(s string) string {
	return strings.Replace(s, "\n", "", -1)
}

func (s *PollsterSuite) TestSelectNoMultiple(c *tc.C) {
	r := strings.NewReader("macintosh,granny smith\ngranny smith")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	sel, err := p.Select(List{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(sel, tc.Equals, "granny smith")
	c.Assert(w.String(), tc.Contains, `Invalid apple: "macintosh,granny smith"`)
}

func (s *PollsterSuite) TestMultiSelectSingle(c *tc.C) {
	r := strings.NewReader("macintosh")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	vals, err := p.MultiSelect(MultiList{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith", "gala"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vals, tc.SameContents, []string{"macintosh"})
}

func (s *PollsterSuite) TestMultiSelectMany(c *tc.C) {
	// note there's a couple spaces in the middle here that we're stripping out.
	r := strings.NewReader("macintosh,  granny smith")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	vals, err := p.MultiSelect(MultiList{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith", "gala"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vals, tc.SameContents, []string{"macintosh", "granny smith"})
}

func (s *PollsterSuite) TestMultiSelectDefault(c *tc.C) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	vals, err := p.MultiSelect(MultiList{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith", "gala"},
		Default:  []string{"gala", "granny smith"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vals, tc.SameContents, []string{"gala", "granny smith"})
}

func (s *PollsterSuite) TestMultiSelectDefaultIfOnlyOne(c *tc.C) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	vals, err := p.MultiSelect(MultiList{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh"},
		Default:  []string{"macintosh"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vals, tc.SameContents, []string{"macintosh"})
	c.Assert(w.String(), tc.Equals, "Apples\n  macintosh\n\n")
}

func (s *PollsterSuite) TestMultiSelectWithMultipleDefaults(c *tc.C) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	vals, err := p.MultiSelect(MultiList{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "gala"},
		Default:  []string{"macintosh", "gala"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vals, tc.SameContents, []string{"macintosh", "gala"})
	c.Assert(w.String(), tc.Contains, "Select one or more apples separated by commas [macintosh, gala]: \n")
}

func (s *PollsterSuite) TestMultiSelectOneError(c *tc.C) {
	r := strings.NewReader("mac\nmacintosh")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	vals, err := p.MultiSelect(MultiList{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith", "gala"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vals, tc.SameContents, []string{"macintosh"})
	c.Assert(w.String(), tc.Contains, `Invalid apple: "mac"`)
}

func (s *PollsterSuite) TestMultiSelectManyErrors(c *tc.C) {
	r := strings.NewReader("mac,  smith\nmacintosh")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	vals, err := p.MultiSelect(MultiList{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith", "gala"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vals, tc.SameContents, []string{"macintosh"})
	c.Assert(w.String(), tc.Contains, `Invalid apples: "mac", "smith"`)
}

func (s *PollsterSuite) TestEnter(c *tc.C) {
	r := strings.NewReader("Bill Smith")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.Enter("your name")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(a, tc.Equals, "Bill Smith")
	c.Assert(w.String(), tc.Equals, "Enter your name: \n")
}

func (s *PollsterSuite) TestEnterEmpty(c *tc.C) {
	r := strings.NewReader("\nBill")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.Enter("your name")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(a, tc.Equals, "Bill")
	// We should re-query without any error on empty input.
	c.Assert(squash(w.String()), tc.Contains, "Enter your name: Enter your name: ")
}

func (s *PollsterSuite) TestEnterVerify(c *tc.C) {
	r := strings.NewReader("Bill Smith")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	verify := func(s string) (ok bool, errmsg string, err error) {
		if s == "Bill Smith" {
			return true, "", nil
		}
		return false, "not bill!", nil
	}
	a, err := p.EnterVerify("your name", verify)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(a, tc.Equals, "Bill Smith")
	c.Assert(w.String(), tc.Equals, "Enter your name: \n")
}

func (s *PollsterSuite) TestEnterVerifyBad(c *tc.C) {
	r := strings.NewReader("Will Smithy\nBill Smith")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	verify := func(s string) (ok bool, errmsg string, err error) {
		if s == "Bill Smith" {
			return true, "", nil
		}
		return false, "not bill!", nil
	}
	a, err := p.EnterVerify("your name", verify)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(a, tc.Equals, "Bill Smith")
	c.Assert(squash(w.String()), tc.Equals, "Enter your name: not bill!Enter your name: ")
}

func (s *PollsterSuite) TestEnterDefaultNonEmpty(c *tc.C) {
	r := strings.NewReader("Bill Smith")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.EnterDefault("your name", "John")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(a, tc.Equals, "Bill Smith")
	c.Assert(w.String(), tc.Equals, "Enter your name [John]: \n")
}

func (s *PollsterSuite) TestEnterDefaultEmpty(c *tc.C) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.EnterDefault("your name", "John")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(a, tc.Equals, "John")
	// We should re-query without any error on empty input.
	c.Assert(squash(w.String()), tc.Contains, "Enter your name [John]: ")
}

func (s *PollsterSuite) TestEnterVerifyDefaultEmpty(c *tc.C) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	// note that the verification does not accept empty string, but the default
	// should still work
	verify := func(s string) (ok bool, errmsg string, err error) {
		if s == "Bill Smith" {
			return true, "", nil
		}
		return false, "not bill!", nil
	}
	a, err := p.EnterVerifyDefault("your name", verify, "John")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(a, tc.Equals, "John")
	// We should re-query without any error on empty input.
	c.Assert(squash(w.String()), tc.Contains, "Enter your name [John]: ")
}

func (s *PollsterSuite) TestYNDefaultFalse(c *tc.C) {
	r := strings.NewReader("Y")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.YN("Should this test pass", false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(a, tc.IsTrue)
	c.Assert(w.String(), tc.Equals, "Should this test pass? (y/N): \n")
}

func (s *PollsterSuite) TestYNDefaultTrue(c *tc.C) {
	r := strings.NewReader("Y")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.YN("Should this test pass", true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(a, tc.IsTrue)
	c.Assert(w.String(), tc.Equals, "Should this test pass? (Y/n): \n")
}

func (s *PollsterSuite) TestYNTable(c *tc.C) {
	tests := []struct {
		In       string
		Def, Res bool
	}{
		0:  {In: "Y", Def: false, Res: true},
		1:  {In: "y", Def: false, Res: true},
		2:  {In: "yes", Def: false, Res: true},
		3:  {In: "YES", Def: false, Res: true},
		4:  {In: "N", Def: true, Res: false},
		5:  {In: "n", Def: true, Res: false},
		6:  {In: "no", Def: true, Res: false},
		7:  {In: "NO", Def: true, Res: false},
		8:  {In: "Y", Def: true, Res: true},
		9:  {In: "y", Def: true, Res: true},
		10: {In: "yes", Def: true, Res: true},
		11: {In: "YES", Def: true, Res: true},
		12: {In: "N", Def: false, Res: false},
		13: {In: "n", Def: false, Res: false},
		14: {In: "no", Def: false, Res: false},
		15: {In: "NO", Def: false, Res: false},
	}
	for i, test := range tests {
		c.Logf("test %d", i)
		r := strings.NewReader(test.In)
		w := &bytes.Buffer{}
		p := New(r, w, w)
		a, err := p.YN("doesn't matter", test.Def)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(a, tc.Equals, test.Res)
	}
}

func (s *PollsterSuite) TestYNInvalid(c *tc.C) {
	r := strings.NewReader("wat\nY")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.YN("Should this test pass", false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(a, tc.IsTrue)
	c.Assert(w.String(), tc.Contains, `Invalid entry: "wat", please choose y or n`)
}

func (s *PollsterSuite) TestQueryStringSchema(c *tc.C) {
	schema := &jsonschema.Schema{
		Singular: "region",
		Type:     []jsonschema.Type{jsonschema.StringType},
	}
	r := strings.NewReader("wat")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	sel, ok := v.(string)
	c.Check(ok, tc.IsTrue)
	c.Check(sel, tc.Equals, "wat")
	c.Assert(w.String(), tc.Contains, "Enter region:")
}

func (s *PollsterSuite) TestQueryStringSchemaWithDefault(c *tc.C) {
	schema := &jsonschema.Schema{
		Singular: "region",
		Type:     []jsonschema.Type{jsonschema.StringType},
		Default:  "foo",
	}
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	sel, ok := v.(string)
	c.Check(ok, tc.IsTrue)
	c.Check(sel, tc.Equals, "foo")
	c.Assert(w.String(), tc.Contains, "Enter region [foo]:")
}

func (s *PollsterSuite) TestQueryStringSchemaWithUnusedDefault(c *tc.C) {
	schema := &jsonschema.Schema{
		Singular: "region",
		Type:     []jsonschema.Type{jsonschema.StringType},
		Default:  "foo",
	}
	r := strings.NewReader("bar\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	sel, ok := v.(string)
	c.Check(ok, tc.IsTrue)
	c.Check(sel, tc.Equals, "bar")
	c.Assert(w.String(), tc.Contains, "Enter region [foo]:")
}

func (s *PollsterSuite) TestQueryStringSchemaWithPromptDefault(c *tc.C) {
	schema := &jsonschema.Schema{
		Singular:      "region",
		Type:          []jsonschema.Type{jsonschema.StringType},
		Default:       "foo",
		PromptDefault: "not foo",
	}
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	sel, ok := v.(string)
	c.Check(ok, tc.IsTrue)
	c.Check(sel, tc.Equals, "foo")
	c.Check(w.String(), tc.Contains, "Enter region [not foo]:")
}

func (s *PollsterSuite) TestQueryStringSchemaWithDefaultEnvVar(c *tc.C) {
	schema := &jsonschema.Schema{
		Singular: "region",
		Type:     []jsonschema.Type{jsonschema.StringType},
		Default:  "",
		EnvVars:  []string{"SCHEMA_VAR"},
	}
	os.Setenv("SCHEMA_VAR", "value from env var")
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	sel, ok := v.(string)
	c.Check(ok, tc.IsTrue)
	c.Check(sel, tc.Equals, "value from env var")
	c.Assert(w.String(), tc.Contains, "Enter region [value from env var]:")
}

func (s *PollsterSuite) TestQueryStringSchemaWithDefaultEnvVarOverride(c *tc.C) {
	schema := &jsonschema.Schema{
		Singular: "region",
		Type:     []jsonschema.Type{jsonschema.StringType},
		Default:  "",
		EnvVars:  []string{"SCHEMA_VAR"},
	}
	os.Setenv("SCHEMA_VAR", "value from env var")
	r := strings.NewReader("use me\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	sel, ok := v.(string)
	c.Check(ok, tc.IsTrue)
	c.Check(sel, tc.Equals, "use me")
	c.Assert(w.String(), tc.Contains, "Enter region [value from env var]:")
}

func (s *PollsterSuite) TestQueryStringSchemaWithDefaultTwoEnvVar(c *tc.C) {
	schema := &jsonschema.Schema{
		Singular: "region",
		Type:     []jsonschema.Type{jsonschema.StringType},
		Default:  "",
		EnvVars:  []string{"SCHEMA_VAR", "SCHEMA_VAR_TWO"},
	}
	os.Setenv("SCHEMA_VAR_TWO", "value from second")
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	sel, ok := v.(string)
	c.Check(ok, tc.IsTrue)
	c.Check(sel, tc.Equals, "value from second")
	c.Assert(w.String(), tc.Contains, "Enter region [value from second]:")
}

func (s *PollsterSuite) TestQueryURISchema(c *tc.C) {
	schema := &jsonschema.Schema{
		Singular: "region",
		Type:     []jsonschema.Type{jsonschema.StringType},
		Format:   jsonschema.FormatURI,
	}
	// invalid escape sequence
	r := strings.NewReader("https://&%5abc")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	_, err := p.QuerySchema(schema)
	c.Check(errors.Cause(err), tc.Equals, io.EOF)
	c.Assert(w.String(), tc.Equals, `
Enter region: Invalid URI: "https://&%5abc"

Enter region: 
`[1:])
}

func (s *PollsterSuite) TestQueryArraySchema(c *tc.C) {
	schema := &jsonschema.Schema{
		Singular: "number",
		Plural:   "numbers",
		Type:     []jsonschema.Type{jsonschema.ArrayType},
		Items: &jsonschema.ItemSpec{
			Schemas: []*jsonschema.Schema{{
				Type: []jsonschema.Type{jsonschema.StringType},
				Enum: []interface{}{
					"one",
					"two",
					"three",
				},
			}},
		},
	}
	r := strings.NewReader("one, three")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w.String(), tc.Equals, `
Numbers
  one
  two
  three

Select one or more numbers separated by commas: 
`[1:])
	sel, ok := v.([]string)
	c.Check(ok, tc.IsTrue)
	c.Check(sel, tc.SameContents, []string{"one", "three"})
}

func (s *PollsterSuite) TestQueryArraySchemaDefault(c *tc.C) {
	schema := &jsonschema.Schema{
		Singular: "number",
		Plural:   "numbers",
		Type:     []jsonschema.Type{jsonschema.ArrayType},
		Default:  "two",
		Items: &jsonschema.ItemSpec{
			Schemas: []*jsonschema.Schema{{
				Type: []jsonschema.Type{jsonschema.StringType},
				Enum: []interface{}{
					"one",
					"two",
					"three",
				},
			}},
		},
	}
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w.String(), tc.Equals, `
Numbers
  one
  two
  three

Select one or more numbers separated by commas [two]: 
`[1:])
	sel, ok := v.([]string)
	c.Check(ok, tc.IsTrue)
	c.Check(sel, tc.SameContents, []string{"two"})
}

func (s *PollsterSuite) TestQueryArraySchemaNotDefault(c *tc.C) {
	schema := &jsonschema.Schema{
		Singular: "number",
		Plural:   "numbers",
		Type:     []jsonschema.Type{jsonschema.ArrayType},
		Default:  "two",
		Items: &jsonschema.ItemSpec{
			Schemas: []*jsonschema.Schema{{
				Type: []jsonschema.Type{jsonschema.StringType},
				Enum: []interface{}{
					"one",
					"two",
					"three",
				},
			}},
		},
	}
	r := strings.NewReader("three")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w.String(), tc.Equals, `
Numbers
  one
  two
  three

Select one or more numbers separated by commas [two]: 
`[1:])
	sel, ok := v.([]string)
	c.Check(ok, tc.IsTrue)
	c.Check(sel, tc.SameContents, []string{"three"})
}

func (s *PollsterSuite) TestQueryEnum(c *tc.C) {
	schema := &jsonschema.Schema{
		Singular: "number",
		Plural:   "numbers",
		Type:     []jsonschema.Type{jsonschema.IntegerType},
		Enum: []interface{}{
			1,
			2,
			3,
		},
	}
	r := strings.NewReader("2")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w.String(), tc.Equals, `
Numbers
  1
  2
  3

Select number: 
`[1:])
	i, ok := v.(int)
	c.Check(ok, tc.IsTrue)
	c.Check(i, tc.Equals, 2)
}

func (s *PollsterSuite) TestQueryObjectSchema(c *tc.C) {
	schema := &jsonschema.Schema{
		Type: []jsonschema.Type{jsonschema.ObjectType},
		Properties: map[string]*jsonschema.Schema{
			"numbers": {
				Singular: "number",
				Plural:   "numbers",
				Type:     []jsonschema.Type{jsonschema.ArrayType},
				Items: &jsonschema.ItemSpec{
					Schemas: []*jsonschema.Schema{{
						Type: []jsonschema.Type{jsonschema.StringType},
						Enum: []interface{}{
							"one",
							"two",
							"three",
						},
					}},
				},
			},
			"name": {
				Type:     []jsonschema.Type{jsonschema.StringType},
				Singular: "the name",
			},
		},
	}
	// queries should be alphabetical without an order specified, so name then
	// number.
	r := strings.NewReader("Bill\ntwo, three")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(v, tc.DeepEquals, map[string]interface{}{
		"name":    "Bill",
		"numbers": []string{"two", "three"},
	})
}

func (s *PollsterSuite) TestQueryObjectSchemaOrder(c *tc.C) {
	schema := &jsonschema.Schema{
		Type: []jsonschema.Type{jsonschema.ObjectType},
		// Order should match up with order of input in strings.NewReader below.
		Order: []string{"numbers", "name"},
		Properties: map[string]*jsonschema.Schema{
			"numbers": {
				Singular: "number",
				Plural:   "numbers",
				Type:     []jsonschema.Type{jsonschema.ArrayType},
				Items: &jsonschema.ItemSpec{
					Schemas: []*jsonschema.Schema{{
						Type: []jsonschema.Type{jsonschema.StringType},
						Enum: []interface{}{
							"one",
							"two",
							"three",
						},
					}},
				},
			},
			"name": {
				Type:     []jsonschema.Type{jsonschema.StringType},
				Singular: "the name",
			},
		},
	}
	// queries should be ordered by order, so number then name.
	r := strings.NewReader("two, three\nBill")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(v, tc.DeepEquals, map[string]interface{}{
		"name":    "Bill",
		"numbers": []string{"two", "three"},
	})
}

func (s *PollsterSuite) TestQueryObjectSchemaAdditional(c *tc.C) {
	schema := &jsonschema.Schema{
		Type:     []jsonschema.Type{jsonschema.ObjectType},
		Singular: "region",
		Plural:   "regions",
		AdditionalProperties: &jsonschema.Schema{
			Type: []jsonschema.Type{jsonschema.ObjectType},
			Properties: map[string]*jsonschema.Schema{
				"loc": {
					Singular: "location",
					Type:     []jsonschema.Type{jsonschema.StringType},
				},
			},
		},
	}
	r := strings.NewReader(`
one
east
y
two
west
n
`[1:])
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(v, tc.DeepEquals, map[string]interface{}{
		"one": map[string]interface{}{"loc": "east"},
		"two": map[string]interface{}{"loc": "west"},
	})
	c.Check(w.String(), tc.Equals, `
Enter region name: 
Enter location: 
Enter another region? (y/N): 
Enter region name: 
Enter location: 
Enter another region? (y/N): 
`[1:])
}

func (s *PollsterSuite) TestQueryObjectSchemaAdditionalEmpty(c *tc.C) {
	schema := &jsonschema.Schema{
		Type:     []jsonschema.Type{jsonschema.ObjectType},
		Singular: "region",
		Plural:   "regions",
		AdditionalProperties: &jsonschema.Schema{
			Type: []jsonschema.Type{jsonschema.ObjectType},
		},
	}
	r := strings.NewReader(`
one
y
two
n
`[1:])
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(v, tc.DeepEquals, map[string]interface{}{
		"one": map[string]interface{}{},
		"two": map[string]interface{}{},
	})
	c.Check(w.String(), tc.Equals, `
Enter region name: 
Enter another region? (y/N): 
Enter region name: 
Enter another region? (y/N): 
`[1:])
}

func (s *PollsterSuite) TestQueryObjectSchemaWithOutDefault(c *tc.C) {
	schema := &jsonschema.Schema{
		Type:  []jsonschema.Type{jsonschema.ObjectType},
		Order: []string{"name", "nested", "bar"},
		Properties: map[string]*jsonschema.Schema{
			"nested": {
				Singular: "nested",
				Type:     []jsonschema.Type{jsonschema.ObjectType},
				AdditionalProperties: &jsonschema.Schema{
					Type:          []jsonschema.Type{jsonschema.ObjectType},
					Required:      []string{"name"},
					MaxProperties: jsonschema.Int(1),
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Singular:      "the name",
							Type:          []jsonschema.Type{jsonschema.StringType},
							Default:       "",
							PromptDefault: "use name",
						},
					},
				},
			},
			"bar": {
				Singular: "nested",
				Type:     []jsonschema.Type{jsonschema.ObjectType},
				Default:  "",
				AdditionalProperties: &jsonschema.Schema{
					Type:          []jsonschema.Type{jsonschema.ObjectType},
					Required:      []string{"name"},
					MaxProperties: jsonschema.Int(1),
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Singular:      "the name",
							Type:          []jsonschema.Type{jsonschema.StringType},
							Default:       "",
							PromptDefault: "use name",
						},
					},
				},
			},
			"name": {
				Type:     []jsonschema.Type{jsonschema.StringType},
				Singular: "the name",
			},
		},
	}
	// queries should be alphabetical without an order specified, so name then
	// number.
	r := strings.NewReader("Bill\n\nnamespace\n\n\nfoo\nbaz\n\n\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(v, tc.DeepEquals, map[string]interface{}{
		"name": "Bill",
		"nested": map[string]interface{}{
			"namespace": map[string]interface{}{
				"name": "",
			},
		},
		"bar": map[string]interface{}{
			"foo": map[string]interface{}{
				"name": "baz",
			},
		},
	})
}

func (s *PollsterSuite) TestQueryObjectSchemaWithDefault(c *tc.C) {
	schema := &jsonschema.Schema{
		Type:  []jsonschema.Type{jsonschema.ObjectType},
		Order: []string{"name", "nested"},
		Properties: map[string]*jsonschema.Schema{
			"nested": {
				Singular: "nested",
				Default:  "default",
				Type:     []jsonschema.Type{jsonschema.ObjectType},
				AdditionalProperties: &jsonschema.Schema{
					Type:          []jsonschema.Type{jsonschema.ObjectType},
					Required:      []string{"name"},
					MaxProperties: jsonschema.Int(1),
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Singular:      "the name",
							Type:          []jsonschema.Type{jsonschema.StringType},
							Default:       "",
							PromptDefault: "use name",
						},
					},
				},
			},
			"name": {
				Type:     []jsonschema.Type{jsonschema.StringType},
				Singular: "the name",
			},
		},
	}
	// queries should be alphabetical without an order specified, so name then
	// number.
	r := strings.NewReader("Bill\n\n\n\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(v, tc.DeepEquals, map[string]interface{}{
		"name": "Bill",
		"nested": map[string]interface{}{
			"default": map[string]interface{}{
				"name": "",
			},
		},
	})
}

func (s *PollsterSuite) TestQueryObjectSchemaWithDefaultEnvVars(c *tc.C) {
	schema := &jsonschema.Schema{
		Type:  []jsonschema.Type{jsonschema.ObjectType},
		Order: []string{"name", "nested"},
		Properties: map[string]*jsonschema.Schema{
			"nested": {
				Singular:      "nested",
				Default:       "default",
				PromptDefault: "use default value",
				EnvVars:       []string{"TEST_ENV_VAR_NESTED"},
				Type:          []jsonschema.Type{jsonschema.ObjectType},
				AdditionalProperties: &jsonschema.Schema{
					Type:          []jsonschema.Type{jsonschema.ObjectType},
					Required:      []string{"name"},
					MaxProperties: jsonschema.Int(1),
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Singular:      "the name",
							Type:          []jsonschema.Type{jsonschema.StringType},
							Default:       "",
							PromptDefault: "use name",
						},
					},
				},
			},
			"name": {
				Type:     []jsonschema.Type{jsonschema.StringType},
				Singular: "the name",
			},
		},
	}
	// queries should be alphabetical without an order specified, so name then
	// number.
	os.Setenv("TEST_ENV_VAR_NESTED", "baz")
	defer os.Unsetenv("TEST_ENV_VAR_NESTED")

	r := strings.NewReader("Bill\n\n\n\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(v, tc.DeepEquals, map[string]interface{}{
		"name": "Bill",
		"nested": map[string]interface{}{
			"baz": map[string]interface{}{
				"name": "",
			},
		},
	})
}

func (s *PollsterSuite) TestQueryObjectSchemaEnvVarsWithOutDefault(c *tc.C) {
	schema := &jsonschema.Schema{
		Type:  []jsonschema.Type{jsonschema.ObjectType},
		Order: []string{"name", "nested"},
		Properties: map[string]*jsonschema.Schema{
			"nested": {
				Singular: "nested",
				EnvVars:  []string{"TEST_ENV_VAR_NESTED"},
				Type:     []jsonschema.Type{jsonschema.ObjectType},
				AdditionalProperties: &jsonschema.Schema{
					Type:          []jsonschema.Type{jsonschema.ObjectType},
					Required:      []string{"name"},
					MaxProperties: jsonschema.Int(1),
					Properties: map[string]*jsonschema.Schema{
						"name": {
							Singular:      "the name",
							Type:          []jsonschema.Type{jsonschema.StringType},
							Default:       "",
							PromptDefault: "use name",
						},
					},
				},
			},
			"name": {
				Type:     []jsonschema.Type{jsonschema.StringType},
				Singular: "the name",
			},
		},
	}
	// queries should be alphabetical without an order specified, so name then
	// number.
	os.Setenv("TEST_ENV_VAR_NESTED", "baz")
	defer os.Unsetenv("TEST_ENV_VAR_NESTED")

	r := strings.NewReader("Bill\nbaz\n\n\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(v, tc.DeepEquals, map[string]interface{}{
		"name": "Bill",
		"nested": map[string]interface{}{
			"baz": map[string]interface{}{
				"name": "",
			},
		},
	})
}
