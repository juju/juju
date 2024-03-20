// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	schematesting "github.com/juju/juju/domain/schema/testing"
	userstate "github.com/juju/juju/domain/user/state"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestInsertInitialControllerConfig(c *gc.C) {
	ctx := context.Background()
	cld := cloud.Cloud{Name: "cirrus", Type: "ec2", AuthTypes: cloud.AuthTypes{cloud.UserPassAuthType}}
	err := cloudbootstrap.InsertCloud(cld)(ctx, s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	userState := userstate.NewState(s.TxnRunnerFactory())
	err = userState.AddUser(
		context.Background(), userUUID,
		"fred",
		"test user",
		userUUID,
		permission.SuperuserAccess,
	)
	c.Assert(err, jc.ErrorIsNil)
	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"foo": "bar"}, false)

	id := credential.ID{
		Cloud: "cirrus",
		Owner: "fred",
		Name:  "foo",
	}

	err = InsertCredential(id, cred)(ctx, s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	var owner, cloudName string
	row := s.DB().QueryRow(`
SELECT owner_uuid, cloud.name FROM cloud_credential
JOIN cloud ON cloud.uuid = cloud_credential.cloud_uuid
WHERE cloud_credential.name = ?`, "foo")
	c.Assert(row.Scan(&owner, &cloudName), jc.ErrorIsNil)
	c.Assert(owner, gc.Equals, userUUID.String())
	c.Assert(cloudName, gc.Equals, "cirrus")
}
