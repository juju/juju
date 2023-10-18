// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	dbcloud "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modeltesting "github.com/juju/juju/domain/model/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type modelSuite struct {
	schematesting.ControllerSuite
	uuid model.UUID
}

var _ = gc.Suite(&modelSuite{})

func (m *modelSuite) SetUpTest(c *gc.C) {
	m.ControllerSuite.SetUpTest(c)

	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	err := cloudSt.UpsertCloud(context.Background(), cloud.Cloud{
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
		context.Background(), credential.ID{
			Cloud: "my-cloud",
			Owner: "wallyworld",
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
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: credential.ID{
				Cloud: "my-cloud",
				Owner: "wallyworld",
				Name:  "foobar",
			},
			Name:  "my-test-model",
			Owner: "wallyworld",
			Type:  model.TypeIAAS,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (m *modelSuite) TestCreateModelMetadataWithNoModel(c *gc.C) {
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
				Owner:       "wallyworld",
				Type:        model.TypeIAAS,
			},
			tx,
		)
	})
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (m *modelSuite) TestCreateModelMetadataWithExistingMetadata(c *gc.C) {
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
				Owner:       "wallyworld",
				Type:        model.TypeIAAS,
			},
			tx,
		)
	})
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (m *modelSuite) TestCreateModelWithSameNameAndOwner(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		context.Background(),
		testUUID,
		model.ModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Name:        "my-test-model",
			Owner:       "wallyworld",
			Type:        model.TypeIAAS,
		},
	)
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (m *modelSuite) TestCreateModelWithInvalidCloudRegion(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		context.Background(),
		testUUID,
		model.ModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "noexist",
			Name:        "noregion",
			Owner:       "wallyworld",
			Type:        model.TypeIAAS,
		},
	)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (m *modelSuite) TestCreateModelWithInvalidCloud(c *gc.C) {
	modelSt := NewState(m.TxnRunnerFactory())
	testUUID := modeltesting.GenModelUUID(c)
	err := modelSt.Create(
		context.Background(),
		testUUID,
		model.ModelCreationArgs{
			Cloud:       "noexist",
			CloudRegion: "my-region",
			Name:        "noregion",
			Owner:       "wallyworld",
			Type:        model.TypeIAAS,
		},
	)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (m *modelSuite) TestUpdateCredentialForDifferentCloud(c *gc.C) {
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
		context.Background(), credential.ID{
			Cloud: "my-cloud2",
			Owner: "wallyworld",
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, jc.ErrorIsNil)

	modelSt := NewState(m.TxnRunnerFactory())
	err = modelSt.UpdateCredential(
		context.Background(),
		m.uuid,
		credential.ID{
			Cloud: "my-cloud2",
			Owner: "wallyworld",
			Name:  "foobar",
		},
	)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (m *modelSuite) TestDeleteModel(c *gc.C) {
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
}

func (m *modelSuite) TestDeleteModelNotFound(c *gc.C) {
	uuid := modeltesting.GenModelUUID(c)
	modelSt := NewState(m.TxnRunnerFactory())
	err := modelSt.Delete(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}
