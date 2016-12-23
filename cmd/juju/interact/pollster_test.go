// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interact

import (
	"bytes"
	"io"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type PollsterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(PollsterSuite{})

func (PollsterSuite) TestSelect(c *gc.C) {
	r := strings.NewReader("macintosh")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	s, err := p.Select(List{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s, gc.Equals, "macintosh")

	// Note: please only check the full output here, so that we don't have to
	// edit a million tests if we make minor tweaks to the output.
	c.Assert(w.String(), gc.Equals, `
Apples
  macintosh
  granny smith

Select apple: 
`[1:])
}

func (PollsterSuite) TestSelectDefault(c *gc.C) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	s, err := p.Select(List{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith"},
		Default:  "macintosh",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s, gc.Equals, "macintosh")
	c.Assert(w.String(), jc.Contains, `Select apple [macintosh]: `)
}

func (PollsterSuite) TestSelectIncorrect(c *gc.C) {
	r := strings.NewReader("mac\nmacintosh")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	s, err := p.Select(List{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s, gc.Equals, "macintosh")

	c.Assert(squash(w.String()), jc.Contains, `Invalid apple: "mac"Select apple:`)
}

// squash removes all newlines from the given string so our tests can be more
// resilient in the face of minor tweaks to spacing.
func squash(s string) string {
	return strings.Replace(s, "\n", "", -1)
}

func (PollsterSuite) TestSelectNoMultiple(c *gc.C) {
	r := strings.NewReader("macintosh,granny smith\ngranny smith")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	s, err := p.Select(List{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s, gc.Equals, "granny smith")
	c.Assert(w.String(), jc.Contains, `Invalid apple: "macintosh,granny smith"`)
}

func (PollsterSuite) TestMultiSelectSingle(c *gc.C) {
	r := strings.NewReader("macintosh")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	vals, err := p.MultiSelect(MultiList{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith", "gala"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vals, jc.SameContents, []string{"macintosh"})
}

func (PollsterSuite) TestMultiSelectMany(c *gc.C) {
	// note there's a couple spaces in the middle here that we're stripping out.
	r := strings.NewReader("macintosh,  granny smith")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	vals, err := p.MultiSelect(MultiList{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith", "gala"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vals, jc.SameContents, []string{"macintosh", "granny smith"})
}

func (PollsterSuite) TestMultiSelectDefault(c *gc.C) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	vals, err := p.MultiSelect(MultiList{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith", "gala"},
		Default:  []string{"gala", "granny smith"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vals, jc.SameContents, []string{"gala", "granny smith"})
}

func (PollsterSuite) TestMultiSelectOneError(c *gc.C) {
	r := strings.NewReader("mac\nmacintosh")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	vals, err := p.MultiSelect(MultiList{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith", "gala"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vals, jc.SameContents, []string{"macintosh"})
	c.Assert(w.String(), jc.Contains, `Invalid apple: "mac"`)
}

func (PollsterSuite) TestMultiSelectManyErrors(c *gc.C) {
	r := strings.NewReader("mac,  smith\nmacintosh")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	vals, err := p.MultiSelect(MultiList{
		Singular: "apple",
		Plural:   "apples",
		Options:  []string{"macintosh", "granny smith", "gala"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vals, jc.SameContents, []string{"macintosh"})
	c.Assert(w.String(), jc.Contains, `Invalid apples: "mac", "smith"`)
}

func (PollsterSuite) TestEnter(c *gc.C) {
	r := strings.NewReader("Bill Smith")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.Enter("your name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, gc.Equals, "Bill Smith")
	c.Assert(w.String(), gc.Equals, "Enter your name: \n")
}

func (PollsterSuite) TestEnterEmpty(c *gc.C) {
	r := strings.NewReader("\nBill")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.Enter("your name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, gc.Equals, "Bill")
	// We should re-query without any error on empty input.
	c.Assert(squash(w.String()), jc.Contains, "Enter your name: Enter your name: ")
}

func (PollsterSuite) TestEnterVerify(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, gc.Equals, "Bill Smith")
	c.Assert(w.String(), gc.Equals, "Enter your name: \n")
}

func (PollsterSuite) TestEnterVerifyBad(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, gc.Equals, "Bill Smith")
	c.Assert(squash(w.String()), gc.Equals, "Enter your name: not bill!Enter your name: ")
}

func (PollsterSuite) TestEnterDefaultNonEmpty(c *gc.C) {
	r := strings.NewReader("Bill Smith")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.EnterDefault("your name", "John")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, gc.Equals, "Bill Smith")
	c.Assert(w.String(), gc.Equals, "Enter your name [John]: \n")
}

func (PollsterSuite) TestEnterDefaultEmpty(c *gc.C) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.EnterDefault("your name", "John")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, gc.Equals, "John")
	// We should re-query without any error on empty input.
	c.Assert(squash(w.String()), jc.Contains, "Enter your name [John]: ")
}

func (PollsterSuite) TestEnterVerifyDefaultEmpty(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, gc.Equals, "John")
	// We should re-query without any error on empty input.
	c.Assert(squash(w.String()), jc.Contains, "Enter your name [John]: ")
}

func (PollsterSuite) TestYNDefaultFalse(c *gc.C) {
	r := strings.NewReader("Y")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.YN("Should this test pass", false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, jc.IsTrue)
	c.Assert(w.String(), gc.Equals, "Should this test pass? (y/N): \n")
}

func (PollsterSuite) TestYNDefaultTrue(c *gc.C) {
	r := strings.NewReader("Y")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.YN("Should this test pass", true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, jc.IsTrue)
	c.Assert(w.String(), gc.Equals, "Should this test pass? (Y/n): \n")
}

func (PollsterSuite) TestYNTable(c *gc.C) {
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
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(a, gc.Equals, test.Res)
	}
}

func (PollsterSuite) TestYNInvalid(c *gc.C) {
	r := strings.NewReader("wat\nY")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	a, err := p.YN("Should this test pass", false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, jc.IsTrue)
	c.Assert(w.String(), jc.Contains, `Invalid entry: "wat", please choose y or n`)
}

func (PollsterSuite) TestQueryStringSchema(c *gc.C) {
	schema := &jsonschema.Schema{
		Singular: "region",
		Type:     []jsonschema.Type{jsonschema.StringType},
	}
	r := strings.NewReader("wat")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, jc.ErrorIsNil)
	s, ok := v.(string)
	c.Check(ok, jc.IsTrue)
	c.Check(s, gc.Equals, "wat")
	c.Assert(w.String(), jc.Contains, "Enter region:")
}

func (PollsterSuite) TestQueryStringSchemaWithDefault(c *gc.C) {
	schema := &jsonschema.Schema{
		Singular: "region",
		Type:     []jsonschema.Type{jsonschema.StringType},
		Default:  "foo",
	}
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, jc.ErrorIsNil)
	s, ok := v.(string)
	c.Check(ok, jc.IsTrue)
	c.Check(s, gc.Equals, "foo")
	c.Assert(w.String(), jc.Contains, "Enter region [foo]:")
}

func (PollsterSuite) TestQueryStringSchemaWithUnusedDefault(c *gc.C) {
	schema := &jsonschema.Schema{
		Singular: "region",
		Type:     []jsonschema.Type{jsonschema.StringType},
		Default:  "foo",
	}
	r := strings.NewReader("bar\n")
	w := &bytes.Buffer{}
	p := New(r, w, w)
	v, err := p.QuerySchema(schema)
	c.Assert(err, jc.ErrorIsNil)
	s, ok := v.(string)
	c.Check(ok, jc.IsTrue)
	c.Check(s, gc.Equals, "bar")
	c.Assert(w.String(), jc.Contains, "Enter region [foo]:")
}

func (PollsterSuite) TestQueryStringSchemaWithPromptDefault(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	s, ok := v.(string)
	c.Check(ok, jc.IsTrue)
	c.Check(s, gc.Equals, "foo")
	c.Check(w.String(), jc.Contains, "Enter region [not foo]:")
}

func (PollsterSuite) TestQueryURISchema(c *gc.C) {
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
	c.Check(errors.Cause(err), gc.Equals, io.EOF)
	c.Assert(w.String(), gc.Equals, `
Enter region: Invalid URI: "https://&%5abc"

Enter region: 
`[1:])
}

func (PollsterSuite) TestQueryArraySchema(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(w.String(), gc.Equals, `
Numbers
  one
  two
  three

Select one or more numbers separated by commas: 
`[1:])
	s, ok := v.([]string)
	c.Check(ok, jc.IsTrue)
	c.Check(s, jc.SameContents, []string{"one", "three"})
}

func (PollsterSuite) TestQueryEnum(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(w.String(), gc.Equals, `
Numbers
  1
  2
  3

Select number: 
`[1:])
	i, ok := v.(int)
	c.Check(ok, jc.IsTrue)
	c.Check(i, gc.Equals, 2)
}

func (PollsterSuite) TestQueryObjectSchema(c *gc.C) {
	schema := &jsonschema.Schema{
		Type: []jsonschema.Type{jsonschema.ObjectType},
		Properties: map[string]*jsonschema.Schema{
			"numbers": &jsonschema.Schema{
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
			"name": &jsonschema.Schema{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, jc.DeepEquals, map[string]interface{}{
		"name":    "Bill",
		"numbers": []string{"two", "three"},
	})
}

func (PollsterSuite) TestQueryObjectSchemaOrder(c *gc.C) {
	schema := &jsonschema.Schema{
		Type: []jsonschema.Type{jsonschema.ObjectType},
		// Order should match up with order of input in strings.NewReader below.
		Order: []string{"numbers", "name"},
		Properties: map[string]*jsonschema.Schema{
			"numbers": &jsonschema.Schema{
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
			"name": &jsonschema.Schema{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, jc.DeepEquals, map[string]interface{}{
		"name":    "Bill",
		"numbers": []string{"two", "three"},
	})
}

func (PollsterSuite) TestQueryObjectSchemaAdditional(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, jc.DeepEquals, map[string]interface{}{
		"one": map[string]interface{}{"loc": "east"},
		"two": map[string]interface{}{"loc": "west"},
	})
	c.Check(w.String(), gc.Equals, `
Enter region name: 
Enter location: 
Enter another region? (Y/n): 
Enter region name: 
Enter location: 
Enter another region? (Y/n): 
`[1:])
}

func (PollsterSuite) TestQueryObjectSchemaAdditionalEmpty(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(v, jc.DeepEquals, map[string]interface{}{
		"one": map[string]interface{}{},
		"two": map[string]interface{}{},
	})
	c.Check(w.String(), gc.Equals, `
Enter region name: 
Enter another region? (Y/n): 
Enter region name: 
Enter another region? (Y/n): 
`[1:])
}
