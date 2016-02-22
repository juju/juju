// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&FlagSuite{})

type FlagSuite struct {
	testing.IsolationSuite
}

func (FlagSuite) TestStringMapNilOk(c *gc.C) {
	// note that the map may start out nil
	var values map[string]string
	c.Assert(values, gc.IsNil)
	sm := stringMap{&values}
	err := sm.Set("foo=foovalue")
	c.Assert(err, jc.ErrorIsNil)
	err = sm.Set("bar=barvalue")
	c.Assert(err, jc.ErrorIsNil)

	// now the map is non-nil and filled
	c.Assert(values, gc.DeepEquals, map[string]string{
		"foo": "foovalue",
		"bar": "barvalue",
	})
}

func (FlagSuite) TestStringMapBadVal(c *gc.C) {
	sm := stringMap{&map[string]string{}}
	err := sm.Set("foo")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(err, gc.ErrorMatches, "badly formatted name value pair: foo")
}

func (FlagSuite) TestStringMapDupVal(c *gc.C) {
	sm := stringMap{&map[string]string{}}
	err := sm.Set("bar=somevalue")
	c.Assert(err, jc.ErrorIsNil)
	err = sm.Set("bar=someothervalue")
	c.Assert(err, gc.ErrorMatches, ".*duplicate.*bar.*")
}
