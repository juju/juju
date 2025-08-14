// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	userstate "github.com/juju/juju/domain/access/state"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite

	controllerUUID string
}

func TestBootstrapSuite(t *testing.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.controllerUUID = s.SeedControllerUUID(c)
}

func (s *bootstrapSuite) TestInsertInitialControllerConfig(c *tc.C) {
	ctx := c.Context()

	userUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	userState := userstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = userState.AddUserWithPermission(
		c.Context(), userUUID,
		user.GenName(c, "fred"),
		"test user",
		false,
		userUUID,
		permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.controllerUUID,
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	cld := cloud.Cloud{Name: "cirrus", Type: "ec2", AuthTypes: cloud.AuthTypes{cloud.UserPassAuthType}}
	err = cloudbootstrap.InsertCloud(user.GenName(c, "fred"), cld)(ctx, s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"foo": "bar"}, false)

	key := credential.Key{
		Cloud: "cirrus",
		Owner: user.GenName(c, "fred"),
		Name:  "foo",
	}

	err = InsertCredential(key, cred)(ctx, s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	var owner, cloudName string
	row := s.DB().QueryRow(`
SELECT owner_uuid, cloud.name FROM cloud_credential
JOIN cloud ON cloud.uuid = cloud_credential.cloud_uuid
WHERE cloud_credential.name = ?`, "foo")
	c.Assert(row.Scan(&owner, &cloudName), tc.ErrorIsNil)
	c.Assert(owner, tc.Equals, userUUID.String())
	c.Assert(cloudName, tc.Equals, "cirrus")
}
