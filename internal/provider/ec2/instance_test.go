// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type fetchInstanceClientFunc func(context.Context, *ec2.DescribeInstanceTypesInput, ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error)

type instanceSuite struct{}

var _ = gc.Suite(&instanceSuite{})

func (f fetchInstanceClientFunc) DescribeInstanceTypes(
	c context.Context,
	i *ec2.DescribeInstanceTypesInput,
	o ...func(*ec2.Options),
) (*ec2.DescribeInstanceTypesOutput, error) {
	return f(c, i, o...)
}

func (s *instanceSuite) TestFetchInstanceTypeInfoPagnation(c *gc.C) {
	callCount := 0
	client := func(
		_ context.Context,
		i *ec2.DescribeInstanceTypesInput,
		o ...func(*ec2.Options),
	) (*ec2.DescribeInstanceTypesOutput, error) {
		if callCount != 0 {
			c.Assert(*i.NextToken, gc.Equals, "next")
		}
		c.Assert(*i.MaxResults, gc.Equals, int32(100))

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
		context.Background(),
		fetchInstanceClientFunc(client),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(res), gc.Equals, 600)
}
