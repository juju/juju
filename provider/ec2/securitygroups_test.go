// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
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

func (s *SecurityGroupSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.instanceStub = &stubInstance{
		Stub: &testing.Stub{},
		deleteSecurityGroup: func(group amzec2.SecurityGroup) (resp *amzec2.SimpleResp, err error) {
			return nil, nil
		},
	}
}

func (s *SecurityGroupSuite) TestDeleteSecurityGroupSuccess(c *gc.C) {
	err := s.deleteFunc(s.instanceStub, amzec2.SecurityGroup{})
	c.Assert(err, jc.ErrorIsNil)
	s.instanceStub.CheckCallNames(c, "DeleteSecurityGroup")
}

func (s *SecurityGroupSuite) TestDeleteSecurityGroupInvalidGroupNotFound(c *gc.C) {
	s.instanceStub.deleteSecurityGroup = func(group amzec2.SecurityGroup) (resp *amzec2.SimpleResp, err error) {
		return nil, &amzec2.Error{Code: "InvalidGroup.NotFound"}
	}
	err := s.deleteFunc(s.instanceStub, amzec2.SecurityGroup{})
	c.Assert(err, jc.ErrorIsNil)
	s.instanceStub.CheckCallNames(c, "DeleteSecurityGroup")
}

func (s *SecurityGroupSuite) TestDeleteSecurityGroupFewCalls(c *gc.C) {
	count := 0
	maxCalls := 4
	s.instanceStub.deleteSecurityGroup = func(group amzec2.SecurityGroup) (resp *amzec2.SimpleResp, err error) {
		if count < maxCalls {
			count++
			return nil, &amzec2.Error{Code: "keep going"}
		}
		return nil, nil
	}
	err := s.deleteFunc(s.instanceStub, amzec2.SecurityGroup{})
	c.Assert(err, jc.ErrorIsNil)

	expectedCalls := make([]string, maxCalls+1)
	for i := 0; i < maxCalls+1; i++ {
		expectedCalls[i] = "DeleteSecurityGroup"
	}
	s.instanceStub.CheckCallNames(c, expectedCalls...)
}

type stubInstance struct {
	*testing.Stub
	deleteSecurityGroup func(group amzec2.SecurityGroup) (resp *amzec2.SimpleResp, err error)
}

func (s *stubInstance) DeleteSecurityGroup(group amzec2.SecurityGroup) (resp *amzec2.SimpleResp, err error) {
	s.MethodCall(s, "DeleteSecurityGroup", group)
	return s.deleteSecurityGroup(group)
}
