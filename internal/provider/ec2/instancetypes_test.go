// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"github.com/juju/testing"
)

type InstanceTypesSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&InstanceTypesSuite{})

func (s *InstanceTypesSuite) TestParseInstanceType(c *tc.C) {
	tests := []struct {
		InstType types.InstanceType
		Expected instanceType
	}{
		{
			"m5.large",
			instanceType{
				Capabilities:    set.NewStrings(),
				Generation:      5,
				Family:          "m",
				ProcessorFamily: "i",
				Size:            "large",
			},
		},
		{
			"m5a.large",
			instanceType{
				Capabilities:    set.NewStrings(),
				Generation:      5,
				Family:          "m",
				ProcessorFamily: "a",
				Size:            "large",
			},
		},
		{
			"m5a.2xlarge",
			instanceType{
				Capabilities:    set.NewStrings(),
				Generation:      5,
				Family:          "m",
				ProcessorFamily: "a",
				Size:            "2xlarge",
			},
		},
		{
			"m5ad.large",
			instanceType{
				Capabilities:    set.NewStrings("d"),
				Generation:      5,
				Family:          "m",
				ProcessorFamily: "a",
				Size:            "large",
			},
		},
		{
			"m6g.24xlarge",
			instanceType{
				Capabilities:    set.NewStrings(),
				Generation:      6,
				Family:          "m",
				ProcessorFamily: "g",
				Size:            "24xlarge",
			},
		},
		{
			"m7gd.large",
			instanceType{
				Capabilities:    set.NewStrings("d"),
				Generation:      7,
				Family:          "m",
				ProcessorFamily: "g",
				Size:            "large",
			},
		},
		{
			"c5ad.large",
			instanceType{
				Capabilities:    set.NewStrings("d"),
				Generation:      5,
				Family:          "c",
				ProcessorFamily: "a",
				Size:            "large",
			},
		},
		{
			"r5a.metal",
			instanceType{
				Capabilities:    set.NewStrings(),
				Generation:      5,
				Family:          "r",
				ProcessorFamily: "a",
				Size:            "metal",
			},
		},
		{
			"Im4gn.large",
			instanceType{
				Capabilities:    set.NewStrings("n"),
				Generation:      4,
				Family:          "Im",
				ProcessorFamily: "g",
				Size:            "large",
			},
		},
		{
			"c4.large",
			instanceType{
				Capabilities:    set.NewStrings(),
				Generation:      4,
				Family:          "c",
				ProcessorFamily: "i",
				Size:            "large",
			},
		},
		{
			"mac2.metal",
			instanceType{
				Capabilities:    set.NewStrings(),
				Generation:      2,
				Family:          "mac",
				ProcessorFamily: "i",
				Size:            "metal",
			},
		},
	}

	for _, test := range tests {
		it, err := parseInstanceType(test.InstType)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(it, tc.DeepEquals, test.Expected)
	}
}
