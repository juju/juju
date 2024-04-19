// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type dummy struct {
	val string
}

type querySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&querySuite{})

func (s *querySuite) TestIn(c *gc.C) {
	v := dummy{val: "whatever"}

	in, out, samples := runProcess(In(v))

	c.Check(in, gc.DeepEquals, []any{v})
	c.Check(out, gc.IsNil)
	c.Check(samples, gc.DeepEquals, []any{v})
}

func (s *querySuite) TestOut(c *gc.C) {
	var v dummy

	in, out, samples := runProcess(Out(&v))

	c.Check(in, gc.IsNil)
	c.Check(out, gc.DeepEquals, []any{&v})
	c.Check(samples, gc.DeepEquals, []any{v})
}

func (s *querySuite) TestOutM(c *gc.C) {
	var v []dummy

	in, out, samples := runProcess(OutM(&v))

	c.Check(in, gc.IsNil)
	c.Check(out, gc.DeepEquals, []any{&v})
	c.Check(samples, gc.DeepEquals, []any{dummy{}})
}

func runProcess(f processFunc) ([]any, []any, []any) {
	var in, out, samples []any
	return f(in, out, samples)
}
