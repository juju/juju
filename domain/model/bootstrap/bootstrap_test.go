// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/agentbinary"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	accessstate "github.com/juju/juju/domain/access/state"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	"github.com/juju/juju/domain/constraints"
	credentialbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	statemodel "github.com/juju/juju/domain/model/state/model"
	"github.com/juju/juju/domain/model/state/testing"
	"github.com/juju/juju/domain/modelagent"
	networkerrors "github.com/juju/juju/domain/network/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type bootstrapSuite struct {
	schematesting.ControllerModelSuite

	adminUserUUID  coreuser.UUID
	cloudName      string
	credentialName string
}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	controllerUUID := s.SeedControllerUUID(c)

	var err error
	s.adminUserUUID, err = coreuser.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	accessState := accessstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = accessState.AddUserWithPermission(
		c.Context(), s.adminUserUUID,
		coreuser.AdminUserName,
		coreuser.AdminUserName.Name(),
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
	c.Assert(err, tc.ErrorIsNil)

	s.cloudName = "test"
	fn := cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      s.cloudName,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
	})

	err = fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	s.credentialName = "test"
	fn = credentialbootstrap.InsertCredential(
		credential.Key{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
			"access-key": "val",
		}),
	)

	err = fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	testing.CreateInternalSecretBackend(c, s.ControllerTxnRunner())
}

func (s *bootstrapSuite) TestUUIDIsRespected(c *tc.C) {
	fn := CreateGlobalModelRecord(
		modeltesting.GenModelUUID(c),
		model.GlobalModelCreationArgs{
			Cloud: s.cloudName,
			Credential: credential.Key{
				Cloud: s.cloudName,
				Name:  s.credentialName,
				Owner: coreuser.AdminUserName,
			},
			Name:       "test",
			Qualifier:  "prod",
			AdminUsers: []coreuser.UUID{s.adminUserUUID},
		})

	err := fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *bootstrapSuite) TestCreateModelDetails(c *tc.C) {
	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)

	args := model.GlobalModelCreationArgs{
		Cloud: s.cloudName,
		Credential: credential.Key{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:       "test",
		Qualifier:  "prod",
		AdminUsers: []coreuser.UUID{s.adminUserUUID},
	}

	// Create a model and then create a read-only model from it.
	fn := CreateGlobalModelRecord(modelUUID, args)
	err := fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	fn = CreateLocalModelRecordWithAgentStream(modelUUID, controllerUUID, jujuversion.Current, agentbinary.AgentStreamReleased)
	err = fn(c.Context(), s.ControllerTxnRunner(), s.ModelTxnRunner(c, modelUUID.String()))
	c.Assert(err, tc.ErrorIsNil)

	m := dbReadOnlyModel{}
	stmt, err := sqlair.Prepare(`SELECT &dbReadOnlyModel.* FROM model`, m)
	c.Assert(err, tc.ErrorIsNil)

	err = s.ModelTxnRunner(c, modelUUID.String()).Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).Get(&m)
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(m.UUID, tc.Equals, modelUUID.String())
	c.Check(m.ControllerUUID, tc.Equals, controllerUUID.String())
	c.Check(m.Name, tc.Equals, args.Name)
	c.Check(m.IsControllerModel, tc.Equals, true)

	v := sqlair.M{}
	stmt, err = sqlair.Prepare(`
SELECT &M.target_version,
       &M.stream_id
FROM agent_version`, v)
	c.Assert(err, tc.ErrorIsNil)

	err = s.ModelTxnRunner(c, modelUUID.String()).Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).Get(&v)
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(v["target_version"], tc.DeepEquals, jujuversion.Current.String())
	c.Check(v["stream_id"], tc.Equals, int64(modelagent.AgentStreamReleased))
}

// TestCreateModelUnsupportedCredential is asserting the fact that if we supply
// an empty credential to the model creation process and this type of credential
// isn't supported by the cloud then an error satisfying
// [modelerrors.CredentialNotValid] is returned.
func (s *bootstrapSuite) TestCreateModelUnsupportedCredential(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	fn := cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      "test-cloud",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
	})
	err := fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	args := model.GlobalModelCreationArgs{
		// We assume here that the cloud made behind s.cloudName
		Cloud:      "test-cloud",
		Credential: credential.Key{},
		Name:       "test",
		Qualifier:  "prod",
		AdminUsers: []coreuser.UUID{s.adminUserUUID},
	}

	// Create a model and then create a read-only model from it.
	fn = CreateGlobalModelRecord(modelUUID, args)
	err = fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Check(err, tc.ErrorIs, modelerrors.CredentialNotValid)
}

// TestCreateModelWithEmptyCredential is asserting that we can create models
// with empty cloud credentials when the cloud supports it.
func (s *bootstrapSuite) TestCreateModelWithEmptyCredential(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	fn := cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      "test-cloud",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
	})
	err := fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	args := model.GlobalModelCreationArgs{
		// We assume here that the cloud made behind s.cloudName
		Cloud:      "test-cloud",
		Credential: credential.Key{},
		Name:       "test",
		Qualifier:  "prod",
		AdminUsers: []coreuser.UUID{s.adminUserUUID},
	}

	fn = CreateGlobalModelRecord(modelUUID, args)
	err = fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Check(err, tc.ErrorIsNil)
}

