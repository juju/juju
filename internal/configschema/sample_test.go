// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package configschema_test

import (
	"bytes"
	"strings"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/configschema"
)

type sampleSuite struct{}

var _ = tc.Suite(&sampleSuite{})

var sampleYAMLTests = []struct {
	about  string
	indent int
	attrs  map[string]interface{}
	fields configschema.Fields
	expect string
}{{
	about: "simple values, all attributes specified", attrs: map[string]interface{}{
		"foo": "foovalue",
		"bar": 1243,
		"baz": false,
		"attrs": map[string]string{
			"arble": "bletch",
			"hello": "goodbye",
		},
	},
	fields: configschema.Fields{
		"foo": {
			Type:        configschema.Tstring,
			Description: "foo is a string.",
		},
		"bar": {
			Type:        configschema.Tint,
			Description: "bar is a number.\nWith a long description that contains newlines. And quite a bit more text that will be folded because it is longer than 80 characters.",
		},
		"baz": {
			Type:        configschema.Tbool,
			Description: "baz is a bool.",
		},
		"attrs": {
			Type:        configschema.Tattrs,
			Description: "attrs is an attribute list",
		},
		"list": {
			Type:        configschema.Tlist,
			Description: "list is a slice",
		},
	},
	expect: `
		|# attrs is an attribute list
		|#
		|attrs:
		|  arble: bletch
		|  hello: goodbye
		|
		|# bar is a number. With a long description that contains newlines. And quite a
		|# bit more text that will be folded because it is longer than 80 characters.
		|#
		|bar: 1243
		|
		|# baz is a bool.
		|#
		|baz: false
		|
		|# foo is a string.
		|#
		|foo: foovalue
		|
		|# list is a slice
		|#
		|# list:
		|#   - example
	`,
}, {
	about: "when a value is not specified, it's commented out",
	attrs: map[string]interface{}{
		"foo": "foovalue",
	},
	fields: configschema.Fields{
		"foo": {
			Type:        configschema.Tstring,
			Description: "foo is a string.",
		},
		"bar": {
			Type:        configschema.Tint,
			Description: "bar is a number.",
			Example:     1243,
		},
	},
	expect: `
		|# bar is a number.
		|#
		|# bar: 1243
		|
		|# foo is a string.
		|#
		|foo: foovalue
	`,
}, {
	about: "environment variables are mentioned as defaults",
	attrs: map[string]interface{}{
		"bar": 1324,
		"baz": true,
		"foo": "foovalue",
	},
	fields: configschema.Fields{
		"bar": {
			Type:        configschema.Tint,
			Description: "bar is a number.",
			EnvVars:     []string{"BAR_VAL", "ALT_BAR_VAL"},
		},
		"baz": {
			Type:        configschema.Tbool,
			Description: "baz is a bool.",
			EnvVar:      "BAZ_VAL",
			EnvVars:     []string{"ALT_BAZ_VAL", "ALT2_BAZ_VAL"},
		},
		"foo": {
			Type:        configschema.Tstring,
			Description: "foo is a string.",
			EnvVar:      "FOO_VAL",
		},
	},
	expect: `
		|# bar is a number.
		|#
		|# Default value taken from $BAR_VAL or $ALT_BAR_VAL.
		|#
		|bar: 1324
		|
		|# baz is a bool.
		|#
		|# Default value taken from $BAZ_VAL, $ALT_BAZ_VAL or $ALT2_BAZ_VAL.
		|#
		|baz: true
		|
		|# foo is a string.
		|#
		|# Default value taken from $FOO_VAL.
		|#
		|foo: foovalue
	`,
}, {
	about: "sorted by attribute group (provider, account, environ, other), then alphabetically",
	fields: configschema.Fields{
		"baz": {
			Type:        configschema.Tbool,
			Description: "baz is a bool.",
			Group:       configschema.ProviderGroup,
		},
		"zaphod": {
			Type:  configschema.Tstring,
			Group: configschema.ProviderGroup,
		},
		"bar": {
			Type:        configschema.Tint,
			Description: "bar is a number.",
			Group:       configschema.AccountGroup,
		},
		"foo": {
			Type:        configschema.Tstring,
			Description: "foo is a string.",
			Group:       configschema.AccountGroup,
		},
		"alpha": {
			Type:  configschema.Tstring,
			Group: configschema.EnvironGroup,
		},
		"bravo": {
			Type:  configschema.Tstring,
			Group: configschema.EnvironGroup,
		},
		"charlie": {
			Type:  configschema.Tstring,
			Group: "unknown",
		},
		"delta": {
			Type:  configschema.Tstring,
			Group: "unknown",
		},
	},
	expect: `
	|# baz is a bool.
	|#
	|# baz: false
	|
	|# zaphod: ""
	|
	|# bar is a number.
	|#
	|# bar: 0
	|
	|# foo is a string.
	|#
	|# foo: ""
	|
	|# alpha: ""
	|
	|# bravo: ""
	|
	|# charlie: ""
	|
	|# delta: ""
`,
}, {
	about: "example value is used when possible; zero value otherwise",
	fields: configschema.Fields{
		"intval-with-example": {
			Type:    configschema.Tint,
			Example: 999,
		},
		"intval": {
			Type: configschema.Tint,
		},
		"boolval": {
			Type: configschema.Tbool,
		},
		"attrsval": {
			Type: configschema.Tattrs,
		},
		"listval": {
			Type: configschema.Tlist,
		},
	},
	expect: `
		|# attrsval:
		|#   example: value
		|
		|# boolval: false
		|
		|# intval: 0
		|
		|# intval-with-example: 999
		|
		|# listval:
		|#   - example
	`,
}, {
	about: "secret values are marked as secret/immutable",
	fields: configschema.Fields{
		"a": {
			Type:        configschema.Tbool,
			Description: "With a description",
			Secret:      true,
		},
		"b": {
			Type:   configschema.Tstring,
			Secret: true,
		},
		"c": {
			Type:        configschema.Tstring,
			Secret:      true,
			Description: "With a description",
			EnvVar:      "VAR",
		},
		"d": {
			Type:      configschema.Tstring,
			Immutable: true,
		},
		"e": {
			Type:      configschema.Tstring,
			Immutable: true,
			Secret:    true,
		},
	},
	expect: `
		|# With a description
		|#
		|# This attribute is considered secret.
		|#
		|# a: false
		|
		|# This attribute is considered secret.
		|#
		|# b: ""
		|
		|# With a description
		|#
		|# Default value taken from $VAR.
		|#
		|# This attribute is considered secret.
		|#
		|# c: ""
		|
		|# This attribute is immutable.
		|#
		|# d: ""
		|
		|# This attribute is immutable and considered secret.
		|#
		|# e: ""
	`,
}}

