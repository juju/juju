// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	userstate "github.com/juju/juju/domain/access/state"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

// CreateTestModel is a testing utility function for creating a basic model for
// a test to rely on. The created model will have it's uuid returned.
func CreateTestModel(
	c *gc.C,
	txnRunner database.TxnRunnerFactory,
	name string,
) coremodel.UUID {
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userState := userstate.NewState(txnRunner, jujutesting.NewCheckLogger(c))
	err = userState.AddUser(
		context.Background(),
		userUUID,
		"test-user",
		"test-user",
		userUUID,
		permission.ControllerForAccess(permission.SuperuserAccess),
	)
	c.Assert(err, jc.ErrorIsNil)

	cloudSt := cloudstate.NewState(txnRunner)
	err = cloudSt.CreateCloud(context.Background(), "test-user",
		uuid.MustNewUUID().String(),
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

	cred := credential.CloudCredentialInfo{
		Label:    "foobar",
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}

	credSt := credentialstate.NewState(txnRunner)
	_, err = credSt.UpsertCloudCredential(
		context.Background(), corecredential.Key{
			Cloud: "my-cloud",
			Owner: "test-user",
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, jc.ErrorIsNil)

	modelUUID := modeltesting.GenModelUUID(c)
	modelSt := modelstate.NewState(txnRunner)
	err = modelSt.Create(
		context.Background(),
		modelUUID,
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
			Name:  name,
			Owner: userUUID,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	return modelUUID
}
