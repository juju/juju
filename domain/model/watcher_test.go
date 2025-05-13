// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"database/sql"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
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
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
	userName user.Name
	userUUID user.UUID
}

var _ = tc.Suite(&watcherSuite{})

func insertModelDependencies(c *tc.C, dbTxnRunnerFactory database.TxnRunnerFactory,
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
	c.Assert(err, tc.ErrorIsNil)

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

	c.Assert(err, tc.ErrorIsNil)

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
	err = credSt.UpsertCloudCredential(
		context.Background(), corecredential.Key{
			Cloud: "my-cloud",
			Owner: userName,
			Name:  "my-cloud-credential",
		},
		cred,
	)
	c.Assert(err, tc.ErrorIsNil)

	err = bootstrap.CreateDefaultBackends(coremodel.IAAS)(context.Background(), dbTxnRunner, dbTxnRunner)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.userUUID = usertesting.GenUserUUID(c)
	s.userName = usertesting.GenNewName(c, "test-user")
	insertModelDependencies(c, s.TxnRunnerFactory(), s.TxnRunner(), s.userUUID, s.userName)
}

func (s *watcherSuite) TestWatchControllerDBModels(c *tc.C) {
	ctx := context.Background()

	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, loggertesting.WrapCheckLog(c))
	st := state.NewState(func() (database.TxnRunner, error) { return watchableDBFactory() })

	modelService := service.NewWatchableService(st, nil, loggertesting.WrapCheckLog(c), watcherFactory)

	// Create a controller service watcher.
	watcher, err := modelService.WatchActivatedModels(ctx)
	c.Assert(err, tc.ErrorIsNil)

	modelName := "test-model"
	var modelUUID coremodel.UUID
	var modelUUIDStr string
	var activateModel func(ctx context.Context) error

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Verifies that watchers do not receive any changes when newly unactivated models are created.
	harness.AddTest(func(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)
		modelUUIDStr = modelUUID.String()
	}, func(w watchertest.WatcherC[[]string]) {
		// Get the change.
		w.AssertNoChange()
	})

	// Verifies that watchers receive changes when models are activated.
	harness.AddTest(func(c *tc.C) {
		err := activateModel(ctx)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				modelUUIDStr,
			),
		)
	})

	// Verifies that watchers do not receive changes when entities other than
	// models are updated.
	harness.AddTest(func(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Verifies that watchers do not receive changes when models are deleted.
	harness.AddTest(func(c *tc.C) {
		// Deletes model from table. This should not trigger a change event.
		err := modelService.DeleteModel(ctx, modelUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchModel(c *tc.C) {
	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, loggertesting.WrapCheckLog(c))

	st := state.NewState(func() (database.TxnRunner, error) { return watchableDBFactory() })

	modelService := service.NewWatchableService(st, nil, loggertesting.WrapCheckLog(c), watcherFactory)

	// Create a new unactivated model named test-model.
	modelName := "test-model"
	modelUUID, activateModel, err := modelService.CreateModel(context.Background(), domainmodel.GlobalModelCreationArgs{
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
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := modelService.WatchModel(context.Background(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Verifies that watchers do not receive any changes when newly unactivated
	// models are created.
	harness.AddTest(func(c *tc.C) {
		activateModel(context.Background())
	}, func(w watchertest.WatcherC[struct{}]) {
		// Get the change.
		w.AssertChange()
	})

	// Verifies that watchers do not receive changes when models are deleted.
	harness.AddTest(func(c *tc.C) {
		// Deletes model from table. This should not trigger a change event.
		err := modelService.DeleteModel(context.Background(), modelUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchModelCloudCredential(c *tc.C) {
	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, loggertesting.WrapCheckLog(c))
	st := state.NewState(func() (database.TxnRunner, error) { return watchableDBFactory() })

	credSt := credentialstate.NewState(s.TxnRunnerFactory())
	anotherKey := corecredential.Key{
		Cloud: "my-cloud",
		Owner: s.userName,
		Name:  "another",
	}
	credInfo2 := credential.CloudCredentialInfo{
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo2": "foo val",
		},
	}
	err := credSt.UpsertCloudCredential(context.Background(), anotherKey, credInfo2)
	c.Assert(err, tc.ErrorIsNil)

	originalKey := corecredential.Key{
		Cloud: "my-cloud",
		Owner: s.userName,
		Name:  "my-cloud-credential",
	}
	modelUUID := modeltesting.GenModelUUID(c)
	err = st.Create(
		context.Background(),
		modelUUID,
		coremodel.IAAS,
		domainmodel.GlobalModelCreationArgs{
			Cloud:         "my-cloud",
			Credential:    originalKey,
			Name:          coremodel.ControllerModelName,
			Owner:         s.userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	err = st.Activate(context.Background(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	modelService := service.NewWatchableService(st, nil, loggertesting.WrapCheckLog(c), watcherFactory)
	watcher, err := modelService.WatchModelCloudCredential(context.Background(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Test that updating the credential content triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		credInfo := credential.CloudCredentialInfo{
			AuthType: string(cloud.AccessKeyAuthType),
			Attributes: map[string]string{
				"foo": "foo val",
				"bar": "bar val",
			},
		}
		err := credSt.UpsertCloudCredential(context.Background(), originalKey, credInfo)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	// Test that updating the model credential reference triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		err := st.UpdateCredential(context.Background(), modelUUID, anotherKey)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	// Test that updating the old original credential after a reference update
	// does not trigger the watcher.
	harness.AddTest(func(c *tc.C) {
		credInfo := credential.CloudCredentialInfo{
			AuthType: string(cloud.AccessKeyAuthType),
			Attributes: map[string]string{
				"foo": "foo val2",
				"bar": "bar val2",
			},
		}
		err := credSt.UpsertCloudCredential(context.Background(), originalKey, credInfo)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})
	// Test that updating the new credential triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		credInfo := credential.CloudCredentialInfo{
			AuthType: string(cloud.AccessKeyAuthType),
			Attributes: map[string]string{
				"foo": "foo val3",
				"bar": "bar val3",
			},
		}
		err := credSt.UpsertCloudCredential(context.Background(), anotherKey, credInfo)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	// Test that updating the cloud endpoint triggers the watcher.
	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	harness.AddTest(func(c *tc.C) {
		cld := cloud.Cloud{
			Name:      "my-cloud",
			Type:      "ec2",
			AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
			Endpoint:  "endpoint",
		}
		err := cloudSt.UpdateCloud(context.Background(), cld)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	// Test that updating the cloud CA cert triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		cld := cloud.Cloud{
			Name:           "my-cloud",
			Type:           "ec2",
			AuthTypes:      cloud.AuthTypes{cloud.AccessKeyAuthType},
			Endpoint:       "endpoint",
			CACertificates: []string{testing.CACert},
		}
		err := cloudSt.UpdateCloud(context.Background(), cld)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	// Test that updating the model life does not trigger the watcher.
	harness.AddTest(func(c *tc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 1 WHERE uuid = ?", modelUUID)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}