func (sampleSuite) TestSampleYAML(c *tc.C) {
	for i, test := range sampleYAMLTests {
		c.Logf("test %d. %s\n", i, test.about)
		var buf bytes.Buffer
		err := configschema.SampleYAML(&buf, 0, test.attrs, test.fields)
		c.Assert(err, tc.IsNil)
		diff(c, buf.String(), unbeautify(test.expect[1:]))
	}
}

// indentReplacer deletes tabs and | beautifier characters.
var indentReplacer = strings.NewReplacer("\t", "", "|", "")

// unbeautify strips the leading tabs and | characters that
// we use to make the tests look nicer.
func unbeautify(s string) string {
	return indentReplacer.Replace(s)
}

func diff(c *tc.C, have, want string) {
	// Final sanity check in case the below logic is flawed.
	defer c.Check(have, tc.Equals, want)

	haveLines := strings.Split(have, "\n")
	wantLines := strings.Split(want, "\n")

	for i, wantLine := range wantLines {
		if i >= len(haveLines) {
			c.Errorf("have too few lines from line %d, %s", i+1, wantLine)
			return
		}
		haveLine := haveLines[i]
		c.Assert(haveLine, tc.Equals, wantLine, tc.Commentf("line %d", i+1))
	}
	if len(haveLines) > len(wantLines) {
		c.Errorf("have too many lines from line %d, %s", len(wantLines), haveLines[len(wantLines)])
		return
	}
}
