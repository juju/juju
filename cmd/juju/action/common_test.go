// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cmd/juju/action"
)

type CommonSuite struct{}

var _ = tc.Suite(&CommonSuite{})

type insertSliceValue struct {
	valuePath []string
	value     interface{}
}

func (s *CommonSuite) TestAddValueToMap(c *tc.C) {
	for i, t := range []struct {
		should       string
		startingMap  map[string]interface{}
		insertSlices []insertSliceValue
		expectedMap  map[string]interface{}
	}{{
		should: "insert a couple of values",
		startingMap: map[string]interface{}{
			"foo": "bar",
			"bar": map[string]interface{}{
				"baz": "bo",
				"bur": "bor",
			},
		},
		insertSlices: []insertSliceValue{
			{
				valuePath: []string{"well", "now"},
				value:     5,
			},
			{
				valuePath: []string{"foo"},
				value:     "kek",
			},
		},
		expectedMap: map[string]interface{}{
			"foo": "kek",
			"bar": map[string]interface{}{
				"baz": "bo",
				"bur": "bor",
			},
			"well": map[string]interface{}{
				"now": 5,
			},
		},
	}} {
		c.Logf("test %d: should %s", i, t.should)
		for _, sVal := range t.insertSlices {
			action.AddValueToMap(sVal.valuePath, sVal.value, t.startingMap)
		}
		// note addValueToMap mutates target.
		c.Check(t.startingMap, jc.DeepEquals, t.expectedMap)
	}
}
