// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	accessstate "github.com/juju/juju/domain/access/state"
	cloudservice "github.com/juju/juju/domain/cloud/service"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	domainmodel "github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/model/service"
	"github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/domain/secretbackend/bootstrap"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
	userName user.Name
	userUUID user.UUID
}

var _ = gc.Suite(&watcherSuite{})

func Test(t *testing.T) {
	gc.TestingT(t)
}

func insertModelDependencies(c *gc.C, dbTxnRunnerFactory database.TxnRunnerFactory,
	dbTxnRunner database.TxnRunner, userUUID user.UUID, userName user.Name) {
	accessState := accessstate.NewState(dbTxnRunnerFactory, loggertesting.WrapCheckLog(c))

	// Add a user so we can set model owner.
	err := accessState.AddUser(
		context.Background(),
		userUUID,
		userName,
		userName.Name(),
		false,
		userUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Add a cloud so we can set the model cloud.
	cloudSt := cloudstate.NewState(dbTxnRunnerFactory)
	err = cloudSt.CreateCloud(context.Background(), userName, uuid.MustNewUUID().String(),
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

	// Add a cloud credential so we can set the model cloud credential.
	cred := credential.CloudCredentialInfo{
		Label:    "label",
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"attr1": "foo",
			"attr2": "bar",
		},
	}
	credSt := credentialstate.NewState(dbTxnRunnerFactory)
	_, err = credSt.UpsertCloudCredential(
		context.Background(), corecredential.Key{
			Cloud: "my-cloud",
			Owner: userName,
			Name:  "my-cloud-credential",
		},
		cred,
	)
	c.Assert(err, jc.ErrorIsNil)

	err = bootstrap.CreateDefaultBackends(coremodel.IAAS)(context.Background(), dbTxnRunner, dbTxnRunner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.userUUID = usertesting.GenUserUUID(c)
	s.userName = usertesting.GenNewName(c, "test-user")
	insertModelDependencies(c, s.TxnRunnerFactory(), s.TxnRunner(), s.userUUID, s.userName)
}

func (s *watcherSuite) TestWatchControllerDBModels(c *gc.C) {
	ctx := context.Background()
	logger := loggertesting.WrapCheckLog(c)
	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, logger)
	st := state.NewState(func() (database.TxnRunner, error) { return watchableDBFactory() })

	modelService := service.NewWatchableService(st, nil, loggertesting.WrapCheckLog(c), watcherFactory)

	// Create a controller service watcher.
	watcher, err := modelService.WatchActivatedModels(ctx)
	c.Assert(err, jc.ErrorIsNil)

	modelName := "test-model"
	var modelUUID coremodel.UUID
	var modelUUIDStr string
	var activateModel func(ctx context.Context) error

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Verifies that watchers do not receive any changes when newly unactivated models are created.
	harness.AddTest(func(c *gc.C) {
		// Create a new unactivated model named test-model.
		modelUUID, activateModel, err = modelService.CreateModel(ctx, domainmodel.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: s.userName,
				Name:  "my-cloud-credential",
			},
			Name:          modelName,
			Owner:         s.userUUID,
			SecretBackend: juju.BackendName,
		})
		c.Assert(err, jc.ErrorIsNil)
		modelUUIDStr = modelUUID.String()
	}, func(w watchertest.WatcherC[[]string]) {
		// Get the change.
		w.AssertNoChange()
	})

	// Verifies that watchers receive changes when models are activated.
	harness.AddTest(func(c *gc.C) {
		err := activateModel(ctx)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				modelUUIDStr,
			),
		)
	})

	// Verifies that watchers do not receive changes when entities other than models are updated.
	harness.AddTest(func(c *gc.C) {
		cloudState := cloudstate.NewState(func() (database.TxnRunner, error) { return watchableDBFactory() })
		cloudService := cloudservice.NewWatchableService(cloudState, watcherFactory)
		err := cloudService.UpdateCloud(ctx, cloud.Cloud{
			Name:      "my-cloud",
			Type:      "ec2",
			AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
			Regions: []cloud.Region{
				{
					Name: "my-region",
				},
			}})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Verifies that watchers do not receive changes when models are deleted.
	harness.AddTest(func(c *gc.C) {
		// Deletes model from table. This should not trigger a change event.
		err := modelService.DeleteModel(ctx, modelUUID)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string(nil))
}
