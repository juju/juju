// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	accessstate "github.com/juju/juju/domain/access/state"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	credentialbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	domainmodel "github.com/juju/juju/domain/model"
	modelbootstrap "github.com/juju/juju/domain/model/bootstrap"
	"github.com/juju/juju/domain/model/state/testing"
	"github.com/juju/juju/domain/modelconfig/bootstrap"
	"github.com/juju/juju/domain/modelconfig/service"
	"github.com/juju/juju/domain/modelconfig/state"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs/config"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type modelConfigSuite struct {
	changestreamtesting.ControllerModelSuite

	modelID model.UUID
}

var _ = tc.Suite(&modelConfigSuite{})

func (s *modelConfigSuite) SetUpTest(c *tc.C) {
	s.ControllerModelSuite.SetUpTest(c)

	controllerUUID := s.SeedControllerUUID(c)

	userID, err := coreuser.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	accessState := accessstate.NewState(s.ControllerSuite.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = accessState.AddUserWithPermission(
		context.Background(), userID,
		coreuser.AdminUserName,
		coreuser.AdminUserName.Name(),
		false,
		userID,
		permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        controllerUUID,
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	cloudName := "test"
	fn := cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      cloudName,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
	})

	err = fn(context.Background(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	credentialName := "test"
	fn = credentialbootstrap.InsertCredential(credential.Key{
		Cloud: cloudName,
		Name:  credentialName,
		Owner: coreuser.AdminUserName,
	},
		cloud.NewCredential(cloud.EmptyAuthType, nil),
	)

	err = fn(context.Background(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	testing.CreateInternalSecretBackend(c, s.ControllerTxnRunner())

	modelUUID := modeltesting.GenModelUUID(c)
	modelFn := modelbootstrap.CreateGlobalModelRecord(modelUUID, domainmodel.GlobalModelCreationArgs{
		Cloud: cloudName,
		Credential: credential.Key{
			Cloud: cloudName,
			Name:  credentialName,
			Owner: coreuser.AdminUserName,
		},
		Name:  "test",
		Owner: userID,
	})
	s.modelID = modelUUID

	err = modelFn(context.Background(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	err = modelbootstrap.CreateLocalModelRecord(modelUUID, uuid.MustNewUUID(), jujuversion.Current)(
		context.Background(), s.ControllerTxnRunner(), s.ModelTxnRunner(c, modelUUID.String()))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelConfigSuite) TestWatchModelConfig(c *tc.C) {
	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	var defaults modelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{
			"foo": modeldefaults.DefaultAttributeValue{
				Controller: "bar",
			},
		}, nil
	}

	attrs := map[string]any{
		"agent-version":  jujuversion.Current.String(),
		"uuid":           s.modelID.String(),
		"type":           "iaas",
		"logging-config": "<root>=ERROR",
	}

	modelTxnRunnerFactory := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(c, string(s.modelID)), nil
	}

	_, idler := s.InitWatchableDB(c, s.modelID.String())

	st := state.NewState(modelTxnRunnerFactory)
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelID.String()),
		loggertesting.WrapCheckLog(c))
	svc := service.NewWatchableService(defaults, config.ModelValidator(), st, factory)

	watcher, err := svc.Watch()
	c.Assert(err, tc.ErrorIsNil)

	err = bootstrap.SetModelConfig(s.modelID, attrs, defaults)(ctx, s.ControllerTxnRunner(), s.ModelTxnRunner(c, s.modelID.String()))
	c.Assert(err, tc.ErrorIsNil)

	w := watchertest.NewStringsWatcherC(c, watcher)

	// Changestream becomes idle and then we receive the bootstrap changes
	// from the model config.
	w.AssertChange("name", "uuid", "type", "foo", "logging-config")

	// Ensure that the changestream is idle.
	idler.AssertChangeStreamIdle(c)

	// Now insert the change and watch it come through.
	attrs["logging-config"] = "<root>=WARNING"

	err = svc.SetModelConfig(ctx, attrs)
	c.Assert(err, tc.ErrorIsNil)

	idler.AssertChangeStreamIdle(c)

	w.AssertChange("logging-config")
}

type modelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

func (f modelDefaultsProviderFunc) ModelDefaults(
	c context.Context,
) (modeldefaults.Defaults, error) {
	return f(c)
}
