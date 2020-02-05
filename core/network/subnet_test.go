// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type subnetSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&subnetSuite{})

func (*subnetSuite) TestFindSubnetIDsForAZ(c *gc.C) {
	testCases := []struct {
		name           string
		zoneName       string
		subnetsToZones map[network.Id][]string
		expected       []string
		expectedErr    func(error) bool
	}{
		{
			name:           "empty",
			zoneName:       "",
			subnetsToZones: make(map[network.Id][]string),
			expected:       make([]string, 0),
			expectedErr:    errors.IsNotFound,
		},
		{
			name:     "no match",
			zoneName: "fuzz",
			subnetsToZones: map[network.Id][]string{
				"bar": []string{"foo", "baz"},
			},
			expected:    make([]string, 0),
			expectedErr: errors.IsNotFound,
		},
		{
			name:     "match",
			zoneName: "foo",
			subnetsToZones: map[network.Id][]string{
				"bar": []string{"foo", "baz"},
			},
			expected: []string{"bar"},
		},
		{
			name:     "multi-match",
			zoneName: "foo",
			subnetsToZones: map[network.Id][]string{
				"bar":   []string{"foo", "baz"},
				"other": []string{"aaa", "foo", "xxx"},
			},
			expected: []string{"bar", "other"},
		},
	}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.name)

		res, err := network.FindSubnetIDsForAvailabilityZone(t.zoneName, t.subnetsToZones)
		if t.expectedErr != nil {
			c.Check(t.expectedErr(err), jc.IsTrue)
		} else {
			c.Assert(err, gc.IsNil)
			c.Check(res, gc.DeepEquals, t.expected)
		}
	}
}
