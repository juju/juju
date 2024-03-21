// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	credentialbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	"github.com/juju/juju/domain/model"
	schematesting "github.com/juju/juju/domain/schema/testing"
	userbootstrap "github.com/juju/juju/domain/user/bootstrap"
	jujuversion "github.com/juju/juju/version"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite

	adminUserUUID  coreuser.UUID
	cloudName      string
	credentialName string
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)

	uuid, fn := userbootstrap.AddUser(coreuser.AdminUserName, permission.ControllerForAccess(permission.SuperuserAccess))
	err := fn(context.Background(), s.ControllerTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	s.adminUserUUID = uuid

	s.cloudName = "test"
	fn = cloudbootstrap.InsertCloud(cloud.Cloud{
		Name:      s.cloudName,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
	})

	err = fn(context.Background(), s.ControllerTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	s.credentialName = "test"
	fn = credentialbootstrap.InsertCredential(credential.ID{
		Cloud: s.cloudName,
		Name:  s.credentialName,
		Owner: coreuser.AdminUserName,
	},
		cloud.NewCredential(cloud.EmptyAuthType, nil),
	)

	err = fn(context.Background(), s.ControllerTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bootstrapSuite) TestUUIDIsCreated(c *gc.C) {
	uuid, fn := CreateModel(model.ModelCreationArgs{
		AgentVersion: jujuversion.Current,
		Cloud:        s.cloudName,
		Credential: credential.ID{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:  "test",
		Owner: s.adminUserUUID,
	})

	err := fn(context.Background(), s.ControllerTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(uuid.String() == "", jc.IsFalse)
}

func (s *bootstrapSuite) TestUUIDIsRespected(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	uuid, fn := CreateModel(model.ModelCreationArgs{
		AgentVersion: jujuversion.Current,
		Cloud:        s.cloudName,
		Credential: credential.ID{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:  "test",
		Owner: s.adminUserUUID,
		UUID:  modelUUID,
	})

	err := fn(context.Background(), s.ControllerTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(uuid, gc.Equals, modelUUID)
}

type modelBootstrapSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&modelBootstrapSuite{})

func (s *modelBootstrapSuite) TestCreateReadOnlyModel(c *gc.C) {
	fn := CreateReadOnlyModel(model.ReadOnlyModelCreationArgs{
		UUID:        modeltesting.GenModelUUID(c),
		Name:        "test",
		Type:        coremodel.IAAS,
		Cloud:       "aws",
		CloudRegion: "myregion",
	})

	err := fn(context.Background(), s.ModelTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}
