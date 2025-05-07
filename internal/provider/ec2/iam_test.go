// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/ec2/internal/testing"
)

type IAMSuite struct {
	server *testing.IAMServer
}

var _ = tc.Suite(&IAMSuite{})

func (i *IAMSuite) SetUpTest(c *tc.C) {
	server, err := testing.NewIAMServer()
	c.Assert(err, tc.ErrorIsNil)
	i.server = server
}

func (i *IAMSuite) TestEnsureControllerInstanceProfileFromScratch(c *tc.C) {
	ip, _, err := ensureControllerInstanceProfile(context.Background(), i.server, "test", "AABBCC")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*ip.InstanceProfileName, tc.Equals, "juju-controller-test")
	c.Assert(*ip.Path, tc.Equals, "/juju/controller/AABBCC/")

	roleOutput, err := i.server.GetRole(context.Background(), &iam.GetRoleInput{
		RoleName: aws.String("juju-controller-test"),
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*roleOutput.Role.RoleName, tc.Equals, "juju-controller-test")
}

func (i *IAMSuite) TestEnsureControllerInstanceProfileAlreadyExists(c *tc.C) {
	_, err := i.server.CreateInstanceProfile(context.Background(), &iam.CreateInstanceProfileInput{
		InstanceProfileName: aws.String("juju-controller-test"),
	})
	c.Assert(err, tc.ErrorIsNil)

	instanceProfile, _, err := ensureControllerInstanceProfile(context.Background(), i.server, "test", "ABCD")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*instanceProfile.InstanceProfileName, tc.Equals, "juju-controller-test")
}

func (i *IAMSuite) TestFindInstanceProfileExists(c *tc.C) {
	_, err := i.server.CreateInstanceProfile(context.Background(), &iam.CreateInstanceProfileInput{
		InstanceProfileName: aws.String("juju-controller-test"),
	})
	c.Assert(err, tc.ErrorIsNil)

	instanceProfile, err := findInstanceProfileFromName(context.Background(), i.server, "juju-controller-test")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*instanceProfile.InstanceProfileName, tc.Equals, "juju-controller-test")
}

func (i *IAMSuite) TestFindInstanceProfileWithNotFoundError(c *tc.C) {
	instanceProfile, err := findInstanceProfileFromName(context.Background(), i.server, "test")
	c.Assert(instanceProfile, tc.IsNil)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (i *IAMSuite) TestFindRoleExists(c *tc.C) {
	_, err := i.server.CreateRole(context.Background(), &iam.CreateRoleInput{
		RoleName:    aws.String("test-role"),
		Description: aws.String("test-description"),
	})
	c.Assert(err, tc.ErrorIsNil)

	role, err := findRoleFromName(context.Background(), i.server, "test-role")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*role.RoleName, tc.Equals, "test-role")
}
