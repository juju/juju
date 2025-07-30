// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	cloudtesting "github.com/juju/juju/core/cloud/testing"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
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
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
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
	changestreamtesting.ControllerSuite

	uuid coremodel.UUID
}

func (m *baseSuite) SetUpTest(c *tc.C) {
	m.ControllerSuite.SetUpTest(c)

	// We need to generate a user in the database so that we can set the model
	// owner.
	m.uuid = modeltesting.GenModelUUID(c)
	userName := usertesting.GenNewName(c, "test-user")
	accessState := accessstate.NewState(m.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	userUUID := usertesting.GenUserUUID(c)
	err := accessState.AddUser(
		c.Context(),
		userUUID,
		userName,
		userName.Name(),
		false,
		userUUID,
	)
	c.Check(err, tc.ErrorIsNil)

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

	modelSt := statecontroller.NewState(m.TxnRunnerFactory())
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
			Qualifier:     "prod",
			AdminUsers:    []user.UUID{userUUID},
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = modelSt.Activate(c.Context(), m.uuid)
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
