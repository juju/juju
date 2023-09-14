// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/ec2/internal/testing"
)

type IAMSuite struct {
	server *testing.IAMServer
}

var _ = gc.Suite(&IAMSuite{})

func (i *IAMSuite) SetUpTest(c *gc.C) {
	server, err := testing.NewIAMServer()
	c.Assert(err, jc.ErrorIsNil)
	i.server = server
}

func (i *IAMSuite) TestEnsureControllerInstanceProfileFromScratch(c *gc.C) {
	ip, _, err := ensureControllerInstanceProfile(context.Background(), i.server, "test", "AABBCC")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*ip.InstanceProfileName, gc.Equals, "juju-controller-test")
	c.Assert(*ip.Path, gc.Equals, "/juju/controller/AABBCC/")

	roleOutput, err := i.server.GetRole(context.Background(), &iam.GetRoleInput{
		RoleName: aws.String("juju-controller-test"),
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*roleOutput.Role.RoleName, gc.Equals, "juju-controller-test")
}

func (i *IAMSuite) TestEnsureControllerInstanceProfileAlreadyExists(c *gc.C) {
	_, err := i.server.CreateInstanceProfile(context.Background(), &iam.CreateInstanceProfileInput{
		InstanceProfileName: aws.String("juju-controller-test"),
	})
	c.Assert(err, jc.ErrorIsNil)

	instanceProfile, _, err := ensureControllerInstanceProfile(context.Background(), i.server, "test", "ABCD")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*instanceProfile.InstanceProfileName, gc.Equals, "juju-controller-test")
}

func (i *IAMSuite) TestFindInstanceProfileExists(c *gc.C) {
	_, err := i.server.CreateInstanceProfile(context.Background(), &iam.CreateInstanceProfileInput{
		InstanceProfileName: aws.String("juju-controller-test"),
	})
	c.Assert(err, jc.ErrorIsNil)

	instanceProfile, err := findInstanceProfileFromName(context.Background(), i.server, "juju-controller-test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*instanceProfile.InstanceProfileName, gc.Equals, "juju-controller-test")
}

func (i *IAMSuite) TestFindInstanceProfileWithNotFoundError(c *gc.C) {
	instanceProfile, err := findInstanceProfileFromName(context.Background(), i.server, "test")
	c.Assert(instanceProfile, gc.IsNil)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (i *IAMSuite) TestFindRoleExists(c *gc.C) {
	_, err := i.server.CreateRole(context.Background(), &iam.CreateRoleInput{
		RoleName:    aws.String("test-role"),
		Description: aws.String("test-description"),
	})
	c.Assert(err, jc.ErrorIsNil)

	role, err := findRoleFromName(context.Background(), i.server, "test-role")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*role.RoleName, gc.Equals, "test-role")
}
