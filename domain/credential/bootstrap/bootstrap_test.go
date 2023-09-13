// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	schematesting "github.com/juju/juju/domain/schema/testing"
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
	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"foo": "bar"}, false)

	tag := names.NewCloudCredentialTag("cirrus/fred/foo")

	err = InsertCredential(tag, cred)(ctx, s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	var owner, cloudName string
	row := s.DB().QueryRow(`
SELECT owner_uuid, cloud.name FROM cloud_credential
JOIN cloud ON cloud.uuid = cloud_credential.cloud_uuid
WHERE cloud_credential.name = ?`, "foo")
	c.Assert(row.Scan(&owner, &cloudName), jc.ErrorIsNil)
	c.Assert(owner, gc.Equals, "fred")
	c.Assert(cloudName, gc.Equals, "cirrus")
}
