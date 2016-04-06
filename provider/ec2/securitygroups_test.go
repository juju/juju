// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	amzec2 "gopkg.in/amz.v3/ec2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/ec2"
	coretesting "github.com/juju/juju/testing"
)

type SecurityGroupSuite struct {
	coretesting.BaseSuite

	instanceStub *stubInstance
	deleteFunc   func(inst ec2.SecurityGroupCleaner, group amzec2.SecurityGroup) error
}

var _ = gc.Suite(&SecurityGroupSuite{})

func (s *SecurityGroupSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.deleteFunc = *ec2.DeleteSecurityGroupInsistently
}

func (s *SecurityGroupSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *SecurityGroupSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.instanceStub = &stubInstance{
		Stub: &testing.Stub{},
		deleteSecurityGroup: func(group amzec2.SecurityGroup) (resp *amzec2.SimpleResp, err error) {
			return nil, nil
		},
	}
}

func (s *SecurityGroupSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
}

func (s *SecurityGroupSuite) TestDeleteSecurityGroupSuccess(c *gc.C) {
	err := s.deleteFunc(s.instanceStub, amzec2.SecurityGroup{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SecurityGroupSuite) TestDeleteSecurityGroupRequestExceedsRate(c *gc.C) {
	callCount := 0
	errMsg := "my message"
	s.instanceStub.deleteSecurityGroup = func(group amzec2.SecurityGroup) (resp *amzec2.SimpleResp, err error) {
		if callCount == 0 {
			callCount++
			return nil, &amzec2.Error{Code: errMsg}
		}
		return nil, &amzec2.Error{Code: "RequestLimitExceeded"}
	}
	err := s.deleteFunc(s.instanceStub, amzec2.SecurityGroup{})
	c.Assert(err, gc.ErrorMatches, ec2LikeErrorString(errMsg))
}

func (s *SecurityGroupSuite) TestDeleteSecurityGroupExponentialRetry(c *gc.C) {
	callCount := 0
	maxCalls := 5
	differencesCount := maxCalls - 1
	errMsg := "my message"

	timestamps := make([]time.Time, maxCalls)
	s.instanceStub.deleteSecurityGroup = func(group amzec2.SecurityGroup) (resp *amzec2.SimpleResp, err error) {
		if callCount < maxCalls {
			timestamps[callCount] = time.Now()
			callCount++
			return nil, &amzec2.Error{Code: errMsg}
		}
		return nil, nil
	}
	err := s.deleteFunc(s.instanceStub, amzec2.SecurityGroup{})
	c.Assert(err, jc.ErrorIsNil)

	// difference between calls should double
	differences := make([]time.Duration, differencesCount)
	for i := 0; i < differencesCount; i++ {
		differences[i] = timestamps[i+1].Sub(timestamps[i])
	}

	// Since retries are measured in seconds,
	// we expect each consequent delay double in duration
	for i := 0; i < differencesCount-1; i++ {
		c.Assert(math.Trunc(differences[i+1].Seconds()), gc.Equals, math.Trunc(differences[i].Seconds())*2)
	}
}

func ec2LikeErrorString(msg string) string {
	return regexp.QuoteMeta(fmt.Sprintf(" (%s)", msg))
}

type stubInstance struct {
	*testing.Stub
	deleteSecurityGroup func(group amzec2.SecurityGroup) (resp *amzec2.SimpleResp, err error)
}

func (s *stubInstance) DeleteSecurityGroup(group amzec2.SecurityGroup) (resp *amzec2.SimpleResp, err error) {
	s.MethodCall(s, "DeleteSecurityGroup", group)
	return s.deleteSecurityGroup(group)
}
