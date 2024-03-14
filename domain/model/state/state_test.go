// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/user"
	dbcloud "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	usererrors "github.com/juju/juju/domain/user/errors"
	userstate "github.com/juju/juju/domain/user/state"
	"github.com/juju/juju/version"
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
	userState := userstate.NewState(m.TxnRunnerFactory())
	err = userState.AddUser(
		context.Background(),
		m.userUUID,
		m.userName,
		m.userName,
		m.userUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	err = cloudSt.UpsertCloud(context.Background(), cloud.Cloud{
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
		context.Background(), corecredential.ID{
			Cloud: "my-cloud",
			Owner: "test-user",
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
		model.ModelCreationArgs{
			AgentVersion: version.Current,
			Cloud:        "my-cloud",
			CloudRegion:  "my-region",
			Credential: corecredential.ID{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:  "my-test-model",
			Owner: m.userUUID,
			Type:  coremodel.IAAS,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (m *stateSuite) TestGetModel(c *gc.C) {
	runner := m.TxnRunnerFactory()

	modelSt := NewState(runner)
	modelInfo, err := modelSt.Get(context.Background(), m.uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelInfo, gc.Equals, coremodel.Model{
		Cloud:       "my-cloud",
		CloudRegion: "my-region",
		Credential: corecredential.ID{
			Cloud: "my-cloud",
			Owner: "test-user",
			Name:  "foobar",
		},
		Name:      "my-test-model",
		Owner:     m.userUUID,
		ModelType: coremodel.IAAS,
	})
}

func (m *stateSuite) TestGetModelNotFound(c *gc.C) {
	runner := m.TxnRunnerFactory()

	modelSt := NewState(runner)

	_, err := modelSt.Get(context.Background(), modeltesting.GenModelUUID(c))
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
		return createModelAgent(context.Background(), testUUID, version.Current, tx)
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
		return createModelAgent(context.Background(), m.uuid, version.Current, tx)
	})

	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (m *stateSuite) TestCreateModelMetadataWithNoModel(c *gc.C) {
	runner, err := m.TxnRunnerFactory()()
	c.Assert(err, jc.ErrorIsNil)

	testUUID := modeltesting.GenModelUUID(c)
	err = runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return createModelMetadata(
			ctx,
			testUUID,
			model.ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "fantasticmodel",
				Owner:       m.userUUID,
				Type:        coremodel.IAAS,
			},
			tx,
		)
	})
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (m *stateSuite) TestCreateModelMetadataWithExistingMetadata(c *gc.C) {
	runner, err := m.TxnRunnerFactory()()
	c.Assert(err, jc.ErrorIsNil)

	err = runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return createModelMetadata(
			ctx,
			m.uuid,
			model.ModelCreationArgs{
				Cloud:       "my-cloud",
				CloudRegion: "my-region",
				Name:        "fantasticmodel",
				Owner:       m.userUUID,
				Type:        coremodel.IAAS,
			},
			tx,
		)
	})
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (m *stateSuite) TestCreateModelWithSameNameAndOwner(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		context.Background(),
		testUUID,
		model.ModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Name:        "my-test-model",
			Owner:       m.userUUID,
			Type:        coremodel.IAAS,
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
		model.ModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "noexist",
			Name:        "noregion",
			Owner:       m.userUUID,
			Type:        coremodel.IAAS,
		},
	)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
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
		model.ModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "noexist",
			Name:        "noregion",
			Owner:       user.UUID("noexist"), // does not exist
			Type:        coremodel.IAAS,
		},
	)
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

// TestCreateModelWithRemovedOwner is here to test that if we try and create a
// new model with an owner that has been removed from the Juju user base that
// the operation fails with a [usererrors.NotFound] error.
func (m *stateSuite) TestCreateModelWithRemovedOwner(c *gc.C) {
	userState := userstate.NewState(m.TxnRunnerFactory())
	err := userState.RemoveUser(context.Background(), m.userName)
	c.Assert(err, jc.ErrorIsNil)

	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		context.Background(),
		testUUID,
		model.ModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "noexist",
			Name:        "noregion",
			Owner:       m.userUUID,
			Type:        coremodel.IAAS,
		},
	)
	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

func (m *stateSuite) TestCreateModelWithInvalidCloud(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		context.Background(),
		testUUID,
		model.ModelCreationArgs{
			Cloud:       "noexist",
			CloudRegion: "my-region",
			Name:        "noregion",
			Owner:       m.userUUID,
			Type:        coremodel.IAAS,
		},
	)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (m *stateSuite) TestUpdateCredentialForDifferentCloud(c *gc.C) {
	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	err := cloudSt.UpsertCloud(context.Background(), cloud.Cloud{
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
		context.Background(), corecredential.ID{
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
		corecredential.ID{
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
	err := cloudSt.UpsertCloud(context.Background(), cloud.Cloud{
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
		context.Background(), corecredential.ID{
			Cloud: "minikube",
			Owner: "test-user",
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
		model.ModelCreationArgs{
			Cloud: "minikube",
			Credential: corecredential.ID{
				Cloud: "minikube",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:  "controller",
			Owner: m.userUUID,
			Type:  coremodel.CAAS,
		},
	)
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
			"SELECT model_uuid FROM model_metadata WHERE model_uuid = ?",
			m.uuid,
		)
		var val string
		err := row.Scan(&val)
		c.Assert(err, jc.ErrorIs, sql.ErrNoRows)

		row = tx.QueryRowContext(
			context.Background(),
			"SELECT uuid FROM model_list WHERE uuid = ?",
			m.uuid,
		)
		err = row.Scan(&val)
		c.Assert(err, jc.ErrorIs, sql.ErrNoRows)

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	modelUUIDS, err := modelSt.List(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(modelUUIDS, gc.HasLen, 0)
}

func (m *stateSuite) TestDeleteModelNotFound(c *gc.C) {
	uuid := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Delete(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestListModels is testing that once we have created several models calling
// list returns all of the models created.
func (m *stateSuite) TestListModels(c *gc.C) {
	uuid1 := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Create(
		context.Background(),
		uuid1,
		model.ModelCreationArgs{
			AgentVersion: version.Current,
			Cloud:        "my-cloud",
			CloudRegion:  "my-region",
			Credential: corecredential.ID{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:  "listtest1",
			Owner: m.userUUID,
			Type:  coremodel.IAAS,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	uuid2 := modeltesting.GenModelUUID(c)
	err = modelSt.Create(
		context.Background(),
		uuid2,
		model.ModelCreationArgs{
			AgentVersion: version.Current,
			Cloud:        "my-cloud",
			CloudRegion:  "my-region",
			Credential: corecredential.ID{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:  "listtest2",
			Owner: m.userUUID,
			Type:  coremodel.IAAS,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	uuids, err := modelSt.List(context.Background())
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
