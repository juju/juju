// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/action"
)

type CommonSuite struct{}

func TestCommonSuite(t *testing.T) {
	tc.Run(t, &CommonSuite{})
}

type insertSliceValue struct {
	valuePath []string
	value     any
}

func (s *CommonSuite) TestAddValueToMap(c *tc.C) {
	for i, t := range []struct {
		should       string
		startingMap  map[string]any
		insertSlices []insertSliceValue
		expectedMap  map[string]any
	}{{
		should: "insert a couple of values",
		startingMap: map[string]any{
			"foo": "bar",
			"bar": map[string]any{
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
		expectedMap: map[string]any{
			"foo": "kek",
			"bar": map[string]any{
				"baz": "bo",
				"bur": "bor",
			},
			"well": map[string]any{
				"now": 5,
			},
		},
	}} {
		c.Logf("test %d: should %s", i, t.should)
		for _, sVal := range t.insertSlices {
			action.AddValueToMap(sVal.valuePath, sVal.value, t.startingMap)
		}
		// note addValueToMap mutates target.
		c.Check(t.startingMap, tc.DeepEquals, t.expectedMap)
	}
}
