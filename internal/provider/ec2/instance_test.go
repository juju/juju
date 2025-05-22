// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/juju/tc"
)

type fetchInstanceClientFunc func(context.Context, *ec2.DescribeInstanceTypesInput, ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error)

type instanceSuite struct{}

func TestInstanceSuite(t *testing.T) {
	tc.Run(t, &instanceSuite{})
}

func (f fetchInstanceClientFunc) DescribeInstanceTypes(
	c context.Context,
	i *ec2.DescribeInstanceTypesInput,
	o ...func(*ec2.Options),
) (*ec2.DescribeInstanceTypesOutput, error) {
	return f(c, i, o...)
}

func (s *instanceSuite) TestFetchInstanceTypeInfoPagnation(c *tc.C) {
	callCount := 0
	client := func(
		_ context.Context,
		i *ec2.DescribeInstanceTypesInput,
		o ...func(*ec2.Options),
	) (*ec2.DescribeInstanceTypesOutput, error) {
		if callCount != 0 {
			c.Assert(*i.NextToken, tc.Equals, "next")
		}
		c.Assert(*i.MaxResults, tc.Equals, int32(100))

		callCount++
		nextToken := aws.String("next")
		// Let 6 calls happen
		if callCount == 6 {
			nextToken = nil
		}

		return &ec2.DescribeInstanceTypesOutput{
			InstanceTypes: make([]types.InstanceTypeInfo, 100),
			NextToken:     nextToken,
		}, nil
	}

	res, err := FetchInstanceTypeInfo(
		c.Context(),
		fetchInstanceClientFunc(client),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(res), tc.Equals, 600)
}
