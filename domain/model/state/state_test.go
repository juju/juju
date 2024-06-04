// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	usererrors "github.com/juju/juju/domain/access/errors"
	accessstate "github.com/juju/juju/domain/access/state"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	dbcloud "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secretbackend/bootstrap"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/version"
	jujuversion "github.com/juju/juju/version"
)

type stateSuite struct {
	schematesting.ControllerSuite

	uuid     coremodel.UUID
	userUUID user.UUID
	userName string
}

var _ = gc.Suite(&stateSuite{})

func (m *stateSuite) SetUpTest(c *gc.C) {
	m.ControllerSuite.SetUpTest(c)

	// We need to generate a user in the database so that we can set the model
	// owner.
	userUUID, err := user.NewUUID()
	m.userUUID = userUUID
	m.userName = "test-user"
	c.Assert(err, jc.ErrorIsNil)
	userState := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = userState.AddUser(
		context.Background(),
		m.userUUID,
		m.userName,
		m.userName,
		m.userUUID,
		permission.ControllerForAccess(permission.SuperuserAccess),
	)
	c.Assert(err, jc.ErrorIsNil)

	// We need to generate a cloud in the database so that we can set the model
	// cloud.
	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	err = cloudSt.CreateCloud(context.Background(), m.userName, uuid.MustNewUUID().String(),
		cloud.Cloud{
			Name:      "my-cloud",
			Type:      "ec2",
			AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
			Regions: []cloud.Region{
				{
					Name: "my-region",
				},
			},
		})
	c.Assert(err, jc.ErrorIsNil)
	err = cloudSt.CreateCloud(context.Background(), m.userName, uuid.MustNewUUID().String(),
		cloud.Cloud{
			Name:      "other-cloud",
			Type:      "ec2",
			AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
			Regions: []cloud.Region{
				{
					Name: "other-region",
				},
			},
		})
	c.Assert(err, jc.ErrorIsNil)

	// We need to generate a cloud credential in the database so that we can set
	// the models cloud credential.
	cred := credential.CloudCredentialInfo{
		Label:    "foobar",
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}

	credSt := credentialstate.NewState(m.TxnRunnerFactory())
	_, err = credSt.UpsertCloudCredential(
		context.Background(), corecredential.Key{
			Cloud: "my-cloud",
			Owner: "test-user",
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, jc.ErrorIsNil)
	_, err = credSt.UpsertCloudCredential(
		context.Background(), corecredential.Key{
			Cloud: "other-cloud",
			Owner: "test-user",
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, jc.ErrorIsNil)

	err = bootstrap.CreateDefaultBackends(coremodel.IAAS)(context.Background(), m.ControllerTxnRunner(), m.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	m.uuid = modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err = modelSt.Create(
		context.Background(),
		m.uuid,
		coremodel.IAAS,
		model.ModelCreationArgs{
			AgentVersion: version.Current,
			Cloud:        "my-cloud",
			CloudRegion:  "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:          "my-test-model",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = modelSt.Activate(context.Background(), m.uuid)
	c.Assert(err, jc.ErrorIsNil)
}

// TestCloudType is testing the happy path of [CloudType] to make sure we get
// back the correct type of a cloud.
func (m *stateSuite) TestCloudType(c *gc.C) {
	st := NewState(m.TxnRunnerFactory())
	ctype, err := st.CloudType(context.Background(), "my-cloud")
	c.Check(err, jc.ErrorIsNil)
	c.Check(ctype, gc.Equals, "ec2")
}

// TestCloudTypeMissing is testing that if we ask for a cloud type of a cloud
// that does not exist we get back an error that satisfies
// [clouderrors.NotFound].
func (m *stateSuite) TestCloudTypeMissing(c *gc.C) {
	st := NewState(m.TxnRunnerFactory())
	ctype, err := st.CloudType(context.Background(), "no-exist-cloud")
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)
	c.Check(ctype, gc.Equals, "")
}

// TestModelCloudNameAndCredential tests the happy path for getting a models
// cloud name and credential.
func (m *stateSuite) TestModelCloudNameAndCredential(c *gc.C) {
	st := NewState(m.TxnRunnerFactory())
	// We are relying on the model setup as part of this suite.
	cloudName, credentialID, err := st.ModelCloudNameAndCredential(context.Background(), "my-test-model", "test-user")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cloudName, gc.Equals, "my-cloud")
	c.Check(credentialID, gc.Equals, corecredential.Key{
		Cloud: "my-cloud",
		Owner: m.userName,
		Name:  "foobar",
	})
}

// TestModelCloudNameAndCredentialController is testing the cloud name and
// credential id is returned for the controller model and owner. This is the
// common pattern that this sate func will be used for so we have made a special
// case to continuously test this.
func (m *stateSuite) TestModelCloudNameAndCredentialController(c *gc.C) {
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userState := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = userState.AddUser(
		context.Background(),
		userUUID,
		coremodel.ControllerModelOwnerUsername,
		coremodel.ControllerModelOwnerUsername,
		userUUID,
		permission.ControllerForAccess(permission.SuperuserAccess),
	)
	c.Assert(err, jc.ErrorIsNil)

	st := NewState(m.TxnRunnerFactory())
	modelUUID := modeltesting.GenModelUUID(c)
	// We need to first inject a model that does not have a cloud credential set
	err = st.Create(
		context.Background(),
		modelUUID,
		coremodel.IAAS,
		model.ModelCreationArgs{
			AgentVersion: version.Current,
			Cloud:        "my-cloud",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: m.userName,
				Name:  "foobar",
			},
			Name:          coremodel.ControllerModelName,
			Owner:         userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = st.Activate(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)

	cloudName, credentialID, err := st.ModelCloudNameAndCredential(
		context.Background(),
		coremodel.ControllerModelName,
		coremodel.ControllerModelOwnerUsername,
	)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(cloudName, gc.Equals, "my-cloud")
	c.Check(credentialID, gc.Equals, corecredential.Key{
		Cloud: "my-cloud",
		Owner: m.userName,
		Name:  "foobar",
	})
}

// TestModelCloudNameAndCredentialNotFound is testing that if we pass a model
// that doesn't exist we get back a [modelerrors.NotFound] error.
func (m *stateSuite) TestModelCloudNameAndCredentialNotFound(c *gc.C) {
	st := NewState(m.TxnRunnerFactory())
	// We are relying on the model setup as part of this suite.
	cloudName, credentialID, err := st.ModelCloudNameAndCredential(context.Background(), "does-not-exist", "test-user")
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
	c.Check(cloudName, gc.Equals, "")
	c.Check(credentialID.IsZero(), jc.IsTrue)
}

func (m *stateSuite) TestGetModel(c *gc.C) {
	runner := m.TxnRunnerFactory()

	modelSt := NewState(runner)
	modelInfo, err := modelSt.GetModel(context.Background(), m.uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelInfo, gc.Equals, coremodel.Model{
		AgentVersion: jujuversion.Current,
		UUID:         m.uuid,
		Cloud:        "my-cloud",
		CloudRegion:  "my-region",
		Credential: corecredential.Key{
			Cloud: "my-cloud",
			Owner: "test-user",
			Name:  "foobar",
		},
		Name:      "my-test-model",
		Owner:     m.userUUID,
		OwnerName: "test-user",
		ModelType: coremodel.IAAS,
		Life:      life.Alive,
	})
}

func (m *stateSuite) TestGetModelType(c *gc.C) {
	runner := m.TxnRunnerFactory()

	modelSt := NewState(runner)
	modelType, err := modelSt.GetModelType(context.Background(), m.uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelType, gc.Equals, coremodel.IAAS)
}

func (m *stateSuite) TestGetModelNotFound(c *gc.C) {
	runner := m.TxnRunnerFactory()
	modelSt := NewState(runner)
	_, err := modelSt.GetModel(context.Background(), modeltesting.GenModelUUID(c))
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestCreateModelAgentWithNoModel is asserting that if we attempt to make a
// model agent record where no model already exists that we get back a
// [modelerrors.NotFound] error.
func (m *stateSuite) TestCreateModelAgentWithNoModel(c *gc.C) {
	runner, err := m.TxnRunnerFactory()()
	c.Assert(err, jc.ErrorIsNil)

	testUUID := modeltesting.GenModelUUID(c)
	err = runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return createModelAgent(context.Background(), tx, testUUID, version.Current)
	})

	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestCreateModelAgentAlreadyExists is asserting that if we attempt to make a
// model agent record when one already exists we get a
// [modelerrors.AlreadyExists] back.
func (m *stateSuite) TestCreateModelAgentAlreadyExists(c *gc.C) {
	runner, err := m.TxnRunnerFactory()()
	c.Assert(err, jc.ErrorIsNil)

	err = runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return createModelAgent(context.Background(), tx, m.uuid, version.Current)
	})

	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

// TestCreateModelWithExisting is testing that if we attempt to make a new model
// with the same uuid as one that already exists we get back a
// [modelerrors.AlreadyExists] error.
func (m *stateSuite) TestCreateModelWithExisting(c *gc.C) {
	runner, err := m.TxnRunnerFactory()()
	c.Assert(err, jc.ErrorIsNil)

	err = runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return createModel(
			ctx,
			tx,
			m.uuid,
			coremodel.IAAS,
			model.ModelCreationArgs{
				Cloud:         "my-cloud",
				CloudRegion:   "my-region",
				Name:          "fantasticmodel",
				Owner:         m.userUUID,
				SecretBackend: juju.BackendName,
			},
		)
	})
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

// TestCreateModelWithSameNameAndOwner is testing that we attempt to create a
// new model with a different uuid but the same owner and name as one that
// exists we get back a [modelerrors.AlreadyExists] error.
func (m *stateSuite) TestCreateModelWithSameNameAndOwner(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		context.Background(),
		testUUID,
		coremodel.IAAS,
		model.ModelCreationArgs{
			Cloud:         "my-cloud",
			CloudRegion:   "my-region",
			Name:          "my-test-model",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (m *stateSuite) TestCreateModelWithInvalidCloudRegion(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		context.Background(),
		testUUID,
		coremodel.IAAS,
		model.ModelCreationArgs{
			Cloud:         "my-cloud",
			CloudRegion:   "noexist",
			Name:          "noregion",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (m *stateSuite) TestCreateWithEmptyRegion(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		context.Background(),
		testUUID,
		coremodel.IAAS,
		model.ModelCreationArgs{
			Cloud: "my-cloud",
			Name:  "noregion",
			Owner: m.userUUID,
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = modelSt.Activate(context.Background(), testUUID)
	c.Assert(err, jc.ErrorIsNil)

	modelInfo, err := modelSt.GetModel(context.Background(), testUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelInfo.CloudRegion, gc.Equals, "")
}

func (m *stateSuite) TestCreateWithEmptyRegionUsesControllerRegion(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())

	err := modelSt.Create(
		context.Background(),
		modeltesting.GenModelUUID(c),
		coremodel.IAAS,
		model.ModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Name:        "controller",
			Owner:       m.userUUID,
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	testUUID := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		context.Background(),
		testUUID,
		coremodel.IAAS,
		model.ModelCreationArgs{
			Cloud: "my-cloud",
			Name:  "noregion",
			Owner: m.userUUID,
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = modelSt.Activate(context.Background(), testUUID)
	c.Assert(err, jc.ErrorIsNil)

	modelInfo, err := modelSt.GetModel(context.Background(), testUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelInfo.CloudRegion, gc.Equals, "my-region")
}

func (m *stateSuite) TestCreateWithEmptyRegionDoesNotUseControllerRegionForDifferentCloudNames(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())

	controllerUUID := modeltesting.GenModelUUID(c)

	err := modelSt.Create(
		context.Background(),
		controllerUUID,
		coremodel.IAAS,
		model.ModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Name:        "controller",
			Owner:       m.userUUID,
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = modelSt.Activate(context.Background(), controllerUUID)
	c.Assert(err, jc.ErrorIsNil)

	modelInfo, err := modelSt.GetModel(context.Background(), controllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelInfo.CloudRegion, gc.Equals, "my-region")

	testUUID := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		context.Background(),
		testUUID,
		coremodel.IAAS,
		model.ModelCreationArgs{
			Cloud: "other-cloud",
			Name:  "noregion",
			Owner: m.userUUID,
			Credential: corecredential.Key{
				Cloud: "other-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = modelSt.Activate(context.Background(), testUUID)
	c.Assert(err, jc.ErrorIsNil)

	modelInfo, err = modelSt.GetModel(context.Background(), testUUID)
	c.Assert(err, jc.ErrorIsNil)

	// We should never set the region to the controller region if the cloud
	// names are different.

	c.Check(modelInfo.CloudRegion, gc.Equals, "")
}

// TestCreateModelWithNonExistentOwner is here to assert that if we try and make
// a model with a user/owner that does not exist a [usererrors.NotFound] error
// is returned.
func (m *stateSuite) TestCreateModelWithNonExistentOwner(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		context.Background(),
		testUUID,
		coremodel.IAAS,
		model.ModelCreationArgs{
			Cloud:         "my-cloud",
			CloudRegion:   "noexist",
			Name:          "noregion",
			Owner:         user.UUID("noexist"), // does not exist
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

// TestCreateModelWithRemovedOwner is here to test that if we try and create a
// new model with an owner that has been removed from the Juju user base that
// the operation fails with a [usererrors.NotFound] error.
func (m *stateSuite) TestCreateModelWithRemovedOwner(c *gc.C) {
	userState := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := userState.RemoveUser(context.Background(), m.userName)
	c.Assert(err, jc.ErrorIsNil)

	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		context.Background(),
		testUUID,
		coremodel.IAAS,
		model.ModelCreationArgs{
			Cloud:         "my-cloud",
			CloudRegion:   "noexist",
			Name:          "noregion",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

// TestCreateModelVerifyPermissionSet is here to test that a permission is
// created for the owning user when a model is created.
func (m *stateSuite) TestCreateModelVerifyPermissionSet(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	ctx := context.Background()
	err := modelSt.Create(
		ctx,
		testUUID,
		coremodel.IAAS,
		model.ModelCreationArgs{
			AgentVersion: version.Current,
			Cloud:        "my-cloud",
			CloudRegion:  "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:          "listtest1",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	accessSt := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	access, err := accessSt.ReadUserAccessLevelForTarget(ctx, m.userName, permission.ID{
		ObjectType: permission.Model,
		Key:        m.uuid.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, permission.AdminAccess)
}

func (m *stateSuite) TestCreateModelWithInvalidCloud(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		context.Background(),
		testUUID,
		coremodel.IAAS,
		model.ModelCreationArgs{
			Cloud:         "noexist",
			CloudRegion:   "my-region",
			Name:          "noregion",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (m *stateSuite) TestUpdateCredentialForDifferentCloud(c *gc.C) {
	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	err := cloudSt.CreateCloud(context.Background(), m.userName, uuid.MustNewUUID().String(),
		cloud.Cloud{
			Name:      "my-cloud2",
			Type:      "ec2",
			AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
			Regions: []cloud.Region{
				{
					Name: "my-region",
				},
			},
		})
	c.Assert(err, jc.ErrorIsNil)

	cred := credential.CloudCredentialInfo{
		Label:    "foobar",
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}

	credSt := credentialstate.NewState(m.TxnRunnerFactory())
	_, err = credSt.UpsertCloudCredential(
		context.Background(), corecredential.Key{
			Cloud: "my-cloud2",
			Owner: "test-user",
			Name:  "foobar1",
		},
		cred,
	)
	c.Assert(err, jc.ErrorIsNil)

	modelSt := NewState(m.TxnRunnerFactory())
	err = modelSt.UpdateCredential(
		context.Background(),
		m.uuid,
		corecredential.Key{
			Cloud: "my-cloud2",
			Owner: "test-user",
			Name:  "foobar1",
		},
	)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

// We are trying to test here that we can set a cloud credential on the model
// for the same cloud as the model when no cloud region has been set. This is a
// regression test discovered during DQlite development where we messed up the
// logic assuming that a cloud region was always set for a model.
func (m *stateSuite) TestSetModelCloudCredentialWithoutRegion(c *gc.C) {
	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	err := cloudSt.CreateCloud(context.Background(), m.userName, uuid.MustNewUUID().String(),
		cloud.Cloud{
			Name:      "minikube",
			Type:      "kubernetes",
			AuthTypes: cloud.AuthTypes{cloud.UserPassAuthType},
			Regions:   []cloud.Region{},
		})
	c.Assert(err, jc.ErrorIsNil)

	cred := credential.CloudCredentialInfo{
		Label:    "foobar",
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"username": "myuser",
			"password": "secret",
		},
	}

	credSt := credentialstate.NewState(m.TxnRunnerFactory())
	_, err = credSt.UpsertCloudCredential(
		context.Background(), corecredential.Key{
			Cloud: "minikube",
			Owner: "test-user",
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, jc.ErrorIsNil)

	err = bootstrap.CreateDefaultBackends(coremodel.CAAS)(context.Background(), m.ControllerTxnRunner(), m.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	m.uuid = modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err = modelSt.Create(
		context.Background(),
		m.uuid,
		coremodel.CAAS,
		model.ModelCreationArgs{
			Cloud: "minikube",
			Credential: corecredential.Key{
				Cloud: "minikube",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:          "controller",
			Owner:         m.userUUID,
			SecretBackend: kubernetes.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = modelSt.Activate(context.Background(), m.uuid)
	c.Assert(err, jc.ErrorIsNil)
}

// TestDeleteModel tests that we can delete a model that is already created in
// the system. We also confirm that list models returns no models after the
// deletion.
func (m *stateSuite) TestDeleteModel(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Delete(
		context.Background(),
		m.uuid,
	)
	c.Assert(err, jc.ErrorIsNil)

	runner, err := m.TxnRunnerFactory()()
	c.Assert(err, jc.ErrorIsNil)

	err = runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(
			context.Background(),
			"SELECT uuid FROM model WHERE uuid = ?",
			m.uuid,
		)
		var val string
		err := row.Scan(&val)
		c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	modelUUIDS, err := modelSt.ListModelIDs(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(modelUUIDS, gc.HasLen, 0)
}

func (m *stateSuite) TestDeleteModelNotFound(c *gc.C) {
	uuid := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Delete(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestListModelIDs is testing that once we have created several models calling
// list returns all of the models created.
func (m *stateSuite) TestListModelIDs(c *gc.C) {
	uuid1 := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Create(
		context.Background(),
		uuid1,
		coremodel.IAAS,
		model.ModelCreationArgs{
			AgentVersion: version.Current,
			Cloud:        "my-cloud",
			CloudRegion:  "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:          "listtest1",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = modelSt.Activate(context.Background(), uuid1)
	c.Assert(err, jc.ErrorIsNil)

	uuid2 := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		context.Background(),
		uuid2,
		coremodel.IAAS,
		model.ModelCreationArgs{
			AgentVersion: version.Current,
			Cloud:        "my-cloud",
			CloudRegion:  "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:          "listtest2",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = modelSt.Activate(context.Background(), uuid2)
	c.Assert(err, jc.ErrorIsNil)

	uuids, err := modelSt.ListModelIDs(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuids, gc.HasLen, 3)

	uuidsSet := set.NewStrings()
	for _, uuid := range uuids {
		uuidsSet.Add(uuid.String())
	}

	c.Check(uuidsSet.Contains(uuid1.String()), jc.IsTrue)
	c.Check(uuidsSet.Contains(uuid2.String()), jc.IsTrue)
	c.Check(uuidsSet.Contains(m.uuid.String()), jc.IsTrue)
}

// TestRegisterModelNamespaceNotFound is asserting that when we ask for the
// namespace of a model that doesn't exist we get back a [modelerrors.NotFound]
// error.
func (m *stateSuite) TestRegisterModelNamespaceNotFound(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	var namespace string
	err := m.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		namespace, err = registerModelNamespace(ctx, tx, modelUUID)
		return err
	})
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
	c.Check(namespace, gc.Equals, "")
}

// TestNamespaceForModelNoModel is asserting that when we ask for a models
// database namespace and the model doesn't exist we get back a
// [modelerrors.NotFound] error.
func (m *stateSuite) TestNamespaceForModelNoModel(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	st := NewState(m.TxnRunnerFactory())
	namespace, err := st.NamespaceForModel(context.Background(), modelUUID)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
	c.Check(namespace, gc.Equals, "")
}

// TestNamespaceForModel is testing the happy path for a successful model
// creation that a namespace is returned with no errors.
func (m *stateSuite) TestNamespaceForModel(c *gc.C) {
	st := NewState(m.TxnRunnerFactory())
	namespace, err := st.NamespaceForModel(context.Background(), m.uuid)
	c.Check(err, jc.ErrorIsNil)
	c.Check(namespace, gc.Equals, m.uuid.String())
}

// TestNamespaceForModelDeleted tests that after we have deleted a model we can
// no longer get back the database namespace.
func (m *stateSuite) TestNamespaceForModelDeleted(c *gc.C) {
	st := NewState(m.TxnRunnerFactory())
	err := st.Delete(context.Background(), m.uuid)
	c.Assert(err, jc.ErrorIsNil)

	namespace, err := st.NamespaceForModel(context.Background(), m.uuid)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
	c.Check(namespace, gc.Equals, "")
}

// TestModelsOwnedByUser is asserting that all models owned by a given user are
// returned in the resultant list.
func (m *stateSuite) TestModelsOwnedByUser(c *gc.C) {
	uuid1 := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Create(
		context.Background(),
		uuid1,
		coremodel.IAAS,
		model.ModelCreationArgs{
			AgentVersion: version.Current,
			Cloud:        "my-cloud",
			CloudRegion:  "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:          "owned1",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelSt.Activate(context.Background(), uuid1), jc.ErrorIsNil)

	uuid2 := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		context.Background(),
		uuid2,
		coremodel.IAAS,
		model.ModelCreationArgs{
			AgentVersion: version.Current,
			Cloud:        "my-cloud",
			CloudRegion:  "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:          "owned2",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelSt.Activate(context.Background(), uuid2), jc.ErrorIsNil)

	models, err := modelSt.ListModelsForUser(context.Background(), m.userUUID)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(len(models), gc.Equals, 3)
	slices.SortFunc(models, func(a, b coremodel.Model) int {
		return strings.Compare(a.Name, b.Name)
	})

	c.Check(models, gc.DeepEquals, []coremodel.Model{
		{
			Name:        "my-test-model",
			UUID:        m.uuid,
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			ModelType:   coremodel.IAAS,
			Owner:       m.userUUID,
			OwnerName:   "test-user",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Life: life.Alive,
		},
		{
			Name:        "owned1",
			UUID:        uuid1,
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			ModelType:   coremodel.IAAS,
			Owner:       m.userUUID,
			OwnerName:   "test-user",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Life: life.Alive,
		},
		{
			Name:        "owned2",
			UUID:        uuid2,
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			ModelType:   coremodel.IAAS,
			Owner:       m.userUUID,
			OwnerName:   "test-user",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Life: life.Alive,
		},
	})
}

// TestModelsOwnedByNonExistantUser tests that if we ask for models from a non
// existent user we get back an empty model list.
func (m *stateSuite) TestModelsOwnedByNonExistantUser(c *gc.C) {
	userID := usertesting.GenUserUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())

	models, err := modelSt.ListModelsForUser(context.Background(), userID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(models), gc.Equals, 0)
}

func (m *stateSuite) TestAllModels(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	models, err := modelSt.ListAllModels(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(models, gc.DeepEquals, []coremodel.Model{
		{
			Name:        "my-test-model",
			UUID:        m.uuid,
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			ModelType:   coremodel.IAAS,
			Owner:       m.userUUID,
			OwnerName:   "test-user",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Life: life.Alive,
		},
	})
}

// TestSecretBackendNotFoundForModelCreate is testing that if we specify a
// secret backend that doesn't exist during model creation we back an error that
// satisfies [secretbackenderrors.NotFound]
func (m *stateSuite) TestSecretBackendNotFoundForModelCreate(c *gc.C) {
	uuid := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Create(
		context.Background(),
		uuid,
		coremodel.IAAS,
		model.ModelCreationArgs{
			AgentVersion: version.Current,
			Cloud:        "my-cloud",
			CloudRegion:  "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:          "secretfailure",
			Owner:         m.userUUID,
			SecretBackend: "no-exist",
		},
	)
	c.Check(err, jc.ErrorIs, secretbackenderrors.NotFound)
}

// TestGetModelByNameNotFound is here to assert that if we try and get a model
// by name for any combination of user or model name that doesn't exist we get
// back an error that satisfies [modelerrors.NotFound].
func (m *stateSuite) TestGetModelByNameNotFound(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	_, err := modelSt.GetModelByName(context.Background(), "nonuser", "my-test-model")
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)

	_, err = modelSt.GetModelByName(context.Background(), m.userName, "noexist")
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)

	_, err = modelSt.GetModelByName(context.Background(), "nouser", "noexist")
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestGetModelByName is asserting the happy path of [State.GetModelByName] and
// checking that we can retrieve the model established in SetUpTest by username
// and model name.
func (m *stateSuite) TestGetModelByName(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	model, err := modelSt.GetModelByName(context.Background(), m.userName, "my-test-model")
	c.Check(err, jc.ErrorIsNil)
	c.Check(model, gc.DeepEquals, coremodel.Model{
		Name:         "my-test-model",
		Life:         life.Alive,
		UUID:         m.uuid,
		ModelType:    coremodel.IAAS,
		AgentVersion: version.Current,
		Cloud:        "my-cloud",
		CloudRegion:  "my-region",
		Credential: corecredential.Key{
			Cloud: "my-cloud",
			Owner: "test-user",
			Name:  "foobar",
		},
		Owner:     m.userUUID,
		OwnerName: m.userName,
	})
}
