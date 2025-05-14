// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/ec2"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type SecurityGroupSuite struct {
	coretesting.BaseSuite

	clientStub *stubClient
	deleteFunc func(context.Context, ec2.SecurityGroupCleaner, types.GroupIdentifier, clock.Clock) error
}

var _ = tc.Suite(&SecurityGroupSuite{})

func (s *SecurityGroupSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.deleteFunc = *ec2.DeleteSecurityGroupInsistently
}

func (s *SecurityGroupSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.clientStub = &stubClient{
		Stub: &testhelpers.Stub{},
		deleteSecurityGroup: func(group types.GroupIdentifier) (resp *awsec2.DeleteSecurityGroupOutput, err error) {
			return nil, nil
		},
	}
}

func (s *SecurityGroupSuite) TestDeleteSecurityGroupSuccess(c *tc.C) {
	err := s.deleteFunc(c.Context(), s.clientStub, types.GroupIdentifier{}, testclock.NewClock(time.Time{}))
	c.Assert(err, tc.ErrorIsNil)
	s.clientStub.CheckCallNames(c, "DeleteSecurityGroup")
}

func (s *SecurityGroupSuite) TestDeleteSecurityGroupInvalidGroupNotFound(c *tc.C) {
	s.clientStub.deleteSecurityGroup = func(group types.GroupIdentifier) (resp *awsec2.DeleteSecurityGroupOutput, err error) {
		return nil, &smithy.GenericAPIError{Code: "InvalidGroup.NotFound"}
	}
	err := s.deleteFunc(c.Context(), s.clientStub, types.GroupIdentifier{}, testclock.NewClock(time.Time{}))
	c.Assert(err, tc.ErrorIsNil)
	s.clientStub.CheckCallNames(c, "DeleteSecurityGroup")
}

func (s *SecurityGroupSuite) TestDeleteSecurityGroupFewCalls(c *tc.C) {
	t0 := time.Time{}
	clock := autoAdvancingClock{testclock.NewClock(t0)}
	count := 0
	maxCalls := 4
	expectedTimes := []time.Time{
		t0,
		t0.Add(time.Second),
		t0.Add(3 * time.Second),
		t0.Add(7 * time.Second),
		t0.Add(15 * time.Second),
	}
	s.clientStub.deleteSecurityGroup = func(group types.GroupIdentifier) (resp *awsec2.DeleteSecurityGroupOutput, err error) {
		c.Assert(clock.Now(), tc.Equals, expectedTimes[count])
		if count < maxCalls {
			count++
			return nil, &smithy.GenericAPIError{Code: "keep going"}
		}
		return nil, nil
	}
	err := s.deleteFunc(c.Context(), s.clientStub, types.GroupIdentifier{}, clock)
	c.Assert(err, tc.ErrorIsNil)

	expectedCalls := make([]string, maxCalls+1)
	for i := 0; i < maxCalls+1; i++ {
		expectedCalls[i] = "DeleteSecurityGroup"
	}
	s.clientStub.CheckCallNames(c, expectedCalls...)
}

type autoAdvancingClock struct {
	*testclock.Clock
}

func (c autoAdvancingClock) After(d time.Duration) <-chan time.Time {
	ch := c.Clock.After(d)
	c.Advance(d)
	return ch
}

type stubClient struct {
	*testhelpers.Stub
	deleteSecurityGroup func(group types.GroupIdentifier) (*awsec2.DeleteSecurityGroupOutput, error)
}

func (s *stubClient) DeleteSecurityGroup(ctx context.Context, input *awsec2.DeleteSecurityGroupInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteSecurityGroupOutput, error) {
	s.MethodCall(s, "DeleteSecurityGroup", aws.ToString(input.GroupId), aws.ToString(input.GroupName))
	return s.deleteSecurityGroup(types.GroupIdentifier{
		GroupId:   input.GroupId,
		GroupName: input.GroupName,
	})
}