type dbReadOnlyModel struct {
	UUID              string `db:"uuid"`
	ControllerUUID    string `db:"controller_uuid"`
	Name              string `db:"name"`
	Type              string `db:"type"`
	Cloud             string `db:"cloud"`
	CloudType         string `db:"cloud_type"`
	CloudRegion       string `db:"cloud_region"`
	CredentialOwner   string `db:"credential_owner"`
	CredentialName    string `db:"credential_name"`
	IsControllerModel bool   `db:"is_controller_model"`
}

func (s *bootstrapSuite) TestSetModelConstraints(c *tc.C) {
	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)

	args := model.GlobalModelCreationArgs{
		Cloud: s.cloudName,
		Credential: credential.Key{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:       "test",
		Qualifier:  "prod",
		AdminUsers: []coreuser.UUID{s.adminUserUUID},
	}

	// Create a model and then create a read-only model from it.
	fn := CreateGlobalModelRecord(modelUUID, args)
	err := fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	fn = CreateLocalModelRecordWithAgentStream(modelUUID, controllerUUID, jujuversion.Current, agentbinary.AgentStreamReleased)
	err = fn(c.Context(), s.ControllerTxnRunner(), s.ModelTxnRunner(c, modelUUID.String()))
	c.Assert(err, tc.ErrorIsNil)

	cons := coreconstraints.Value{
		Arch:      ptr("amd64"),
		Container: ptr(instance.LXD),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	fn = SetModelConstraints(cons)
	err = fn(c.Context(), s.ControllerTxnRunner(), s.ModelTxnRunner(c, modelUUID.String()))
	c.Assert(err, tc.ErrorIsNil)

	modelState := statemodel.NewState(func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(c, modelUUID.String()), nil
	}, loggertesting.WrapCheckLog(c))

	expected := constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.LXD),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}

	data, err := modelState.GetModelConstraints(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(data, tc.DeepEquals, expected)
}

// TestSetModelConstraintsFailedModelNotFound is asserting that if we set model
// constraints and the model does not exist we get back an error satisfying
// [modelerrors.NotFound].
func (s *bootstrapSuite) TestSetModelConstraintFailedModelNotFound(c *tc.C) {
	state := statemodel.NewState(func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(c, modeltesting.GenModelUUID(c).String()), nil
	}, loggertesting.WrapCheckLog(c))

	err := state.SetModelConstraints(c.Context(), constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
	})
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestSetModelConstraintsInvalidContainerType asserts that if we set model
// constraints with an unknown/invalid container type we get back an error
// satisfying [machineerrors.InvalidContainerType] and no changes are made to
// the database.
func (s *bootstrapSuite) TestSetModelConstraintsInvalidContainerType(c *tc.C) {
	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)

	args := model.GlobalModelCreationArgs{
		Cloud: s.cloudName,
		Credential: credential.Key{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:       "test",
		Qualifier:  "prod",
		AdminUsers: []coreuser.UUID{s.adminUserUUID},
	}

	// Create a model and then create a read-only model from it.
	fn := CreateGlobalModelRecord(modelUUID, args)
	err := fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	fn = CreateLocalModelRecord(modelUUID, controllerUUID, jujuversion.Current)
	err = fn(c.Context(), s.ControllerTxnRunner(), s.ModelTxnRunner(c, modelUUID.String()))
	c.Assert(err, tc.ErrorIsNil)

	state := statemodel.NewState(func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(c, modelUUID.String()), nil
	}, loggertesting.WrapCheckLog(c))

	cons := constraints.Constraints{
		Container: ptr(instance.ContainerType("noexist")),
		ImageID:   ptr("image-id"),
	}

	err = state.SetModelConstraints(c.Context(), cons)
	c.Check(err, tc.ErrorIs, machineerrors.InvalidContainerType)

	_, err = state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.ConstraintsNotFound)
}

// TestSetModelConstraintFailedSpaceDoesNotExist asserts that if we set model
// constraints for a space that doesn't exist we get back an error satisfying
// [networkerrors.SpaceNotFound] and that no changes are made to the database.
func (s *bootstrapSuite) TestSetModelConstraintFailedSpaceDoesNotExist(c *tc.C) {
	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)

	args := model.GlobalModelCreationArgs{
		Cloud: s.cloudName,
		Credential: credential.Key{
			Cloud: s.cloudName,
			Name:  s.credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:       "test",
		Qualifier:  "prod",
		AdminUsers: []coreuser.UUID{s.adminUserUUID},
	}

	// Create a model and then create a read-only model from it.
	fn := CreateGlobalModelRecord(modelUUID, args)
	err := fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	fn = CreateLocalModelRecordWithAgentStream(modelUUID, controllerUUID, jujuversion.Current, agentbinary.AgentStreamReleased)
	err = fn(c.Context(), s.ControllerTxnRunner(), s.ModelTxnRunner(c, modelUUID.String()))
	c.Assert(err, tc.ErrorIsNil)

	state := statemodel.NewState(func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(c, modelUUID.String()), nil
	}, loggertesting.WrapCheckLog(c))

	err = state.SetModelConstraints(c.Context(), constraints.Constraints{
		Spaces: ptr([]constraints.SpaceConstraint{
			{
				SpaceName: "space1",
				Exclude:   false,
			},
		}),
		ImageID: ptr("image-id"),
	})
	c.Check(err, tc.ErrorIs, networkerrors.SpaceNotFound)

	_, err = state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.ConstraintsNotFound)
}

func ptr[T any](s T) *T {
	return &s
}
