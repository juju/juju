// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	userbootstrap "github.com/juju/juju/domain/access/bootstrap"
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
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type modelConfigSuite struct {
	changestreamtesting.ControllerSuite
	changestreamtesting.ModelSuite

	modelID model.UUID
}

var _ = gc.Suite(&modelConfigSuite{})

func (s *modelConfigSuite) SetUpTest(c *gc.C) {
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
	modelFn := modelbootstrap.CreateModel(modelUUID, domainmodel.ModelCreationArgs{
		AgentVersion: jujuversion.Current,
		Cloud:        cloudName,
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
	c.Assert(err, jc.ErrorIsNil)

	err = modelbootstrap.CreateReadOnlyModel(modelUUID, uuid.MustNewUUID())(context.Background(), s.ControllerTxnRunner(), s.ModelTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelConfigSuite) TestWatchModelConfig(c *gc.C) {
	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	var defaults modelDefaultsProviderFunc = func(_ context.Context) (modeldefaults.Defaults, error) {
		return modeldefaults.Defaults{
			"foo": modeldefaults.DefaultAttributeValue{
				Source: config.JujuControllerSource,
				Value:  "bar",
			},
		}, nil
	}

	attrs := map[string]any{
		"agent-version":  jujuversion.Current.String(),
		"uuid":           s.modelID.String(),
		"type":           "iaas",
		"logging-config": "<root>=ERROR",
		"secret-backend": "auto",
	}

	st := state.NewState(s.ModelSuite.TxnRunnerFactory())
	ctrlSt := state.NewControllerState(s.ControllerSuite.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.ModelSuite.GetWatchableDB, s.modelID.String()),
		loggertesting.WrapCheckLog(c))
	svc := service.NewWatchableService(defaults, config.ModelValidator(), loggertesting.WrapCheckLog(c), ctrlSt, st, factory)

	watcher, err := svc.Watch()
	c.Assert(err, jc.ErrorIsNil)

	err = bootstrap.SetModelConfig(s.modelID, attrs, defaults)(ctx, s.ControllerTxnRunner(), s.ModelTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	w := watchertest.NewStringsWatcherC(c, watcher)

	// Changestream becomes idle and then we receive the bootstrap changes
	// from the model config.
	w.AssertChange("name", "uuid", "type", "foo", "logging-config", "secret-backend")

	// Ensure that the changestream is idle.
	s.ModelSuite.AssertChangeStreamIdle(c)

	// Now insert the change and watch it come through.
	attrs = make(map[string]any)
	attrs["logging-config"] = "<root>=WARNING"

	err = svc.UpdateModelConfig(ctx, attrs, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.ModelSuite.AssertChangeStreamIdle(c)

	w.AssertChange("logging-config")
}

type modelDefaultsProviderFunc func(context.Context) (modeldefaults.Defaults, error)

func (f modelDefaultsProviderFunc) ModelDefaults(
	c context.Context,
) (modeldefaults.Defaults, error) {
	return f(c)
}
