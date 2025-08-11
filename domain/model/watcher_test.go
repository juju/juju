// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
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
	statecontroller "github.com/juju/juju/domain/model/state/controller"
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

func TestWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &watcherSuite{})
}

func insertModelDependencies(c *tc.C, dbTxnRunnerFactory database.TxnRunnerFactory,
	dbTxnRunner database.TxnRunner, userUUID user.UUID, userName user.Name) {
	accessState := accessstate.NewState(dbTxnRunnerFactory, loggertesting.WrapCheckLog(c))

	// Add a user so we can set model owner.
	err := accessState.AddUser(
		c.Context(),
		userUUID,
		userName,
		userName.Name(),
		false,
		userUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	// Add a cloud so we can set the model cloud.
	cloudSt := cloudstate.NewState(dbTxnRunnerFactory)
	err = cloudSt.CreateCloud(c.Context(), userName, uuid.MustNewUUID().String(),
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
		c.Context(), corecredential.Key{
			Cloud: "my-cloud",
			Owner: userName,
			Name:  "my-cloud-credential",
		},
		cred,
	)
	c.Assert(err, tc.ErrorIsNil)

	err = bootstrap.CreateDefaultBackends(coremodel.IAAS)(c.Context(), dbTxnRunner, dbTxnRunner)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.userUUID = usertesting.GenUserUUID(c)
	s.userName = usertesting.GenNewName(c, "test-user")
	insertModelDependencies(c, s.TxnRunnerFactory(), s.TxnRunner(), s.userUUID, s.userName)
}

func (s *watcherSuite) TestWatchControllerDBModels(c *tc.C) {
	ctx := c.Context()

	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, loggertesting.WrapCheckLog(c))
	st := statecontroller.NewState(func(ctx context.Context) (database.TxnRunner, error) { return watchableDBFactory(ctx) })

	modelService := service.NewWatchableService(
		st,
		watcherFactory,
		newStatusHistoryGetter(c),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	// Create a controller service watcher.
	watcher, err := modelService.WatchActivatedModels(ctx)
	c.Assert(err, tc.ErrorIsNil)

	modelName := "test-model"
	var modelUUID coremodel.UUID
	var modelUUIDStr string
	var activateModel func(ctx context.Context) error

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Verifies that watchers do not receive any changes when newly unactivated models are created.
	harness.AddTest(c, func(c *tc.C) {
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
			Qualifier:     "prod",
			AdminUsers:    []user.UUID{s.userUUID},
			SecretBackend: juju.BackendName,
		})
		c.Assert(err, tc.ErrorIsNil)
		modelUUIDStr = modelUUID.String()
	}, func(w watchertest.WatcherC[[]string]) {
		// Get the change.
		w.AssertNoChange()
	})

	// Verifies that watchers receive changes when models are activated.
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
		cloudState := cloudstate.NewState(func(ctx context.Context) (database.TxnRunner, error) { return watchableDBFactory(ctx) })
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

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchModel(c *tc.C) {
	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, loggertesting.WrapCheckLog(c))

	st := statecontroller.NewState(func(ctx context.Context) (database.TxnRunner, error) { return watchableDBFactory(ctx) })

	modelService := service.NewWatchableService(
		st,
		watcherFactory,
		newStatusHistoryGetter(c),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	// Create a new unactivated model named test-model.
	modelName := "test-model"
	modelUUID, activateModel, err := modelService.CreateModel(c.Context(), domainmodel.GlobalModelCreationArgs{
		Cloud:       "my-cloud",
		CloudRegion: "my-region",
		Credential: corecredential.Key{
			Cloud: "my-cloud",
			Owner: s.userName,
			Name:  "my-cloud-credential",
		},
		Name:          modelName,
		Qualifier:     "prod",
		AdminUsers:    []user.UUID{s.userUUID},
		SecretBackend: juju.BackendName,
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := modelService.WatchModel(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Verifies that watchers do not receive any changes when newly unactivated
	// models are created.
	harness.AddTest(c, func(c *tc.C) {
		err := activateModel(c.Context())
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		// Get the change.
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchModelCloudCredential(c *tc.C) {
	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, loggertesting.WrapCheckLog(c))
	st := statecontroller.NewState(func(ctx context.Context) (database.TxnRunner, error) { return watchableDBFactory(ctx) })

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
	err := credSt.UpsertCloudCredential(c.Context(), anotherKey, credInfo2)
	c.Assert(err, tc.ErrorIsNil)

	originalKey := corecredential.Key{
		Cloud: "my-cloud",
		Owner: s.userName,
		Name:  "my-cloud-credential",
	}
	modelUUID := modeltesting.GenModelUUID(c)
	err = st.Create(
		c.Context(),
		modelUUID,
		coremodel.IAAS,
		domainmodel.GlobalModelCreationArgs{
			Cloud:         "my-cloud",
			Credential:    originalKey,
			Name:          coremodel.ControllerModelName,
			Qualifier:     "prod",
			AdminUsers:    []user.UUID{s.userUUID},
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	err = st.Activate(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	modelService := service.NewWatchableService(
		st,
		watcherFactory,
		newStatusHistoryGetter(c),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	watcher, err := modelService.WatchModelCloudCredential(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Test that updating the credential content triggers the watcher.
	harness.AddTest(c, func(c *tc.C) {
		credInfo := credential.CloudCredentialInfo{
			AuthType: string(cloud.AccessKeyAuthType),
			Attributes: map[string]string{
				"foo": "foo val",
				"bar": "bar val",
			},
		}
		err := credSt.UpsertCloudCredential(c.Context(), originalKey, credInfo)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	// Test that updating the model credential reference triggers the watcher.
	harness.AddTest(c, func(c *tc.C) {
		err := st.UpdateCredential(c.Context(), modelUUID, anotherKey)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	// Test that updating the old original credential after a reference update
	// does not trigger the watcher.
	harness.AddTest(c, func(c *tc.C) {
		credInfo := credential.CloudCredentialInfo{
			AuthType: string(cloud.AccessKeyAuthType),
			Attributes: map[string]string{
				"foo": "foo val2",
				"bar": "bar val2",
			},
		}
		err := credSt.UpsertCloudCredential(c.Context(), originalKey, credInfo)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})
	// Test that updating the new credential triggers the watcher.
	harness.AddTest(c, func(c *tc.C) {
		credInfo := credential.CloudCredentialInfo{
			AuthType: string(cloud.AccessKeyAuthType),
			Attributes: map[string]string{
				"foo": "foo val3",
				"bar": "bar val3",
			},
		}
		err := credSt.UpsertCloudCredential(c.Context(), anotherKey, credInfo)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	// Test that updating the cloud endpoint triggers the watcher.
	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	harness.AddTest(c, func(c *tc.C) {
		cld := cloud.Cloud{
			Name:      "my-cloud",
			Type:      "ec2",
			AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
			Endpoint:  "endpoint",
		}
		err := cloudSt.UpdateCloud(c.Context(), cld)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	// Test that updating the cloud CA cert triggers the watcher.
	harness.AddTest(c, func(c *tc.C) {
		cld := cloud.Cloud{
			Name:           "my-cloud",
			Type:           "ec2",
			AuthTypes:      cloud.AuthTypes{cloud.AccessKeyAuthType},
			Endpoint:       "endpoint",
			CACertificates: []string{testing.CACert},
		}
		err := cloudSt.UpdateCloud(c.Context(), cld)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	// Test that updating the model life does not trigger the watcher.
	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, "UPDATE model SET life_id = 1 WHERE uuid = ?", modelUUID)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

type statusHistoryGetter struct {
	loggerContextGetter logger.LoggerContextGetter
	clock               clock.Clock
}

func newStatusHistoryGetter(c *tc.C) service.StatusHistoryGetter {
	return statusHistoryGetter{
		loggerContextGetter: loggertesting.WrapCheckLogForContextGetter(c),
		clock:               clock.WallClock,
	}
}

// GetLoggerContext returns a logger context for the given model UUID.
func (l statusHistoryGetter) GetStatusHistoryForModel(ctx context.Context, modelUUID coremodel.UUID) (service.StatusHistory, error) {
	loggerContext, err := l.loggerContextGetter.GetLoggerContext(ctx, modelUUID)
	if err != nil {
		return nil, err
	}

	logger := loggerContext.GetLogger("juju.services")
	return domain.NewStatusHistory(logger, l.clock), nil
}
