// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	cloudtesting "github.com/juju/juju/core/cloud/testing"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accessstate "github.com/juju/juju/domain/access/state"
	dbcloud "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/model"
	statecontroller "github.com/juju/juju/domain/model/state/controller"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secretbackend/bootstrap"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

type baseSuite struct {
	schematesting.ControllerSuite

	uuid coremodel.UUID
}

func (m *baseSuite) SetUpTest(c *tc.C) {
	m.ControllerSuite.SetUpTest(c)

	// We need to generate a user in the database so that we can set the model
	// owner.
	m.uuid = modeltesting.GenModelUUID(c)
	userName := usertesting.GenNewName(c, "test-user")
	accessState := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	adminUserUUID := usertesting.GenUserUUID(c)
	err := accessState.AddUser(
		c.Context(),
		adminUserUUID,
		user.AdminUserName,
		"admin",
		false,
		adminUserUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	everyoneExternalUUID := usertesting.GenUserUUID(c)
	err = accessState.AddUser(
		c.Context(),
		everyoneExternalUUID,
		permission.EveryoneUserName,
		"",
		true,
		adminUserUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	userUUID := usertesting.GenUserUUID(c)
	err = accessState.AddUser(
		c.Context(),
		userUUID,
		userName,
		userName.Name(),
		false,
		everyoneExternalUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We need to generate a cloud in the database so that we can set the model
	// cloud.
	cloudSt := dbcloud.NewState(m.TxnRunnerFactory())
	cloudUUID := cloudtesting.GenCloudUUID(c)
	err = cloudSt.CreateCloud(c.Context(), userName, cloudUUID.String(),
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
	err = cloudSt.CreateCloud(c.Context(), userName, uuid.MustNewUUID().String(),
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

	err = m.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err := statecontroller.Create(
			ctx,
			preparer{},
			tx,
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
				Qualifier:     "prod",
				AdminUsers:    []user.UUID{userUUID},
				SecretBackend: juju.BackendName,
			},
		)
		if err != nil {
			return err
		}

		activator := statecontroller.GetActivator()
		return activator(ctx, preparer{}, tx, m.uuid)
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) advanceModelLife(c *tc.C, modelUUID string, newLife life.Life) {
	_, err := s.DB().Exec("UPDATE model SET life_id = ? WHERE uuid = ?", newLife, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) checkModelLife(c *tc.C, modelUUID string, expectedLife life.Life) {
	row := s.DB().QueryRow("SELECT life_id FROM model WHERE uuid = ?", modelUUID)
	var lifeID int
	err := row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, int(expectedLife))
}

type preparer struct{}

func (p preparer) Prepare(query string, args ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, args...)
}
