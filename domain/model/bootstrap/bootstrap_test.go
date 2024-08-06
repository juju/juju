// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	accessstate "github.com/juju/juju/domain/access/state"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	credentialbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type baseSuite struct {
	schematesting.ControllerSuite

	adminUserUUID  coreuser.UUID
	cloudName      string
	credentialName string
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)

	controllerUUID := s.SeedControllerUUID(c)

	var err error
	s.adminUserUUID, err = coreuser.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	accessState := accessstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = accessState.AddUser(
		context.Background(), s.adminUserUUID,
		coreuser.AdminUserName,
		coreuser.AdminUserName,
		false,
		s.adminUserUUID,
		permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        controllerUUID,
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.cloudName = "test"
	fn := cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      s.cloudName,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
	})

	err = fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	s.credentialName = "test"
	fn = credentialbootstrap.InsertCredential(credential.Key{
		Cloud: s.cloudName,
		Name:  s.credentialName,
		Owner: coreuser.AdminUserName,
	},
		cloud.NewCredential(cloud.EmptyAuthType, nil),
	)

	err = fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	testing.CreateInternalSecretBackend(c, s.ControllerTxnRunner())
}

type bootstrapSuite struct {
	baseSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestUUIDIsRespected(c *gc.C) {
	fn := CreateModel(
		modeltesting.GenModelUUID(c),
		model.ModelCreationArgs{
			AgentVersion: jujuversion.Current,
			Cloud:        s.cloudName,
			Credential: credential.Key{
				Cloud: s.cloudName,
				Name:  s.credentialName,
				Owner: coreuser.AdminUserName,
			},
			Name:  "test",
			Owner: s.adminUserUUID,
		})

	err := fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}

type modelBootstrapSuite struct {
	baseSuite
	schematesting.ModelSuite
}

var _ = gc.Suite(&modelBootstrapSuite{})

func (s *modelBootstrapSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	s.ModelSuite.SetUpTest(c)
}

func (s *modelBootstrapSuite) TestCreateReadOnlyModel(c *gc.C) {
	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)

	args := model.ModelCreationArgs{
		AgentVersion: jujuversion.Current,
		Cloud:        s.cloudName,
		Credential: credential.Key{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:  "test",
		Owner: s.adminUserUUID,
	}

	// Create a model and then create a read-only model from it.
	fn := CreateModel(modelUUID, args)
	err := fn(context.Background(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	fn = CreateReadOnlyModel(modelUUID, controllerUUID)
	err = fn(context.Background(), s.ControllerTxnRunner(), s.ModelTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelBootstrapSuite) TestCreateModelWithDifferingBuildNumber(c *gc.C) {
	v := jujuversion.Current
	v.Build++

	args := model.ModelCreationArgs{
		AgentVersion: v,
		Cloud:        s.cloudName,
		Credential: credential.Key{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:  "test",
		Owner: s.adminUserUUID,
	}

	// Create a model and then create a read-only model from it.
	fn := CreateModel(modeltesting.GenModelUUID(c), args)
	err := fn(context.Background(), s.ControllerTxnRunner(), s.ModelTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}
