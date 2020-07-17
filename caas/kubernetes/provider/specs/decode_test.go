// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	k8sspces "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/testing"
)

type decoderSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&decoderSuite{})

func (s *decoderSuite) TestYAMLOrJSONDecoder(c *gc.C) {
	type tS struct {
		Foo string `json:"foo,omitempty" yaml:"foo,omitempty"`
		Bar string `json:"bar,omitempty" yaml:"bar,omitempty"`
	}

	var in string
	var out tS

	in = `
foo: foo1
bar: bar1`
	// decode YAML in strict mode - good.
	decoder := k8sspces.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in), true)
	c.Assert(decoder.Decode(&out), jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, tS{
		Foo: "foo1",
		Bar: "bar1",
	})
	// decode YAML in non-strict mode - good.
	decoder = k8sspces.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in), false)
	c.Assert(decoder.Decode(&out), jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, tS{
		Foo: "foo1",
		Bar: "bar1",
	})

	in = `
{
    "foo": "foo1",
    "bar": "bar1"
}`
	// decode JSON in strict mode - good.
	decoder = k8sspces.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in), true)
	c.Assert(decoder.Decode(&out), jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, tS{
		Foo: "foo1",
		Bar: "bar1",
	})
	// decode JSON in non-strict mode - good.
	decoder = k8sspces.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in), false)
	c.Assert(decoder.Decode(&out), jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, tS{
		Foo: "foo1",
		Bar: "bar1",
	})

	in = `
foo: foo1
bar: bar1
unknownkey: ops`
	// decode YAML in strict mode - bad.
	decoder = k8sspces.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in), true)
	c.Assert(decoder.Decode(&out), gc.ErrorMatches, `json: unknown field "unknownkey"`)
	// decode YAML in non-strict mode - unknown field ignored.
	decoder = k8sspces.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in), false)
	c.Assert(decoder.Decode(&out), jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, tS{
		Foo: "foo1",
		Bar: "bar1",
	})

	in = `
{
    "foo": "foo1",
	"bar": "bar1",
	"unknownkey": "ops"
}`
	// decode JSON in strict mode - bad.
	decoder = k8sspces.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in), true)
	c.Assert(decoder.Decode(&out), gc.ErrorMatches, `json: unknown field "unknownkey"`)
	// decode JSON in non-strict mode - unknown field ignored.
	decoder = k8sspces.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in), false)
	c.Assert(decoder.Decode(&out), jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, tS{
		Foo: "foo1",
		Bar: "bar1",
	})

	in = `
{
    "foo": "foo1"
}
{
    "bar": "bar1"
}`
	// decode JSON in strict mode - good.
	decoder = k8sspces.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in), true)
	c.Assert(decoder.Decode(&out), jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, tS{
		Foo: "foo1",
		Bar: "bar1",
	})
	// decode JSON in non-strict mode - good.
	decoder = k8sspces.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in), false)
	c.Assert(decoder.Decode(&out), jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, tS{
		Foo: "foo1",
		Bar: "bar1",
	})

	in = `
{
    "foo": "foo1"
}
{
	"bar": "bar1",
	"unknownkey": "ops"
}`
	// decode JSON in strict mode - bad.
	decoder = k8sspces.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in), true)
	c.Assert(decoder.Decode(&out), gc.ErrorMatches, `json: unknown field "unknownkey"`)
	// decode JSON in non-strict mode - unknown field ignored.
	decoder = k8sspces.NewYAMLOrJSONDecoder(strings.NewReader(in), len(in), false)
	c.Assert(decoder.Decode(&out), jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, tS{
		Foo: "foo1",
		Bar: "bar1",
	})
}
