// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package cmd_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/cmd"
)

var _ = gc.Suite(&StringMapSuite{})

type StringMapSuite struct {
	testing.IsolationSuite
}

func (StringMapSuite) TestStringMapNilOk(c *gc.C) {
	// note that the map may start out nil
	var values map[string]string
	c.Assert(values, gc.IsNil)
	sm := cmd.StringMap{Mapping: &values}
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

func (StringMapSuite) TestStringMapBadVal(c *gc.C) {
	sm := cmd.StringMap{Mapping: &map[string]string{}}
	err := sm.Set("foo")
	c.Assert(err, gc.ErrorMatches, "expected key=value format")
}

func (StringMapSuite) TestStringMapDupVal(c *gc.C) {
	sm := cmd.StringMap{Mapping: &map[string]string{}}
	err := sm.Set("bar=somevalue")
	c.Assert(err, jc.ErrorIsNil)
	err = sm.Set("bar=someothervalue")
	c.Assert(err, gc.ErrorMatches, "duplicate key specified")
}

func (StringMapSuite) TestStringMapNoValue(c *gc.C) {
	sm := cmd.StringMap{Mapping: &map[string]string{}}
	err := sm.Set("bar=")
	c.Assert(err, gc.ErrorMatches, "key and value must be non-empty")
}

func (StringMapSuite) TestStringMapNoKey(c *gc.C) {
	sm := cmd.StringMap{Mapping: &map[string]string{}}
	err := sm.Set("=bar")
	c.Assert(err, gc.ErrorMatches, "key and value must be non-empty")
}
