// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	corecloud "github.com/juju/juju/core/cloud"
	cloudtesting "github.com/juju/juju/core/cloud/testing"
	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accesserrors "github.com/juju/juju/domain/access/errors"
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

	cloudUUID      corecloud.UUID
	credentialUUID corecredential.UUID
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

// insertÇloud is a helper method to create new cloud's in the database during
// testing.
func (m *stateSuite) insertCloud(c *tc.C, cloud cloud.Cloud) {
	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	cloudUUID := uuid.MustNewUUID()
	err := cloudSt.CreateCloud(c.Context(), usertesting.GenNewName(c, "admin"), cloudUUID.String(), cloud)
	c.Assert(err, tc.ErrorIsNil)
}

func (m *stateSuite) SetUpTest(c *tc.C) {
	m.ControllerSuite.SetUpTest(c)

	// We need to generate a user in the database so that we can set the model
	// owner.
	m.uuid = modeltesting.GenModelUUID(c)
	m.userName = usertesting.GenNewName(c, "test-user")
	accessState := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	m.userUUID = usertesting.GenUserUUID(c)
	err := accessState.AddUser(
		c.Context(),
		m.userUUID,
		m.userName,
		m.userName.Name(),
		false,
		m.userUUID,
	)
	c.Check(err, tc.ErrorIsNil)

	// We need to generate a cloud in the database so that we can set the model
	// cloud.
	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	m.cloudUUID = cloudtesting.GenCloudUUID(c)
	err = cloudSt.CreateCloud(c.Context(), m.userName, m.cloudUUID.String(),
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
	c.Assert(err, tc.ErrorIsNil)
	err = cloudSt.CreateCloud(c.Context(), m.userName, uuid.MustNewUUID().String(),
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
	c.Assert(err, tc.ErrorIsNil)

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
	err = credSt.UpsertCloudCredential(
		c.Context(), corecredential.Key{
			Cloud: "my-cloud",
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, tc.ErrorIsNil)
	err = m.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT uuid FROM cloud_credential WHERE owner_uuid = ? AND name = ? AND cloud_uuid = ?", m.userUUID, "foobar", m.cloudUUID).
			Scan(&m.credentialUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	err = credSt.UpsertCloudCredential(
		c.Context(), corecredential.Key{
			Cloud: "other-cloud",
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, tc.ErrorIsNil)

	err = bootstrap.CreateDefaultBackends(coremodel.IAAS)(c.Context(), m.ControllerTxnRunner(), m.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	modelSt := NewState(m.TxnRunnerFactory())
	err = modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	err = modelSt.Activate(c.Context(), m.uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// TestCloudType is testing the happy path of [CloudType] to make sure we get
// back the correct type of a cloud.
func (s *stateSuite) TestCloudType(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctype, err := st.CloudType(c.Context(), "my-cloud")
	c.Check(err, tc.ErrorIsNil)
	c.Check(ctype, tc.Equals, "ec2")
}

// TestCloudTypeMissing is testing that if we ask for a cloud type of a cloud
// that does not exist we get back an error that satisfies
// [clouderrors.NotFound].
func (m *stateSuite) TestCloudTypeMissing(c *tc.C) {
	st := NewState(m.TxnRunnerFactory())
	ctype, err := st.CloudType(c.Context(), "no-exist-cloud")
	c.Check(err, tc.ErrorIs, clouderrors.NotFound)
	c.Check(ctype, tc.Equals, "")
}

// TestModelCloudInfo tests the happy path for getting a models
// cloud name and credential.
func (m *stateSuite) TestModelCloudInfo(c *tc.C) {
	st := NewState(m.TxnRunnerFactory())
	// We are relying on the model setup as part of this suite.
	cloudName, regionName, err := st.GetModelCloudInfo(
		c.Context(),
		m.uuid,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cloudName, tc.Equals, "my-cloud")
	c.Check(regionName, tc.Equals, "my-region")
}

// TestModelCloudInfoController is testing the cloud name and cloud region returned for
// the controller model and owner. This is the common pattern that this state func
// will be used for so we have made a special case to continuously test this.
func (m *stateSuite) TestModelCloudInfoController(c *tc.C) {
	st := NewState(m.TxnRunnerFactory())
	modelUUID := modeltesting.GenModelUUID(c)

	// We need to first inject a model that does not have a cloud credential set
	err := st.Create(
		c.Context(),
		modelUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
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
	c.Assert(err, tc.ErrorIsNil)

	// We need to establish the fact that the model created above is in fact the
	// the controller model.
	m.ControllerSuite.SeedControllerTable(c, modelUUID)

	err = st.Activate(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	cloudName, regionName, err := st.GetModelCloudInfo(
		c.Context(),
		modelUUID,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(cloudName, tc.Equals, "my-cloud")
	c.Check(regionName, tc.Equals, "my-region")
}

// TestModelCloudInfoNotFound is testing that if we pass a model
// that doesn't exist we get back a [modelerrors.NotFound] error.
func (m *stateSuite) TestModelCloudInfoNotFound(c *tc.C) {
	noExistModelUUID := modeltesting.GenModelUUID(c)
	st := NewState(m.TxnRunnerFactory())
	cloudName, regionName, err := st.GetModelCloudInfo(c.Context(), noExistModelUUID)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
	c.Check(cloudName, tc.Equals, "")
	c.Check(regionName, tc.Equals, "")
}

func (m *stateSuite) TestGetModelCloudAndCredential(c *tc.C) {
	st := NewState(m.TxnRunnerFactory())
	// We are relying on the model setup as part of this suite.
	cloudUUID, credentialUUID, err := st.GetModelCloudAndCredential(
		c.Context(),
		m.uuid,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cloudUUID, tc.Equals, m.cloudUUID)
	c.Check(credentialUUID, tc.Equals, m.credentialUUID)
}

func (m *stateSuite) TestGetModelCloudAndCredentialNotFound(c *tc.C) {
	st := NewState(m.TxnRunnerFactory())
	_, _, err := st.GetModelCloudAndCredential(
		c.Context(),
		modeltesting.GenModelUUID(c),
	)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (m *stateSuite) TestGetModel(c *tc.C) {
	runner := m.TxnRunnerFactory()

	modelSt := NewState(runner)
	modelInfo, err := modelSt.GetModel(c.Context(), m.uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelInfo, tc.Equals, coremodel.Model{
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
		Qualifier: m.userName.String(),
		ModelType: coremodel.IAAS,
		Life:      corelife.Alive,
	})
}

// TestGetModelSeedInformationNotActivated tests that
// [State.GetModelSeedInformation] return information about a model that is not
// yet activated.
func (m *stateSuite) TestGetModelSeedInformationNotActivated(c *tc.C) {
	runner := m.TxnRunnerFactory()

	modelUUID := modeltesting.GenModelUUID(c)

	modelSt := NewState(runner)
	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)
	controllerUUID, err := uuid.UUIDFromString(m.SeedControllerUUID(c))
	c.Assert(err, tc.ErrorIsNil)

	userName, err := user.NewName("test-user")
	c.Assert(err, tc.ErrorIsNil)

	modelInfo, err := modelSt.GetModelSeedInformation(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelInfo, tc.Equals, coremodel.ModelInfo{
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

// TestGetModelInfoActivated tests that [State.GetModelSeedInformation] returns
// information about a model when it is activated.
func (m *stateSuite) TestGetModelSeedInformationActivated(c *tc.C) {
	runner := m.TxnRunnerFactory()

	userName, err := user.NewName("test-user")
	c.Assert(err, tc.ErrorIsNil)

	controllerUUID, err := uuid.UUIDFromString(m.SeedControllerUUID(c))
	c.Assert(err, tc.ErrorIsNil)

	modelSt := NewState(runner)
	modelInfo, err := modelSt.GetModelSeedInformation(c.Context(), m.uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelInfo, tc.Equals, coremodel.ModelInfo{
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

// TestGetModelSeedInformationNotFound tests that when asking for seed
// information on a model that does not exist not just non activated we get an
// error satisfying [modelerrors.NotFound] back.
func (m *stateSuite) TestGetModelSeedInformationNotFound(c *tc.C) {
	runner := m.TxnRunnerFactory()

	modelUUID := modeltesting.GenModelUUID(c)
	modelSt := NewState(runner)
	_, err := modelSt.GetModelSeedInformation(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (m *stateSuite) TestGetModelNotFound(c *tc.C) {
	runner := m.TxnRunnerFactory()
	modelSt := NewState(runner)
	_, err := modelSt.GetModel(c.Context(), modeltesting.GenModelUUID(c))
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestCreateModelWithExisting is testing that if we attempt to make a new model
// with the same uuid as one that already exists we get back a
// [modelerrors.AlreadyExists] error.
func (m *stateSuite) TestCreateModelWithExisting(c *tc.C) {
	runner, err := m.TxnRunnerFactory()()
	c.Assert(err, tc.ErrorIsNil)

	err = runner.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
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
	c.Assert(err, tc.ErrorIs, modelerrors.AlreadyExists)
}

// TestCreateModelWithSameNameAndOwner is testing that we attempt to create a
// new model with a different uuid but the same owner and name as one that
// exists we get back a [modelerrors.AlreadyExists] error.
func (m *stateSuite) TestCreateModelWithSameNameAndOwner(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIs, modelerrors.AlreadyExists)
}

func (m *stateSuite) TestCreateModelWithInvalidCloudRegion(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (m *stateSuite) TestCreateWithEmptyRegion(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	err = modelSt.Activate(c.Context(), testUUID)
	c.Assert(err, tc.ErrorIsNil)

	modelInfo, err := modelSt.GetModel(c.Context(), testUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelInfo.CloudRegion, tc.Equals, "")
}

func (m *stateSuite) TestCreateWithEmptyRegionUsesControllerRegion(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())

	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	testUUID := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	err = modelSt.Activate(c.Context(), testUUID)
	c.Assert(err, tc.ErrorIsNil)

	modelInfo, err := modelSt.GetModel(c.Context(), testUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelInfo.CloudRegion, tc.Equals, "my-region")
}

func (m *stateSuite) TestCreateWithEmptyRegionDoesNotUseControllerRegionForDifferentCloudNames(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())

	controllerUUID := modeltesting.GenModelUUID(c)

	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	err = modelSt.Activate(c.Context(), controllerUUID)
	c.Assert(err, tc.ErrorIsNil)

	modelInfo, err := modelSt.GetModel(c.Context(), controllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelInfo.CloudRegion, tc.Equals, "my-region")

	testUUID := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	err = modelSt.Activate(c.Context(), testUUID)
	c.Assert(err, tc.ErrorIsNil)

	modelInfo, err = modelSt.GetModel(c.Context(), testUUID)
	c.Assert(err, tc.ErrorIsNil)

	// We should never set the region to the controller region if the cloud
	// names are different.

	c.Check(modelInfo.CloudRegion, tc.Equals, "")
}

// TestCreateModelWithNonExistentOwner is here to assert that if we try and make
// a model with a user/owner that does not exist a [accesserrors.NotFound] error
// is returned.
func (m *stateSuite) TestCreateModelWithNonExistentOwner(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestCreateModelWithRemovedOwner is here to test that if we try and create a
// new model with an owner that has been removed from the Juju user base that
// the operation fails with a [accesserrors.NotFound] error.
func (m *stateSuite) TestCreateModelWithRemovedOwner(c *tc.C) {
	accessState := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := accessState.RemoveUser(c.Context(), m.userName)
	c.Assert(err, tc.ErrorIsNil)

	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestCreateModelVerifyPermissionSet is here to test that a permission is
// created for the owning user when a model is created.
func (m *stateSuite) TestCreateModelVerifyPermissionSet(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	ctx := c.Context()
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
	c.Assert(err, tc.ErrorIsNil)

	accessSt := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	access, err := accessSt.ReadUserAccessLevelForTarget(ctx, m.userName, permission.ID{
		ObjectType: permission.Model,
		Key:        m.uuid.String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(access, tc.Equals, permission.AdminAccess)
}

func (m *stateSuite) TestCreateModelWithInvalidCloud(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIs, clouderrors.NotFound)
}

func (m *stateSuite) TestUpdateCredentialForDifferentCloud(c *tc.C) {
	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	err := cloudSt.CreateCloud(c.Context(), m.userName, uuid.MustNewUUID().String(),
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
	c.Assert(err, tc.ErrorIsNil)

	cred := credential.CloudCredentialInfo{
		Label:    "foobar",
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}

	credSt := credentialstate.NewState(m.TxnRunnerFactory())
	err = credSt.UpsertCloudCredential(
		c.Context(), corecredential.Key{
			Cloud: "my-cloud2",
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar1",
		},
		cred,
	)
	c.Assert(err, tc.ErrorIsNil)

	modelSt := NewState(m.TxnRunnerFactory())
	err = modelSt.UpdateCredential(
		c.Context(),
		m.uuid,
		corecredential.Key{
			Cloud: "my-cloud2",
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar1",
		},
	)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// We are trying to test here that we can set a cloud credential on the model
// for the same cloud as the model when no cloud region has been set. This is a
// regression test discovered during DQlite development where we messed up the
// logic assuming that a cloud region was always set for a model.
func (m *stateSuite) TestSetModelCloudCredentialWithoutRegion(c *tc.C) {
	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	err := cloudSt.CreateCloud(c.Context(), m.userName, uuid.MustNewUUID().String(),
		cloud.Cloud{
			Name:      "minikube",
			Type:      "kubernetes",
			AuthTypes: cloud.AuthTypes{cloud.UserPassAuthType},
			Regions:   []cloud.Region{},
		})
	c.Assert(err, tc.ErrorIsNil)

	cred := credential.CloudCredentialInfo{
		Label:    "foobar",
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"username": "myuser",
			"password": "secret",
		},
	}

	credSt := credentialstate.NewState(m.TxnRunnerFactory())
	err = credSt.UpsertCloudCredential(
		c.Context(), corecredential.Key{
			Cloud: "minikube",
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, tc.ErrorIsNil)

	m.uuid = modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err = modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	err = modelSt.Activate(c.Context(), m.uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// TestDeleteModel tests that we can delete a model that is already created in
// the system. We also confirm that list models returns no models after the
// deletion.
//
// This test is also confirming cleaning up of other resources related to the
// model. Specifically:
// - Authorized keys onto the model.
func (m *stateSuite) TestDeleteModel(c *tc.C) {
	keyManagerState := keymanagerstate.NewState(m.TxnRunnerFactory())
	err := keyManagerState.AddPublicKeysForUser(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	modelSt := NewState(m.TxnRunnerFactory())
	err = modelSt.Delete(
		c.Context(),
		m.uuid,
	)
	c.Assert(err, tc.ErrorIsNil)

	db := m.DB()
	row := db.QueryRowContext(
		c.Context(),
		"SELECT uuid FROM model WHERE uuid = ?",
		m.uuid,
	)
	var val string
	err = row.Scan(&val)
	c.Assert(err, tc.ErrorIs, sql.ErrNoRows)

	modelUUIDS, err := modelSt.ListModelUUIDs(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(modelUUIDS, tc.HasLen, 0)

	row = db.QueryRow(`
SELECT model_uuid
FROM model_authorized_keys
WHERE model_uuid = ?
	`, m.uuid)
	// ErrNoRows is not returned by row.Err, it is deferred until row.Scan
	// is called.
	c.Assert(row.Scan(nil), tc.ErrorIs, sql.ErrNoRows)
}

func (m *stateSuite) TestDeleteModelNotFound(c *tc.C) {
	uuid := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Delete(c.Context(), uuid)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestListModelUUIDs is testing that once we have created several models calling
// list returns all the models created.
func (m *stateSuite) TestListModelUUIDs(c *tc.C) {
	uuid1 := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)
	err = modelSt.Activate(c.Context(), uuid1)
	c.Assert(err, tc.ErrorIsNil)

	uuid2 := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)
	err = modelSt.Activate(c.Context(), uuid2)
	c.Assert(err, tc.ErrorIsNil)

	uuids, err := modelSt.ListModelUUIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuids, tc.HasLen, 3)

	uuidsSet := set.NewStrings()
	for _, uuid := range uuids {
		uuidsSet.Add(uuid.String())
	}

	c.Check(uuidsSet.Contains(uuid1.String()), tc.IsTrue)
	c.Check(uuidsSet.Contains(uuid2.String()), tc.IsTrue)
	c.Check(uuidsSet.Contains(m.uuid.String()), tc.IsTrue)
}

// TestRegisterModelNamespaceNotFound is asserting that when we ask for the
// namespace of a model that doesn't exist we get back a [modelerrors.NotFound]
// error.
func (m *stateSuite) TestRegisterModelNamespaceNotFound(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	var namespace string
	err := m.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		namespace, err = registerModelNamespace(ctx, preparer{}, tx, modelUUID)
		return err
	})
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
	c.Check(namespace, tc.Equals, "")
}

// TestNamespaceForModelNoModel is asserting that when we ask for a models
// database namespace and the model doesn't exist we get back a
// [modelerrors.NotFound] error.
func (m *stateSuite) TestNamespaceForModelNoModel(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)

	st := NewState(m.TxnRunnerFactory())
	namespace, err := st.NamespaceForModel(c.Context(), modelUUID)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
	c.Check(namespace, tc.Equals, "")
}

// TestNamespaceForModel is testing the happy path for a successful model
// creation that a namespace is returned with no errors.
func (m *stateSuite) TestNamespaceForModel(c *tc.C) {
	st := NewState(m.TxnRunnerFactory())
	namespace, err := st.NamespaceForModel(c.Context(), m.uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(namespace, tc.Equals, m.uuid.String())
}

// TestNamespaceForModelDeleted tests that after we have deleted a model we can
// no longer get back the database namespace.
func (m *stateSuite) TestNamespaceForModelDeleted(c *tc.C) {
	st := NewState(m.TxnRunnerFactory())
	err := st.Delete(c.Context(), m.uuid)
	c.Assert(err, tc.ErrorIsNil)

	namespace, err := st.NamespaceForModel(c.Context(), m.uuid)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
	c.Check(namespace, tc.Equals, "")
}

// TestListUserModelUUIDsUserNotFound tests that if a caller asks for the model
// uuids a user has access to and that user doesn't exist the caller will get
// back an error that satisfies [accesserrors.UserNotFound].
func (m *stateSuite) TestListUserModelUUIDsUserNotFound(c *tc.C) {
	fakeUserUUID := usertesting.GenUserUUID(c)

	modelSt := NewState(m.TxnRunnerFactory())
	_, err := modelSt.ListModelUUIDsForUser(c.Context(), fakeUserUUID)
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestListUserModelUUIDs is testing the happy path of getting all of the model
// uuids that a given user has access to.
func (m *stateSuite) TestListUserModelUUIDs(c *tc.C) {
	// Make the first model that is not owned by the user we will be testing
	// with.
	modelUUID1 := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Create(
		c.Context(),
		modelUUID1,
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelSt.Activate(c.Context(), modelUUID1), tc.ErrorIsNil)

	// Make test user to use for the final check.
	user2UUID := usertesting.GenUserUUID(c)
	user2Name := usertesting.GenNewName(c, "foo")
	accessState := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = accessState.AddUser(
		c.Context(),
		user2UUID,
		user2Name,
		user2Name.Name(),
		false,
		m.userUUID,
	)
	c.Check(err, tc.ErrorIsNil)

	// Make second model owned by the test user.
	modelUUID2 := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		c.Context(),
		modelUUID2,
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
			Owner:         user2UUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelSt.Activate(c.Context(), modelUUID2), tc.ErrorIsNil)

	// Add test user as an admin of test model 1.
	permissionID, err := uuid.NewUUID()
	c.Check(err, tc.ErrorIsNil)
	_, err = accessState.CreatePermission(
		c.Context(),
		permissionID, permission.UserAccessSpec{
			AccessSpec: permission.AccessSpec{
				Target: permission.ID{
					ObjectType: permission.Model,
					Key:        modelUUID1.String(),
				},
				Access: permission.AdminAccess,
			},
			User: user2Name,
		},
	)
	c.Check(err, tc.ErrorIsNil)

	// Check that the test user has access to both models.
	uuids, err := modelSt.ListModelUUIDsForUser(c.Context(), user2UUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(uuids, tc.SameContents, []coremodel.UUID{modelUUID1, modelUUID2})
}

// TestModelsOwnedByUser is asserting that all models owned by a given user are
// returned in the resultant list.
func (m *stateSuite) TestModelsOwnedByUser(c *tc.C) {
	uuid1 := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelSt.Activate(c.Context(), uuid1), tc.ErrorIsNil)

	uuid2 := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelSt.Activate(c.Context(), uuid2), tc.ErrorIsNil)

	models, err := modelSt.ListModelsForUser(c.Context(), m.userUUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(len(models), tc.Equals, 3)
	slices.SortFunc(models, func(a, b coremodel.Model) int {
		return strings.Compare(a.Name, b.Name)
	})

	c.Check(models, tc.DeepEquals, []coremodel.Model{
		{
			Name:        "my-test-model",
			UUID:        m.uuid,
			Cloud:       "my-cloud",
			CloudType:   "ec2",
			CloudRegion: "my-region",
			ModelType:   coremodel.IAAS,
			Qualifier:   m.userName.String(),
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Life: corelife.Alive,
		},
		{
			Name:        "owned1",
			UUID:        uuid1,
			Cloud:       "my-cloud",
			CloudType:   "ec2",
			CloudRegion: "my-region",
			ModelType:   coremodel.IAAS,
			Qualifier:   m.userName.String(),
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Life: corelife.Alive,
		},
		{
			Name:        "owned2",
			UUID:        uuid2,
			Cloud:       "my-cloud",
			CloudType:   "ec2",
			CloudRegion: "my-region",
			ModelType:   coremodel.IAAS,
			Qualifier:   m.userName.String(),
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Life: corelife.Alive,
		},
	})
}

// TestModelsOwnedByNonExistantUser tests that if we ask for models from a non
// existent user we get back an empty model list.
func (m *stateSuite) TestModelsOwnedByNonExistantUser(c *tc.C) {
	userID := usertesting.GenUserUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())

	models, err := modelSt.ListModelsForUser(c.Context(), userID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(models), tc.Equals, 0)
}

func (m *stateSuite) TestAllModels(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	models, err := modelSt.ListAllModels(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(models, tc.DeepEquals, []coremodel.Model{
		{
			Name:        "my-test-model",
			UUID:        m.uuid,
			Cloud:       "my-cloud",
			CloudType:   "ec2",
			CloudRegion: "my-region",
			ModelType:   coremodel.IAAS,
			Qualifier:   m.userName.String(),
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Life: corelife.Alive,
		},
	})
}

// TestSecretBackendNotFoundForModelCreate is testing that if we specify a
// secret backend that doesn't exist during model creation we back an error that
// satisfies [secretbackenderrors.NotFound]
func (m *stateSuite) TestSecretBackendNotFoundForModelCreate(c *tc.C) {
	uuid := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Create(
		c.Context(),
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
	c.Check(err, tc.ErrorIs, secretbackenderrors.NotFound)
}

// TestGetModelByNameNotFound is here to assert that if we try and get a model
// by name for any combination of user or model name that doesn't exist we get
// back an error that satisfies [modelerrors.NotFound].
func (m *stateSuite) TestGetModelByNameNotFound(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	_, err := modelSt.GetModelByName(c.Context(), usertesting.GenNewName(c, "nonuser"), "my-test-model")
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)

	_, err = modelSt.GetModelByName(c.Context(), m.userName, "noexist")
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)

	_, err = modelSt.GetModelByName(c.Context(), usertesting.GenNewName(c, "nouser"), "noexist")
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetModelByName is asserting the happy path of [State.GetModelByName] and
// checking that we can retrieve the model established in SetUpTest by username
// and model name.
func (m *stateSuite) TestGetModelByName(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	model, err := modelSt.GetModelByName(c.Context(), m.userName, "my-test-model")
	c.Check(err, tc.ErrorIsNil)
	c.Check(model, tc.DeepEquals, coremodel.Model{
		Name:        "my-test-model",
		Life:        corelife.Alive,
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
		Qualifier: m.userName.String(),
	})
}

// TestCleanupBrokenModel tests that when creation of a model fails (it is not
// activated), and the user tries to recreate the model with the same name, we
// can successfully clean up the broken model state and create the new model.
// This is a regression test for a bug in the original code, where State.Create
// was unable to clean up all the references to the original model.
// Bug report: https://bugs.launchpad.net/juju/+bug/2072601
func (m *stateSuite) TestCleanupBrokenModel(c *tc.C) {
	st := NewState(m.TxnRunnerFactory())

	// Create a "broken" model
	modelID := modeltesting.GenModelUUID(c)
	err := st.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	// Suppose that model creation failed after the Create function was called,
	// and so the model was never activated. Now, the user tries to create a
	// new model with exactly the same name and owner.
	newModelID := modeltesting.GenModelUUID(c)
	err = st.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)
}

// TestIsControllerModelDDL is asserting the DDL that we have inside of the
// v_model view. v_model contains a column names "is_controller_model" that
// reports if the given model is the controller model etc. This is important for
// things like model summaries that need to know this information.
//
// For this test we want to assert that the value returns true and only true for
// for the model that is the controller.
func (m *stateSuite) TestIsControllerModelDDL(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	modelUUID := modeltesting.GenModelUUID(c)

	// We need to first inject a model that does not have a cloud credential set
	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	// We need to establish the fact that the model created above is in fact the
	// the controller model.
	m.ControllerSuite.SeedControllerTable(c, modelUUID)

	err = modelSt.Activate(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	var isControllerModel bool
	err = m.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(
			"SELECT is_controller_model FROM v_model WHERE uuid = ?",
			modelUUID.String(),
		).Scan(&isControllerModel)
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(isControllerModel, tc.IsTrue)

	var count int
	err = m.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(
			"SELECT count(*) FROM v_model WHERE is_controller_model = false",
		).Scan(&count)
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	// reset count
	count = 0
	err = m.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(
			"SELECT count(*) FROM v_model WHERE is_controller_model = true",
		).Scan(&count)
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

// TestGetControllerModel is asserting the happy path of
// [State.GetControllerModel] and checking that we can retrieve the controller
// model established in this test.
func (m *stateSuite) TestGetControllerModel(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	modelUUID := modeltesting.GenModelUUID(c)

	// We need to first inject a model that does not have a cloud credential set
	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	// We need to establish the fact that the model created above is in fact the
	// the controller model.
	m.ControllerSuite.SeedControllerTable(c, modelUUID)

	err = modelSt.Activate(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// The controller model uuid was set in SetUpTest.
	model, err := modelSt.GetControllerModel(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(model, tc.DeepEquals, coremodel.Model{
		Name:      coremodel.ControllerModelName,
		Life:      corelife.Alive,
		UUID:      modelUUID,
		ModelType: coremodel.IAAS,
		Cloud:     "my-cloud",
		CloudType: "ec2",
		Credential: corecredential.Key{
			Cloud: "my-cloud",
			Owner: m.userName,
			Name:  "foobar",
		},
		Qualifier: m.userName.String(),
	})
}

// TestGetControllerModelNotFound is asserting that if we ask for the controller
// model from state and no controller model exists we get back an error that
// satisfies [modelerrors.NotFound].
func (m *stateSuite) TestGetControllerModelNotFound(c *tc.C) {
	_, err := NewState(m.TxnRunnerFactory()).GetControllerModel(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

func (m *stateSuite) TestGetUserModelSummary(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	// Add a second model (one was added in SetUpTest).
	modelUUID := m.createTestModel(c, modelSt, "my-test-model-2", m.userUUID)
	expectedLoginTime := time.Now().Truncate(time.Minute).UTC()
	accessState := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := accessState.UpdateLastModelLogin(c.Context(), m.userName, modelUUID, expectedLoginTime)
	c.Assert(err, tc.ErrorIsNil)

	summary, err := modelSt.GetUserModelSummary(c.Context(), m.userUUID, modelUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(summary, tc.DeepEquals, model.UserModelSummary{
		ModelSummary: model.ModelSummary{
			Life:      corelife.Alive,
			OwnerName: m.userName,
			State: model.ModelState{
				Destroying:                   false,
				HasInvalidCloudCredential:    false,
				InvalidCloudCredentialReason: "",
				Migrating:                    false,
			},
		},
		UserAccess:         permission.AdminAccess,
		UserLastConnection: &expectedLoginTime,
	})
}

// TestGetUserModelSummaryUserNotFound tests that asking for a model summary for
// a user that doesn't exist results in a [accesserrors.UserNotFound] error.
func (m *stateSuite) TestGetUserModelSummaryUserNotFound(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	_, err := modelSt.GetUserModelSummary(
		c.Context(),
		usertesting.GenUserUUID(c),
		m.uuid,
	)
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestGetUserModelSummaryModelNotFound tests that asking for a model summary
// for a model that doesn't exist results in a [modelerrors.NotFound] error.
func (m *stateSuite) TestGetUserModelSummaryModelNotFound(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	_, err := modelSt.GetUserModelSummary(
		c.Context(),
		m.userUUID,
		modeltesting.GenModelUUID(c),
	)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetUserModelSummaryNoAccess tests that asking for a model summary for a
// model that the user doesn't have access to results in a
// [accesserrors.AccessNotFound] error.
func (m *stateSuite) TestGetUserModelSummaryNoAccess(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")
	accessSt := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := accessSt.AddUser(
		c.Context(),
		userUUID,
		userName,
		userName.Name(),
		false,
		userUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = modelSt.GetUserModelSummary(c.Context(), userUUID, m.uuid)
	c.Check(err, tc.ErrorIs, accesserrors.AccessNotFound)
}

// TestGetModelSummary is asserting the happy path of [State.GetModelSummary].
func (m *stateSuite) TestGetModelSummary(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	summary, err := modelSt.GetModelSummary(c.Context(), m.uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(summary, tc.DeepEquals, model.ModelSummary{
		Life:      corelife.Alive,
		OwnerName: m.userName,
		State: model.ModelState{
			Destroying:                   false,
			Migrating:                    false,
			InvalidCloudCredentialReason: "",
			HasInvalidCloudCredential:    false,
		},
	})
}

// TestGetModelSummaryModelNotFound is asserting that if we ask for a model
// summary on a model that doesn't exist we get a [modelerrors.NotFound] error.
func (m *stateSuite) TestGetModelSummaryModelNotFound(c *tc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	_, err := modelSt.GetModelSummary(c.Context(), modeltesting.GenModelUUID(c))
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestGetModelUsers(c *tc.C) {
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
	err := accessState.DisableUserAuthentication(c.Context(), disabledName)
	c.Assert(err, tc.ErrorIsNil)
	err = accessState.RemoveUser(c.Context(), removedName)
	c.Assert(err, tc.ErrorIsNil)

	modelUsers, err := st.GetModelUsers(c.Context(), s.uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(modelUsers, tc.SameContents, []coremodel.ModelUserInfo{
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

func (s *stateSuite) TestGetModelUsersModelNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetModelUsers(c.Context(), "bad-uuid")
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestGetModelStateModelNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	uuid := modeltesting.GenModelUUID(c)

	_, err := st.GetModelState(c.Context(), uuid)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetModelState is asserting the happy path of getting a model's state for
// status. The model is in a normal state and so we are asserting the response
// from the point of the model having nothing interesting to report.
func (s *stateSuite) TestGetModelState(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	mSt, err := st.GetModelState(c.Context(), s.uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(mSt, tc.DeepEquals, model.ModelState{
		Destroying:                   false,
		Migrating:                    false,
		HasInvalidCloudCredential:    false,
		InvalidCloudCredentialReason: "",
	})
}

// TestGetModelStateinvalidCredentials is here to assert  that when the model's
// cloud credential is invalid, the model state is updated to indicate this with
// the invalid reason.
func (s *stateSuite) TestGetModelStateInvalidCredentials(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	m, err := st.GetModel(c.Context(), s.uuid)
	c.Assert(err, tc.ErrorIsNil)

	credentialSt := credentialstate.NewState(s.TxnRunnerFactory())
	err = credentialSt.InvalidateModelCloudCredential(
		c.Context(),
		m.UUID,
		"test-invalid",
	)
	c.Assert(err, tc.ErrorIsNil)

	mSt, err := st.GetModelState(c.Context(), s.uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(mSt, tc.DeepEquals, model.ModelState{
		Destroying:                   false,
		Migrating:                    false,
		HasInvalidCloudCredential:    true,
		InvalidCloudCredentialReason: "test-invalid",
	})
}

// TestGetModelStateDestroying is asserting that when the model's life is set to
// destroying that the model state is updated to reflect this.
func (s *stateSuite) TestGetModelStateDestroying(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE model SET life_id = 1 WHERE uuid = ?
	`, s.uuid)
		return err
	})
	c.Check(err, tc.ErrorIsNil)

	mSt, err := st.GetModelState(c.Context(), s.uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(mSt, tc.DeepEquals, model.ModelState{
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
func (m *stateSuite) TestGetEmptyCredentialsModel(c *tc.C) {
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

		err := modelState.Create(c.Context(), modelUUID, test.modelType, modelCreationArgs)
		c.Assert(err, tc.ErrorIsNil)

		err = modelState.Activate(c.Context(), modelUUID)
		c.Assert(err, tc.ErrorIsNil)

		retrievedModel, err := modelState.GetModel(c.Context(), modelUUID)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(retrievedModel, tc.NotNil)

		c.Check(retrievedModel.Cloud, tc.Equals, modelCreationArgs.Cloud)
		c.Check(retrievedModel.CloudRegion, tc.Equals, modelCreationArgs.CloudRegion)
		c.Check(retrievedModel.Credential, tc.DeepEquals, modelCreationArgs.Credential)
		c.Check(retrievedModel.Name, tc.Equals, modelCreationArgs.Name)
		c.Check(retrievedModel.Qualifier, tc.Equals, m.userName.String())
	}
}

// createSuperuser adds a new user with permissions on a model.
func (s *stateSuite) createModelUser(
	c *tc.C,
	accessState *accessstate.State,
	name user.Name,
	createdByUUID user.UUID,
	accessLevel permission.Access,
	modelUUID coremodel.UUID,
) user.UUID {
	userUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	err = accessState.AddUserWithPermission(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)
	return userUUID
}

func (m *stateSuite) createTestModel(c *tc.C, modelSt *State, name string, creatorUUID user.UUID) coremodel.UUID {
	modelUUID := m.createTestModelWithoutActivation(c, modelSt, name, creatorUUID)
	c.Assert(modelSt.Activate(c.Context(), modelUUID), tc.ErrorIsNil)
	return modelUUID
}

func (m *stateSuite) createTestModelWithoutActivation(
	c *tc.C, modelSt *State, name string, creatorUUID user.UUID) coremodel.UUID {

	modelUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)
	return modelUUID
}

// TestCloudSupportsAuthTypeTrue is asserting the happy path that for a valid
// cloud and supported auth type we get back true with no errors.
func (s *stateSuite) TestCloudSupportsAuthTypeTrue(c *tc.C) {
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
	supports, err := st.CloudSupportsAuthType(c.Context(), fakeCloud.Name, cloud.UserPassAuthType)
	c.Check(err, tc.ErrorIsNil)
	c.Check(supports, tc.IsTrue)
}

// TestCloudSupportsAuthTypeFalse is asserting the happy path that for a valid
// cloud and a non supported auth type we get back false with no errors.
func (s *stateSuite) TestCloudSupportsAuthTypeFalse(c *tc.C) {
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
	supports, err := st.CloudSupportsAuthType(c.Context(), fakeCloud.Name, cloud.CertificateAuthType)
	c.Check(err, tc.ErrorIsNil)
	c.Check(supports, tc.IsFalse)
}

// TestCloudSupportsAuthTypeCloudNotFound is checking to that if we ask if a
// cloud supports an auth type and the cloud doesn't exist we get back a
// [clouderrors.NotFound] error.
func (s *stateSuite) TestCloudSupportsAuthTypeCloudNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	supports, err := st.CloudSupportsAuthType(c.Context(), "no-exist", cloud.AuthType("no-exist"))
	c.Check(err, tc.ErrorIs, clouderrors.NotFound)
	c.Check(supports, tc.IsFalse)
}

// TestGetControllerModelUUID is asserting the happy path of
// [State.GetControllerModelUUID] in that if a controller model exists we get
// back the uuid of the controller model.
func (s *stateSuite) TestGetControllerModelUUID(c *tc.C) {
	modelSt := NewState(s.TxnRunnerFactory())
	modelUUID := modeltesting.GenModelUUID(c)

	err := modelSt.Create(
		c.Context(),
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
	c.Assert(err, tc.ErrorIsNil)

	// We need to establish the fact that the model created above is in fact the
	// the controller model.
	s.ControllerSuite.SeedControllerTable(c, modelUUID)
	err = modelSt.Activate(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := modelSt.GetControllerModelUUID(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(uuid, tc.DeepEquals, modelUUID)
}

// TestGetControllerModelUUIDNotFound is asserting that if we ask for the
// controller model uuid and no controller model exists we get back an error
// that satisfies [modelerrors.NotFound].
func (s *stateSuite) TestGetControllerModelUUIDNotFound(c *tc.C) {
	modelSt := NewState(s.TxnRunnerFactory())
	_, err := modelSt.GetControllerModelUUID(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetActivatedModelUUIDs asserts the behavior of
// [State.GetActivatedModelUUIDs] to ensure that only activated model UUIDs
// are returned. It verifies cases for activated, non-activated, and
// non-existent model UUIDs.
func (s *stateSuite) TestGetActivatedModelUUIDs(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Test no input model UUIDs.
	activatedModelUUIDs, err := st.GetActivatedModelUUIDs(c.Context(), []coremodel.UUID{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(activatedModelUUIDs, tc.HasLen, 0)

	// Test activated model UUID.
	activatedModelUUIDs, err = st.GetActivatedModelUUIDs(c.Context(), []coremodel.UUID{s.uuid})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(activatedModelUUIDs, tc.HasLen, 1)
	c.Check(activatedModelUUIDs[0], tc.Equals, s.uuid)

	// Test non-activated model UUID.
	unactivatedModelUUID := s.createTestModelWithoutActivation(c, st, "my-unactivated-model", s.userUUID)
	activatedModelUUIDs, err = st.GetActivatedModelUUIDs(c.Context(), []coremodel.UUID{unactivatedModelUUID})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(activatedModelUUIDs, tc.HasLen, 0)

	// Test non-existent model UUID.
	activatedModelUUIDs, err = st.GetActivatedModelUUIDs(c.Context(), []coremodel.UUID{"non-existent-uuid"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(activatedModelUUIDs, tc.HasLen, 0)

	// Test activated, non-activated and non-existent model UUIDs.
	activatedModelUUID := s.createTestModel(c, st, "my-activated-model", s.userUUID)
	activatedModelUUIDs, err = st.GetActivatedModelUUIDs(c.Context(),
		[]coremodel.UUID{s.uuid, activatedModelUUID, unactivatedModelUUID, "non-existent-uuid"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(activatedModelUUIDs, tc.HasLen, 2)
	c.Check(activatedModelUUIDs[0], tc.Equals, s.uuid)
	c.Check(activatedModelUUIDs[1], tc.Equals, activatedModelUUID)
}

func (s *stateSuite) TestGetModelLife(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	modelUUID := s.createTestModel(c, st, "my-unactivated-model", s.userUUID)

	result, err := st.GetModelLife(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, domainlife.Alive)
}

func (s *stateSuite) TestGetModelLifeDying(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	modelUUID := s.createTestModel(c, st, "my-unactivated-model", s.userUUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE model SET life_id = 1 WHERE uuid = ?
	`, modelUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	result, err := st.GetModelLife(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, domainlife.Dying)
}

func (s *stateSuite) TestGetModelLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	modelUUID := modeltesting.GenModelUUID(c)

	_, err := st.GetModelLife(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestGetModelLifeNotActivated(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	modelUUID := s.createTestModelWithoutActivation(c, st, "my-unactivated-model", s.userUUID)

	_, err := st.GetModelLife(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, modelerrors.NotActivated)
}

func (s *stateSuite) TestCheckModelExists(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	exists, err := st.CheckModelExists(c.Context(), s.uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
}

func (s *stateSuite) TestCheckModelDoesNotExist(c *tc.C) {
	uuid := modeltesting.GenModelUUID(c)
	st := NewState(s.TxnRunnerFactory())
	exists, err := st.CheckModelExists(c.Context(), uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

func (s *stateSuite) TestCheckModelExistsNotActivated(c *tc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	modelSt := NewState(s.TxnRunnerFactory())
	err := modelSt.Create(
		c.Context(),
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
			Owner:         s.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Check(err, tc.ErrorIsNil)

	exists, err := modelSt.CheckModelExists(c.Context(), modelUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}
