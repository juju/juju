// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	stdtesting "testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/ec2/internal/testing"
)

type IAMSuite struct {
	server *testing.IAMServer
}

func TestIAMSuite(t *stdtesting.T) { tc.Run(t, &IAMSuite{}) }
func (i *IAMSuite) SetUpTest(c *tc.C) {
	server, err := testing.NewIAMServer()
	c.Assert(err, tc.ErrorIsNil)
	i.server = server
}

func (i *IAMSuite) TestEnsureControllerInstanceProfileFromScratch(c *tc.C) {
	ip, _, err := ensureControllerInstanceProfile(c.Context(), i.server, "test", "AABBCC")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*ip.InstanceProfileName, tc.Equals, "juju-controller-test")
	c.Assert(*ip.Path, tc.Equals, "/juju/controller/AABBCC/")

	roleOutput, err := i.server.GetRole(c.Context(), &iam.GetRoleInput{
		RoleName: aws.String("juju-controller-test"),
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*roleOutput.Role.RoleName, tc.Equals, "juju-controller-test")
}

func (i *IAMSuite) TestEnsureControllerInstanceProfileAlreadyExists(c *tc.C) {
	_, err := i.server.CreateInstanceProfile(c.Context(), &iam.CreateInstanceProfileInput{
		InstanceProfileName: aws.String("juju-controller-test"),
	})
	c.Assert(err, tc.ErrorIsNil)

	instanceProfile, _, err := ensureControllerInstanceProfile(c.Context(), i.server, "test", "ABCD")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*instanceProfile.InstanceProfileName, tc.Equals, "juju-controller-test")
}

func (i *IAMSuite) TestFindInstanceProfileExists(c *tc.C) {
	_, err := i.server.CreateInstanceProfile(c.Context(), &iam.CreateInstanceProfileInput{
		InstanceProfileName: aws.String("juju-controller-test"),
	})
	c.Assert(err, tc.ErrorIsNil)

	instanceProfile, err := findInstanceProfileFromName(c.Context(), i.server, "juju-controller-test")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*instanceProfile.InstanceProfileName, tc.Equals, "juju-controller-test")
}

func (i *IAMSuite) TestFindInstanceProfileWithNotFoundError(c *tc.C) {
	instanceProfile, err := findInstanceProfileFromName(c.Context(), i.server, "test")
	c.Assert(instanceProfile, tc.IsNil)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (i *IAMSuite) TestFindRoleExists(c *tc.C) {
	_, err := i.server.CreateRole(c.Context(), &iam.CreateRoleInput{
		RoleName:    aws.String("test-role"),
		Description: aws.String("test-description"),
	})
	c.Assert(err, tc.ErrorIsNil)

	role, err := findRoleFromName(c.Context(), i.server, "test-role")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(*role.RoleName, tc.Equals, "test-role")
}
