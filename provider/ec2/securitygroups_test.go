// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	stdcontext "context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/environs/context"
	"github.com/juju/juju/v2/provider/ec2"
	coretesting "github.com/juju/juju/v2/testing"
)

type SecurityGroupSuite struct {
	coretesting.BaseSuite

	clientStub   *stubClient
	deleteFunc   func(ec2.SecurityGroupCleaner, context.ProviderCallContext, types.GroupIdentifier, clock.Clock) error
	cloudCallCtx context.ProviderCallContext
}

var _ = gc.Suite(&SecurityGroupSuite{})

func (s *SecurityGroupSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.deleteFunc = *ec2.DeleteSecurityGroupInsistently
}

func (s *SecurityGroupSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.clientStub = &stubClient{
		Stub: &testing.Stub{},
		deleteSecurityGroup: func(group types.GroupIdentifier) (resp *awsec2.DeleteSecurityGroupOutput, err error) {
			return nil, nil
		},
	}
	s.cloudCallCtx = context.NewEmptyCloudCallContext()
}

func (s *SecurityGroupSuite) TestDeleteSecurityGroupSuccess(c *gc.C) {
	err := s.deleteFunc(s.clientStub, s.cloudCallCtx, types.GroupIdentifier{}, testclock.NewClock(time.Time{}))
	c.Assert(err, jc.ErrorIsNil)
	s.clientStub.CheckCallNames(c, "DeleteSecurityGroup")
}

func (s *SecurityGroupSuite) TestDeleteSecurityGroupInvalidGroupNotFound(c *gc.C) {
	s.clientStub.deleteSecurityGroup = func(group types.GroupIdentifier) (resp *awsec2.DeleteSecurityGroupOutput, err error) {
		return nil, &smithy.GenericAPIError{Code: "InvalidGroup.NotFound"}
	}
	err := s.deleteFunc(s.clientStub, s.cloudCallCtx, types.GroupIdentifier{}, testclock.NewClock(time.Time{}))
	c.Assert(err, jc.ErrorIsNil)
	s.clientStub.CheckCallNames(c, "DeleteSecurityGroup")
}

func (s *SecurityGroupSuite) TestDeleteSecurityGroupFewCalls(c *gc.C) {
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
		c.Assert(clock.Now(), gc.Equals, expectedTimes[count])
		if count < maxCalls {
			count++
			return nil, &smithy.GenericAPIError{Code: "keep going"}
		}
		return nil, nil
	}
	err := s.deleteFunc(s.clientStub, s.cloudCallCtx, types.GroupIdentifier{}, clock)
	c.Assert(err, jc.ErrorIsNil)

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
	*testing.Stub
	deleteSecurityGroup func(group types.GroupIdentifier) (*awsec2.DeleteSecurityGroupOutput, error)
}

func (s *stubClient) DeleteSecurityGroup(ctx stdcontext.Context, input *awsec2.DeleteSecurityGroupInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteSecurityGroupOutput, error) {
	s.MethodCall(s, "DeleteSecurityGroup", aws.ToString(input.GroupId), aws.ToString(input.GroupName))
	return s.deleteSecurityGroup(types.GroupIdentifier{
		GroupId:   input.GroupId,
		GroupName: input.GroupName,
	})
}
