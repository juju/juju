// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type utilsSuite struct {
}

var _ = gc.Suite(&utilsSuite{})

func (*utilsSuite) TestFlattenOneArray(c *gc.C) {
	given := [][]string{{"a", "b", "c"}}
	expected := []string{"a", "b", "c"}
	flattened := Flatten(given)

	c.Check(flattened, jc.SameContents, expected)
}

func (*utilsSuite) TestFlattenTwoArrays(c *gc.C) {
	given := [][]string{{"a", "b"}, {"c"}}
	expected := []string{"a", "b", "c"}
	flattened := Flatten(given)

	c.Check(flattened, jc.SameContents, expected)
}

func (*utilsSuite) TestFlattenEmpty(c *gc.C) {
	given := [][]string{}
	expected := []string{}
	flattened := Flatten(given)

	c.Check(flattened, jc.SameContents, expected)
}

func (*utilsSuite) TestFlattenNestedEmpty(c *gc.C) {
	given := [][]string{{}}
	expected := []string{}
	flattened := Flatten(given)

	c.Check(flattened, jc.SameContents, expected)
}
