// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
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
	"github.com/juju/juju/domain/keymanager"
	keymanagerstate "github.com/juju/juju/domain/keymanager/state"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secretbackend/bootstrap"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerSuite

	uuid     coremodel.UUID
	userUUID user.UUID
	userName user.Name
}

var _ = gc.Suite(&stateSuite{})

// insertÇloud is a helper method to create new cloud's in the database during
// testing.
func (m *stateSuite) insertCloud(c *gc.C, cloud cloud.Cloud) {
	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	cloudUUID := uuid.MustNewUUID()
	err := cloudSt.CreateCloud(context.Background(), usertesting.GenNewName(c, "admin"), cloudUUID.String(), cloud)
	c.Assert(err, jc.ErrorIsNil)
}

func (m *stateSuite) SetUpTest(c *gc.C) {
	m.ControllerSuite.SetUpTest(c)

	// We need to generate a user in the database so that we can set the model
	// owner.
	m.uuid = modeltesting.GenModelUUID(c)
	//m.controllerUUID = m.SeedControllerTable(c, m.uuid)
	m.userName = usertesting.GenNewName(c, "test-user")
	accessState := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	//m.userUUID = m.createSuperuser(c, accessState, m.userName)

	m.userUUID = usertesting.GenUserUUID(c)
	err := accessState.AddUser(
		context.Background(),
		m.userUUID,
		m.userName,
		m.userName.Name(),
		false,
		m.userUUID,
	)
	c.Check(err, jc.ErrorIsNil)

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
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, jc.ErrorIsNil)
	_, err = credSt.UpsertCloudCredential(
		context.Background(), corecredential.Key{
			Cloud: "other-cloud",
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, jc.ErrorIsNil)

	err = bootstrap.CreateDefaultBackends(coremodel.IAAS)(context.Background(), m.ControllerTxnRunner(), m.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	modelSt := NewState(m.TxnRunnerFactory())
	err = modelSt.Create(
		context.Background(),
		m.uuid,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
func (s *stateSuite) TestCloudType(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
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
	cloudName, credentialID, err := st.GetModelCloudNameAndCredential(
		context.Background(),
		m.uuid,
	)
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
// common pattern that this state func will be used for so we have made a
// special case to continuously test this.
func (m *stateSuite) TestModelCloudNameAndCredentialController(c *gc.C) {
	st := NewState(m.TxnRunnerFactory())
	modelUUID := modeltesting.GenModelUUID(c)

	// We need to first inject a model that does not have a cloud credential set
	err := st.Create(
		context.Background(),
		modelUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud: "my-cloud",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: m.userName,
				Name:  "foobar",
			},
			Name:          coremodel.ControllerModelName,
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// We need to establish the fact that the model created above is in fact the
	// the controller model.
	m.ControllerSuite.SeedControllerTable(c, modelUUID)

	err = st.Activate(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)

	cloudName, credentialID, err := st.GetModelCloudNameAndCredential(
		context.Background(),
		modelUUID,
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
	noExistModelUUID := modeltesting.GenModelUUID(c)
	st := NewState(m.TxnRunnerFactory())
	cloudName, credentialID, err := st.GetModelCloudNameAndCredential(context.Background(), noExistModelUUID)
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
		UUID:        m.uuid,
		Cloud:       "my-cloud",
		CloudType:   "ec2",
		CloudRegion: "my-region",
		Credential: corecredential.Key{
			Cloud: "my-cloud",
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar",
		},
		Name:      "my-test-model",
		Owner:     m.userUUID,
		OwnerName: usertesting.GenNewName(c, "test-user"),
		ModelType: coremodel.IAAS,
		Life:      life.Alive,
	})
}

func (m *stateSuite) TestGetModelInfoNotActivated(c *gc.C) {
	runner := m.TxnRunnerFactory()

	modelUUID := modeltesting.GenModelUUID(c)

	modelSt := NewState(runner)
	err := modelSt.Create(
		context.Background(),
		modelUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Name:          "my-amazing-model",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	controllerUUID, err := uuid.UUIDFromString(m.SeedControllerUUID(c))
	c.Assert(err, jc.ErrorIsNil)

	userName, err := user.NewName("test-user")
	c.Assert(err, jc.ErrorIsNil)

	modelInfo, err := modelSt.GetModelInfo(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelInfo, gc.Equals, coremodel.ModelInfo{
		UUID:            modelUUID,
		ControllerUUID:  controllerUUID,
		Cloud:           "my-cloud",
		CloudType:       "ec2",
		CloudRegion:     "my-region",
		CredentialOwner: userName,
		CredentialName:  "foobar",
		Name:            "my-amazing-model",
		Type:            coremodel.IAAS,
	})
}

func (m *stateSuite) TestGetModelInfoActivated(c *gc.C) {
	runner := m.TxnRunnerFactory()

	userName, err := user.NewName("test-user")
	c.Assert(err, jc.ErrorIsNil)

	controllerUUID, err := uuid.UUIDFromString(m.SeedControllerUUID(c))
	c.Assert(err, jc.ErrorIsNil)

	modelSt := NewState(runner)
	modelInfo, err := modelSt.GetModelInfo(context.Background(), m.uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelInfo, gc.Equals, coremodel.ModelInfo{
		UUID:            m.uuid,
		ControllerUUID:  controllerUUID,
		Cloud:           "my-cloud",
		CloudType:       "ec2",
		CloudRegion:     "my-region",
		CredentialOwner: userName,
		CredentialName:  "foobar",
		Name:            "my-test-model",
		Type:            coremodel.IAAS,
	})
}

func (m *stateSuite) TestGetModelInfoNotFound(c *gc.C) {
	runner := m.TxnRunnerFactory()

	modelSt := NewState(runner)
	_, err := modelSt.GetModelInfo(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
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

// TestCreateModelWithExisting is testing that if we attempt to make a new model
// with the same uuid as one that already exists we get back a
// [modelerrors.AlreadyExists] error.
func (m *stateSuite) TestCreateModelWithExisting(c *gc.C) {
	runner, err := m.TxnRunnerFactory()()
	c.Assert(err, jc.ErrorIsNil)

	err = runner.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return createModel(
			ctx,
			preparer{},
			tx,
			m.uuid,
			coremodel.IAAS,
			model.GlobalModelCreationArgs{
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
		model.GlobalModelCreationArgs{
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
		model.GlobalModelCreationArgs{
			Cloud:         "my-cloud",
			CloudRegion:   "noexist",
			Name:          "noregion",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
}

func (m *stateSuite) TestCreateWithEmptyRegion(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		context.Background(),
		testUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud: "my-cloud",
			Name:  "noregion",
			Owner: m.userUUID,
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Name:        "controller",
			Owner:       m.userUUID,
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
		model.GlobalModelCreationArgs{
			Cloud: "my-cloud",
			Name:  "noregion",
			Owner: m.userUUID,
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Name:        "controller",
			Owner:       m.userUUID,
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
		model.GlobalModelCreationArgs{
			Cloud: "other-cloud",
			Name:  "noregion",
			Owner: m.userUUID,
			Credential: corecredential.Key{
				Cloud: "other-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
		model.GlobalModelCreationArgs{
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
	accessState := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := accessState.RemoveUser(context.Background(), m.userName)
	c.Assert(err, jc.ErrorIsNil)

	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		context.Background(),
		testUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
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
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
		model.GlobalModelCreationArgs{
			Cloud:         "noexist",
			CloudRegion:   "my-region",
			Name:          "noregion",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIs, clouderrors.NotFound)
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
			Owner: usertesting.GenNewName(c, "test-user"),
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
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar1",
		},
	)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
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
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, jc.ErrorIsNil)

	m.uuid = modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err = modelSt.Create(
		context.Background(),
		m.uuid,
		coremodel.CAAS,
		model.GlobalModelCreationArgs{
			Cloud: "minikube",
			Credential: corecredential.Key{
				Cloud: "minikube",
				Owner: usertesting.GenNewName(c, "test-user"),
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
//
// This test is also confirming cleaning up of other resources related to the
// model. Specifically:
// - Authorized keys onto the model.
func (m *stateSuite) TestDeleteModel(c *gc.C) {
	keyManagerState := keymanagerstate.NewState(m.TxnRunnerFactory())
	err := keyManagerState.AddPublicKeysForUser(
		context.Background(),
		m.uuid,
		m.userUUID,
		[]keymanager.PublicKey{
			{
				Comment:         "juju2@example.com",
				FingerprintHash: keymanager.FingerprintHashAlgorithmSHA256,
				Fingerprint:     "SHA256:+xUEnDVz0S//+1etL4rHjyopargd+HV78r0iRyx0cYw",
				Key:             "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8h8XBpjS9aBUG5cdoSWubs7wT2Lc/BEZIUQCqoaOZR juju2@example.com",
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	modelSt := NewState(m.TxnRunnerFactory())
	err = modelSt.Delete(
		context.Background(),
		m.uuid,
	)
	c.Assert(err, jc.ErrorIsNil)

	db := m.DB()
	row := db.QueryRowContext(
		context.Background(),
		"SELECT uuid FROM model WHERE uuid = ?",
		m.uuid,
	)
	var val string
	err = row.Scan(&val)
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)

	modelUUIDS, err := modelSt.ListModelIDs(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(modelUUIDS, gc.HasLen, 0)

	row = db.QueryRow(`
SELECT model_uuid
FROM model_authorized_keys
WHERE model_uuid = ?
	`, m.uuid)
	// ErrNoRows is not returned by row.Err, it is deferred until row.Scan
	// is called.
	c.Assert(row.Scan(nil), jc.ErrorIs, sql.ErrNoRows)
}

func (m *stateSuite) TestDeleteModelNotFound(c *gc.C) {
	uuid := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Delete(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestListModelIDs is testing that once we have created several models calling
// list returns all the models created.
func (m *stateSuite) TestListModelIDs(c *gc.C) {
	uuid1 := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Create(
		context.Background(),
		uuid1,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
	err := m.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		namespace, err = registerModelNamespace(ctx, preparer{}, tx, modelUUID)
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
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
			CloudType:   "ec2",
			CloudRegion: "my-region",
			ModelType:   coremodel.IAAS,
			Owner:       m.userUUID,
			OwnerName:   usertesting.GenNewName(c, "test-user"),
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Life: life.Alive,
		},
		{
			Name:        "owned1",
			UUID:        uuid1,
			Cloud:       "my-cloud",
			CloudType:   "ec2",
			CloudRegion: "my-region",
			ModelType:   coremodel.IAAS,
			Owner:       m.userUUID,
			OwnerName:   usertesting.GenNewName(c, "test-user"),
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Life: life.Alive,
		},
		{
			Name:        "owned2",
			UUID:        uuid2,
			Cloud:       "my-cloud",
			CloudType:   "ec2",
			CloudRegion: "my-region",
			ModelType:   coremodel.IAAS,
			Owner:       m.userUUID,
			OwnerName:   usertesting.GenNewName(c, "test-user"),
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
			CloudType:   "ec2",
			CloudRegion: "my-region",
			ModelType:   coremodel.IAAS,
			Owner:       m.userUUID,
			OwnerName:   usertesting.GenNewName(c, "test-user"),
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
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
	_, err := modelSt.GetModelByName(context.Background(), usertesting.GenNewName(c, "nonuser"), "my-test-model")
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)

	_, err = modelSt.GetModelByName(context.Background(), m.userName, "noexist")
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)

	_, err = modelSt.GetModelByName(context.Background(), usertesting.GenNewName(c, "nouser"), "noexist")
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
		Name:        "my-test-model",
		Life:        life.Alive,
		UUID:        m.uuid,
		ModelType:   coremodel.IAAS,
		Cloud:       "my-cloud",
		CloudType:   "ec2",
		CloudRegion: "my-region",
		Credential: corecredential.Key{
			Cloud: "my-cloud",
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar",
		},
		Owner:     m.userUUID,
		OwnerName: m.userName,
	})
}

// TestCleanupBrokenModel tests that when creation of a model fails (it is not
// activated), and the user tries to recreate the model with the same name, we
// can successfully clean up the broken model state and create the new model.
// This is a regression test for a bug in the original code, where State.Create
// was unable to clean up all the references to the original model.
// Bug report: https://bugs.launchpad.net/juju/+bug/2072601
func (m *stateSuite) TestCleanupBrokenModel(c *gc.C) {
	st := NewState(m.TxnRunnerFactory())

	// Create a "broken" model
	modelID := modeltesting.GenModelUUID(c)
	err := st.Create(
		context.Background(),
		modelID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Name:          "broken-model",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Suppose that model creation failed after the Create function was called,
	// and so the model was never activated. Now, the user tries to create a
	// new model with exactly the same name and owner.
	newModelID := modeltesting.GenModelUUID(c)
	err = st.Create(
		context.Background(),
		newModelID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Name:          "broken-model",
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

// TestIsControllerModelDDL is asserting the DDL that we have inside of the
// v_model view. v_model contains a column names "is_controller_model" that
// reports if the given model is the controller model etc. This is important for
// things like model summaries that need to know this information.
//
// For this test we want to assert that the value returns true and only true for
// for the model that is the controller.
func (m *stateSuite) TestIsControllerModelDDL(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	modelUUID := modeltesting.GenModelUUID(c)

	// We need to first inject a model that does not have a cloud credential set
	err := modelSt.Create(
		context.Background(),
		modelUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud: "my-cloud",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: m.userName,
				Name:  "foobar",
			},
			Name:          coremodel.ControllerModelName,
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// We need to establish the fact that the model created above is in fact the
	// the controller model.
	m.ControllerSuite.SeedControllerTable(c, modelUUID)

	err = modelSt.Activate(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)

	var isControllerModel bool
	err = m.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(
			"SELECT is_controller_model FROM v_model WHERE uuid = ?",
			modelUUID.String(),
		).Scan(&isControllerModel)
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(isControllerModel, jc.IsTrue)

	var count int
	err = m.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(
			"SELECT count(*) FROM v_model WHERE is_controller_model = false",
		).Scan(&count)
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, 1)

	// reset count
	count = 0
	err = m.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(
			"SELECT count(*) FROM v_model WHERE is_controller_model = true",
		).Scan(&count)
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, 1)
}

// TestGetControllerModel is asserting the happy path of
// [State.GetControllerModel] and checking that we can retrieve the controller
// model established in this test.
func (m *stateSuite) TestGetControllerModel(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	modelUUID := modeltesting.GenModelUUID(c)

	// We need to first inject a model that does not have a cloud credential set
	err := modelSt.Create(
		context.Background(),
		modelUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud: "my-cloud",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: m.userName,
				Name:  "foobar",
			},
			Name:          coremodel.ControllerModelName,
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// We need to establish the fact that the model created above is in fact the
	// the controller model.
	m.ControllerSuite.SeedControllerTable(c, modelUUID)

	err = modelSt.Activate(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)

	// The controller model uuid was set in SetUpTest.
	model, err := modelSt.GetControllerModel(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(model, gc.DeepEquals, coremodel.Model{
		Name:      coremodel.ControllerModelName,
		Life:      life.Alive,
		UUID:      modelUUID,
		ModelType: coremodel.IAAS,
		Cloud:     "my-cloud",
		CloudType: "ec2",
		Credential: corecredential.Key{
			Cloud: "my-cloud",
			Owner: m.userName,
			Name:  "foobar",
		},
		Owner:     m.userUUID,
		OwnerName: m.userName,
	})
}

// TestGetControllerModelNotFound is asserting that if we ask for the controller
// model from state and no controller model exists we get back an error that
// satisfies [modelerrors.NotFound].
func (m *stateSuite) TestGetControllerModelNotFound(c *gc.C) {
	_, err := NewState(m.TxnRunnerFactory()).GetControllerModel(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

func (m *stateSuite) TestListModelSummariesForUser(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	// Add a second model (one was added in SetUpTest).
	modelUUID := m.createTestModel(c, modelSt, "my-test-model-2", m.userUUID)
	controllerUUID := m.ControllerSuite.SeedControllerTable(c, modelUUID)

	accessState := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	expectedLoginTime := time.Now().Truncate(time.Minute).UTC()
	err := accessState.UpdateLastModelLogin(context.Background(), m.userName, m.uuid, expectedLoginTime)
	c.Assert(err, jc.ErrorIsNil)

	models, err := modelSt.ListModelSummariesForUser(context.Background(), usertesting.GenNewName(c, "test-user"))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(len(models), gc.Equals, 2)

	expected := []coremodel.UserModelSummary{
		{
			UserLastConnection: &expectedLoginTime,
			UserAccess:         permission.AdminAccess,
			ModelSummary: coremodel.ModelSummary{
				Name:        "my-test-model",
				UUID:        m.uuid,
				CloudName:   "my-cloud",
				CloudRegion: "my-region",
				CloudType:   "ec2",
				CloudCredentialKey: corecredential.Key{
					Cloud: "my-cloud",
					Owner: usertesting.GenNewName(c, "test-user"),
					Name:  "foobar",
				},
				ControllerUUID: controllerUUID,
				IsController:   false,
				// TODO (manadart 2024-01-29): We need to generate model summaries
				// with an agent version, but we can't do that from the controller
				// database.
				ModelType: coremodel.IAAS,
				OwnerName: usertesting.GenNewName(c, "test-user"),
				Life:      life.Alive,
			},
		},
		{
			UserLastConnection: nil,
			UserAccess:         permission.AdminAccess,
			ModelSummary: coremodel.ModelSummary{
				Name:        "my-test-model-2",
				UUID:        modelUUID,
				CloudName:   "my-cloud",
				CloudRegion: "my-region",
				CloudType:   "ec2",
				CloudCredentialKey: corecredential.Key{
					Cloud: "my-cloud",
					Owner: usertesting.GenNewName(c, "test-user"),
					Name:  "foobar",
				},
				ControllerUUID: controllerUUID,
				IsController:   true,
				// TODO (manadart 2024-01-29): We need to generate model summaries
				// with an agent version, but we can't do that from the controller
				// database.
				ModelType: coremodel.IAAS,
				OwnerName: usertesting.GenNewName(c, "test-user"),
				Life:      life.Alive,
			},
		},
	}

	sortFunc := func(a, b coremodel.UserModelSummary) int {
		return strings.Compare(a.Name, b.Name)
	}
	slices.SortFunc(models, sortFunc)
	slices.SortFunc(expected, sortFunc)

	c.Check(models, jc.DeepEquals, expected)
}

func (m *stateSuite) TestListModelSummariesForUserModelNotFound(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())

	_, err := modelSt.ListModelSummariesForUser(context.Background(), usertesting.GenNewName(c, "wrong-user"))
	c.Assert(err, jc.ErrorIsNil)
}

func (m *stateSuite) TestListAllModelSummaries(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	accessSt := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	newUserUUID := usertesting.GenUserUUID(c)
	newUserName := usertesting.GenNewName(c, "new-user")
	err := accessSt.AddUser(
		context.Background(),
		newUserUUID,
		newUserName,
		newUserName.Name(),
		false,
		newUserUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	modelUUID := m.createTestModel(c, modelSt, "ctrl-model", newUserUUID)
	controllerUUID := m.ControllerSuite.SeedControllerTable(c, modelUUID)
	models, err := modelSt.ListAllModelSummaries(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Check(len(models), gc.Equals, 2)

	expected := []coremodel.ModelSummary{
		{
			Name:        "my-test-model",
			UUID:        m.uuid,
			CloudName:   "my-cloud",
			CloudRegion: "my-region",
			CloudType:   "ec2",
			CloudCredentialKey: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			ControllerUUID: controllerUUID,
			IsController:   false,
			// TODO (manadart 2024-01-29): We need to generate model summaries
			// with an agent version, but we can't do that from the controller
			// database.
			ModelType: coremodel.IAAS,
			OwnerName: usertesting.GenNewName(c, "test-user"),
			Life:      life.Alive,
		},
		{
			Name:        "ctrl-model",
			UUID:        modelUUID,
			CloudName:   "my-cloud",
			CloudRegion: "my-region",
			CloudType:   "ec2",
			CloudCredentialKey: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			ControllerUUID: controllerUUID,
			IsController:   true,
			// TODO (manadart 2024-01-29): We need to generate model summaries
			// with an agent version, but we can't do that from the controller
			// database.
			ModelType: coremodel.IAAS,
			OwnerName: usertesting.GenNewName(c, "new-user"),
			Life:      life.Alive,
		},
	}

	sortFunc := func(a, b coremodel.ModelSummary) int {
		return strings.Compare(a.Name, b.Name)
	}
	slices.SortFunc(models, sortFunc)
	slices.SortFunc(expected, sortFunc)
	c.Check(models, gc.DeepEquals, expected)
}

func (s *stateSuite) TestGetModelUsers(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	accessState := accessstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	// Add test users.
	jimName := usertesting.GenNewName(c, "jim")
	bobName := usertesting.GenNewName(c, "bob")
	s.createModelUser(c, accessState, jimName, s.userUUID, permission.WriteAccess, s.uuid)
	s.createModelUser(c, accessState, bobName, s.userUUID, permission.ReadAccess, s.uuid)

	// Add and disabled/remove users to check they do not show up.
	disabledName := usertesting.GenNewName(c, "disabled-dude")
	removedName := usertesting.GenNewName(c, "removed-dude")
	s.createModelUser(c, accessState, disabledName, s.userUUID, permission.AdminAccess, s.uuid)
	s.createModelUser(c, accessState, removedName, s.userUUID, permission.AdminAccess, s.uuid)
	err := accessState.DisableUserAuthentication(context.Background(), disabledName)
	c.Assert(err, jc.ErrorIsNil)
	err = accessState.RemoveUser(context.Background(), removedName)
	c.Assert(err, jc.ErrorIsNil)

	modelUsers, err := st.GetModelUsers(context.Background(), s.uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelUsers, jc.SameContents, []coremodel.ModelUserInfo{
		{
			Name:           jimName,
			DisplayName:    jimName.Name(),
			Access:         permission.WriteAccess,
			LastModelLogin: time.Time{},
		},
		{
			Name:           bobName,
			DisplayName:    bobName.Name(),
			Access:         permission.ReadAccess,
			LastModelLogin: time.Time{},
		},
		{
			Name:           s.userName,
			DisplayName:    s.userName.Name(),
			Access:         permission.AdminAccess,
			LastModelLogin: time.Time{},
		},
	})
}

func (s *stateSuite) TestGetModelUsersModelNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetModelUsers(context.Background(), "bad-uuid")
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestGetModelStateModelNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	uuid := modeltesting.GenModelUUID(c)

	_, err := st.GetModelState(context.Background(), uuid)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestGetModelState is asserting the happy path of getting a model's state for
// status. The model is in a normal state and so we are asserting the response
// from the point of the model having nothing interesting to report.
func (s *stateSuite) TestGetModelState(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	mSt, err := st.GetModelState(context.Background(), s.uuid)
	c.Check(err, jc.ErrorIsNil)
	c.Check(mSt, gc.DeepEquals, model.ModelState{
		Destroying:                   false,
		Migrating:                    false,
		HasInvalidCloudCredential:    false,
		InvalidCloudCredentialReason: "",
	})
}

// TestGetModelStateinvalidCredentials is here to assert  that when the model's
// cloud credential is invalid, the model state is updated to indicate this with
// the invalid reason.
func (s *stateSuite) TestGetModelStateInvalidCredentials(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	m, err := st.GetModel(context.Background(), s.uuid)
	c.Assert(err, jc.ErrorIsNil)

	credentialSt := credentialstate.NewState(s.TxnRunnerFactory())
	err = credentialSt.InvalidateCloudCredential(
		context.Background(),
		m.Credential,
		"test-invalid",
	)
	c.Assert(err, jc.ErrorIsNil)

	mSt, err := st.GetModelState(context.Background(), s.uuid)
	c.Check(err, jc.ErrorIsNil)
	c.Check(mSt, gc.DeepEquals, model.ModelState{
		Destroying:                   false,
		Migrating:                    false,
		HasInvalidCloudCredential:    true,
		InvalidCloudCredentialReason: "test-invalid",
	})
}

// TestGetModelStateDestroying is asserting that when the model's life is set to
// destroying that the model state is updated to reflect this.
func (s *stateSuite) TestGetModelStateDestroying(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE model SET life_id = 1 WHERE uuid = ?
	`, s.uuid)
		return err
	})
	c.Check(err, jc.ErrorIsNil)

	mSt, err := st.GetModelState(context.Background(), s.uuid)
	c.Check(err, jc.ErrorIsNil)
	c.Check(mSt, gc.DeepEquals, model.ModelState{
		Destroying:                   true,
		Migrating:                    false,
		HasInvalidCloudCredential:    false,
		InvalidCloudCredentialReason: "",
	})
}

// TestGetEmptyCredentialsModel verifies that the model view continues to work correctly
// when the model's credentials are empty. This ensures that even in cases where a model
// is created without associated credentials (e.g., nil credential scenarios), the system
// behaves as expected.
//
// Specifically, the test validates that (for both IAAS and CAAS model types):
//  1. A model does not encounter errors when created without credentials.
//  2. Activating such a model does not cause unexpected errors.
//  3. Retrieving the model's state and properties works as expected even when credentials
//     are missing.
//
// This test addresses potential issues with SQL queries that include joins involving credential data.
// With this test, we can ensure that future modifications to the SQL layer do not unintentionally
// break functionality, preserving accessibility and consistent behavior for models with nil credentials.
func (m *stateSuite) TestGetEmptyCredentialsModel(c *gc.C) {
	// Define the test cases for different model types
	testCases := []struct {
		modelType coremodel.ModelType
		modelName string
	}{
		{
			modelType: coremodel.IAAS,
			modelName: "my-iaas-model",
		},
		{
			modelType: coremodel.CAAS,
			modelName: "my-container-model",
		},
	}

	for _, test := range testCases {
		modelState := NewState(m.TxnRunnerFactory())
		modelUUID := modeltesting.GenModelUUID(c)

		// Create model with empty credentials
		modelCreationArgs := model.GlobalModelCreationArgs{
			Cloud:         "my-cloud",
			CloudRegion:   "my-region",
			Name:          test.modelName,
			Owner:         m.userUUID,
			SecretBackend: juju.BackendName,
		}

		err := modelState.Create(context.Background(), modelUUID, test.modelType, modelCreationArgs)
		c.Assert(err, jc.ErrorIsNil)

		err = modelState.Activate(context.Background(), modelUUID)
		c.Assert(err, jc.ErrorIsNil)

		retrievedModel, err := modelState.GetModel(context.Background(), modelUUID)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(retrievedModel, gc.NotNil)

		c.Check(retrievedModel.Cloud, gc.Equals, modelCreationArgs.Cloud)
		c.Check(retrievedModel.CloudRegion, gc.Equals, modelCreationArgs.CloudRegion)
		c.Check(retrievedModel.Credential, jc.DeepEquals, modelCreationArgs.Credential)
		c.Check(retrievedModel.Name, gc.Equals, modelCreationArgs.Name)
		c.Check(retrievedModel.Owner, jc.DeepEquals, modelCreationArgs.Owner)
	}
}

// createSuperuser adds a new user with permissions on a model.
func (s *stateSuite) createModelUser(
	c *gc.C,
	accessState *accessstate.State,
	name user.Name,
	createdByUUID user.UUID,
	accessLevel permission.Access,
	modelUUID coremodel.UUID,
) user.UUID {
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	err = accessState.AddUserWithPermission(
		context.Background(),
		userUUID,
		name,
		name.Name(),
		false,
		createdByUUID,
		permission.AccessSpec{
			Access: accessLevel,
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        modelUUID.String(),
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	return userUUID
}

func (m *stateSuite) createTestModel(c *gc.C, modelSt *State, name string, creatorUUID user.UUID) coremodel.UUID {
	modelUUID := m.createTestModelWithoutActivation(c, modelSt, name, creatorUUID)
	c.Assert(modelSt.Activate(context.Background(), modelUUID), jc.ErrorIsNil)
	return modelUUID
}

func (m *stateSuite) createTestModelWithoutActivation(
	c *gc.C, modelSt *State, name string, creatorUUID user.UUID) coremodel.UUID {

	modelUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		context.Background(),
		modelUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Name:          name,
			Owner:         creatorUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	return modelUUID
}

// TestCloudSupportsAuthTypeTrue is asserting the happy path that for a valid
// cloud and supported auth type we get back true with no errors.
func (s *stateSuite) TestCloudSupportsAuthTypeTrue(c *gc.C) {
	fakeCloud := cloud.Cloud{
		Name:             "fluffy",
		Type:             "ec2",
		AuthTypes:        []cloud.AuthType{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Endpoint:         "https://endpoint",
		IdentityEndpoint: "https://identity-endpoint",
		StorageEndpoint:  "https://storage-endpoint",
		Regions: []cloud.Region{{
			Name:             "region1",
			Endpoint:         "http://region-endpoint1",
			IdentityEndpoint: "http://region-identity-endpoint1",
			StorageEndpoint:  "http://region-identity-endpoint1",
		}, {
			Name:             "region2",
			Endpoint:         "http://region-endpoint2",
			IdentityEndpoint: "http://region-identity-endpoint2",
			StorageEndpoint:  "http://region-identity-endpoint2",
		}},
		CACertificates:    []string{"cert1", "cert2"},
		SkipTLSVerify:     true,
		IsControllerCloud: false,
	}
	s.insertCloud(c, fakeCloud)

	st := NewState(s.TxnRunnerFactory())
	supports, err := st.CloudSupportsAuthType(context.Background(), fakeCloud.Name, cloud.UserPassAuthType)
	c.Check(err, jc.ErrorIsNil)
	c.Check(supports, jc.IsTrue)
}

// TestCloudSupportsAuthTypeFalse is asserting the happy path that for a valid
// cloud and a non supported auth type we get back false with no errors.
func (s *stateSuite) TestCloudSupportsAuthTypeFalse(c *gc.C) {
	fakeCloud := cloud.Cloud{
		Name:             "fluffy",
		Type:             "ec2",
		AuthTypes:        []cloud.AuthType{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Endpoint:         "https://endpoint",
		IdentityEndpoint: "https://identity-endpoint",
		StorageEndpoint:  "https://storage-endpoint",
		Regions: []cloud.Region{{
			Name:             "region1",
			Endpoint:         "http://region-endpoint1",
			IdentityEndpoint: "http://region-identity-endpoint1",
			StorageEndpoint:  "http://region-identity-endpoint1",
		}, {
			Name:             "region2",
			Endpoint:         "http://region-endpoint2",
			IdentityEndpoint: "http://region-identity-endpoint2",
			StorageEndpoint:  "http://region-identity-endpoint2",
		}},
		CACertificates:    []string{"cert1", "cert2"},
		SkipTLSVerify:     true,
		IsControllerCloud: false,
	}
	s.insertCloud(c, fakeCloud)

	st := NewState(s.TxnRunnerFactory())
	supports, err := st.CloudSupportsAuthType(context.Background(), fakeCloud.Name, cloud.CertificateAuthType)
	c.Check(err, jc.ErrorIsNil)
	c.Check(supports, jc.IsFalse)
}

// TestCloudSupportsAuthTypeCloudNotFound is checking to that if we ask if a
// cloud supports an auth type and the cloud doesn't exist we get back a
// [clouderrors.NotFound] error.
func (s *stateSuite) TestCloudSupportsAuthTypeCloudNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	supports, err := st.CloudSupportsAuthType(context.Background(), "no-exist", cloud.AuthType("no-exist"))
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)
	c.Check(supports, jc.IsFalse)
}

// TestGetControllerModelUUID is asserting the happy path of
// [State.GetControllerModelUUID] in that if a controller model exists we get
// back the uuid of the controller model.
func (s *stateSuite) TestGetControllerModelUUID(c *gc.C) {
	modelSt := NewState(s.TxnRunnerFactory())
	modelUUID := modeltesting.GenModelUUID(c)

	err := modelSt.Create(
		context.Background(),
		modelUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud: "my-cloud",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: s.userName,
				Name:  "foobar",
			},
			Name:          coremodel.ControllerModelName,
			Owner:         s.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// We need to establish the fact that the model created above is in fact the
	// the controller model.
	s.ControllerSuite.SeedControllerTable(c, modelUUID)
	err = modelSt.Activate(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := modelSt.GetControllerModelUUID(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(uuid, gc.DeepEquals, modelUUID)
}

// TestGetControllerModelUUIDNotFound is asserting that if we ask for the
// controller model uuid and no controller model exists we get back an error
// that satisfies [modelerrors.NotFound].
func (s *stateSuite) TestGetControllerModelUUIDNotFound(c *gc.C) {
	modelSt := NewState(s.TxnRunnerFactory())
	_, err := modelSt.GetControllerModelUUID(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestGetActivatedModelUUIDs asserts the behavior of
// [State.GetActivatedModelUUIDs] to ensure that only activated model UUIDs
// are returned. It verifies cases for activated, non-activated, and
// non-existent model UUIDs.
func (s *stateSuite) TestGetActivatedModelUUIDs(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Test no input model UUIDs.
	activatedModelUUIDs, err := st.GetActivatedModelUUIDs(context.Background(), []coremodel.UUID{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(activatedModelUUIDs, gc.HasLen, 0)

	// Test activated model UUID.
	activatedModelUUIDs, err = st.GetActivatedModelUUIDs(context.Background(), []coremodel.UUID{s.uuid})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(activatedModelUUIDs, gc.HasLen, 1)
	c.Check(activatedModelUUIDs[0], gc.Equals, s.uuid)

	// Test non-activated model UUID.
	unactivatedModelUUID := s.createTestModelWithoutActivation(c, st, "my-unactivated-model", s.userUUID)
	activatedModelUUIDs, err = st.GetActivatedModelUUIDs(context.Background(), []coremodel.UUID{unactivatedModelUUID})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(activatedModelUUIDs, gc.HasLen, 0)

	// Test non-existent model UUID.
	activatedModelUUIDs, err = st.GetActivatedModelUUIDs(context.Background(), []coremodel.UUID{"non-existent-uuid"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(activatedModelUUIDs, gc.HasLen, 0)

	// Test activated, non-activated and non-existent model UUIDs.
	activatedModelUUID := s.createTestModel(c, st, "my-activated-model", s.userUUID)
	activatedModelUUIDs, err = st.GetActivatedModelUUIDs(context.Background(),
		[]coremodel.UUID{s.uuid, activatedModelUUID, unactivatedModelUUID, "non-existent-uuid"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(activatedModelUUIDs, gc.HasLen, 2)
	c.Check(activatedModelUUIDs[0], gc.Equals, s.uuid)
	c.Check(activatedModelUUIDs[1], gc.Equals, activatedModelUUID)
}

func (s *stateSuite) TestGetModelLife(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	modelUUID := s.createTestModel(c, st, "my-unactivated-model", s.userUUID)

	result, err := st.GetModelLife(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, domainlife.Alive)
}

func (s *stateSuite) TestGetModelLifeDying(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	modelUUID := s.createTestModel(c, st, "my-unactivated-model", s.userUUID)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE model SET life_id = 1 WHERE uuid = ?
	`, modelUUID.String())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.GetModelLife(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, domainlife.Dying)
}

func (s *stateSuite) TestGetModelLifeNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	modelUUID := modeltesting.GenModelUUID(c)

	_, err := st.GetModelLife(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestGetModelLifeNotActivated(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	modelUUID := s.createTestModelWithoutActivation(c, st, "my-unactivated-model", s.userUUID)

	_, err := st.GetModelLife(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIs, modelerrors.NotActivated)
}
