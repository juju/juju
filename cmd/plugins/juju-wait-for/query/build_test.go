// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type buildSuite struct{}

var _ = gc.Suite(&buildSuite{})

func (p *buildSuite) TestLogicalAND(c *gc.C) {
	a := Equality("a", "1")
	b := Equality("b", "2")

	var builders Builders
	builders = append(builders, a)
	builders = append(builders, b)

	builder := builders.LogicalAND()

	result, err := builder.Build("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, `a == "1" && b == "2"`)
}

func (p *buildSuite) TestLogicalANDWithPrefix(c *gc.C) {
	a := Equality("a", "1")
	b := Equality("b", "2")

	var builders Builders
	builders = append(builders, a)
	builders = append(builders, b)

	builder := builders.LogicalAND()

	result, err := builder.Build("x")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, `x.a == "1" && x.b == "2"`)
}

func (p *buildSuite) TestForEach(c *gc.C) {
	a := Equality("a", "1")

	var builders Builders
	builder := builders.ForEach("x", "y", func() (Builder, error) {
		return a, nil
	})

	result, err := builder.Build("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, `forEach(x, y => y.a == "1")`)
}

func (p *buildSuite) TestMultipleBuilders(c *gc.C) {
	a := Equality("a", "1")
	b := Equality("b", "2")

	var builders Builders
	builders = append(builders, a)
	builders = append(builders, b)
	builders = append(builders, builders.ForEach("x", "y", func() (Builder, error) {
		return a, nil
	}))

	builder := builders.LogicalAND()

	result, err := builder.Build("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, `a == "1" && b == "2" && forEach(x, y => y.a == "1")`)
}
