// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	userbootstrap "github.com/juju/juju/domain/access/bootstrap"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	credentialbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	"github.com/juju/juju/domain/model"
	modelbootstrap "github.com/juju/juju/domain/model/bootstrap"
	"github.com/juju/juju/domain/model/state/testing"
	"github.com/juju/juju/domain/modeldefaults"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/environs/config"
	jujuversion "github.com/juju/juju/version"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
	schematesting.ModelSuite

	modelID coremodel.UUID
}

type ModelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

var _ = gc.Suite(&bootstrapSuite{})

func (f ModelDefaultsProviderFunc) ModelDefaults(
	c context.Context,
) (modeldefaults.Defaults, error) {
	return f(c)
}

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.ModelSuite.SetUpTest(c)

	userID, fn := userbootstrap.AddUser(coreuser.AdminUserName, permission.ControllerForAccess(permission.SuperuserAccess))
	err := fn(context.Background(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	cloudName := "test"
	fn = cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      cloudName,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
	})

	err = fn(context.Background(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	credentialName := "test"
	fn = credentialbootstrap.InsertCredential(credential.Key{
		Cloud: cloudName,
		Name:  credentialName,
		Owner: coreuser.AdminUserName,
	},
		cloud.NewCredential(cloud.EmptyAuthType, nil),
	)

	err = fn(context.Background(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	testing.CreateInternalSecretBackend(c, s.ControllerTxnRunner())

	modelUUID := modeltesting.GenModelUUID(c)
	modelFn := modelbootstrap.CreateModel(model.ModelCreationArgs{
		AgentVersion: jujuversion.Current,
		Cloud:        cloudName,
		Credential: credential.Key{
			Cloud: cloudName,
			Name:  credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:  "test",
		Owner: userID,
		UUID:  modelUUID,
	})
	s.modelID = modelUUID

	err = modelFn(context.Background(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bootstrapSuite) TestSetModelConfig(c *gc.C) {
	var defaults ModelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{
			"foo": modeldefaults.DefaultAttributeValue{
				Source: config.JujuControllerSource,
				Value:  "bar",
			},
		}, nil
	}

	//cfg, err := config.New(config.NoDefaults, map[string]any{
	//	"name": "wallyworld",
	//	"uuid": "a677bdfd-3c96-46b2-912f-38e25faceaf7",
	//	"type": "sometype",
	//})
	//c.Assert(err, jc.ErrorIsNil)

	err := SetModelConfig(s.modelID, nil, defaults)(context.Background(), s.ControllerTxnRunner(), s.ModelTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	rows, err := s.ModelSuite.DB().Query("SELECT * FROM model_config")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	configVals := map[string]string{}
	var k, v string
	for rows.Next() {
		err = rows.Scan(&k, &v)
		c.Assert(err, jc.ErrorIsNil)
		configVals[k] = v
	}

	c.Assert(rows.Err(), jc.ErrorIsNil)
	c.Assert(configVals, jc.DeepEquals, map[string]string{
		"name":           "test",
		"uuid":           s.modelID.String(),
		"type":           "iaas",
		"foo":            "bar",
		"logging-config": "<root>=INFO",
		"secret-backend": "auto",
	})
}
