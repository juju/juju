// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/domain/model"
	modelstate "github.com/juju/juju/domain/model/state/model"
	"github.com/juju/juju/domain/modelmigration/service"
	migrationstate "github.com/juju/juju/domain/modelmigration/state/model"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite

	modelUUID coremodel.UUID
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.modelUUID = coremodel.UUID(s.ModelUUID())

	// Seed the model row so that model_migrating's FK to model is satisfied.
	modelSt := modelstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := modelSt.Create(c.Context(), model.ModelDetailArgs{
		UUID:               s.modelUUID,
		AgentStream:        domainagentbinary.AgentStreamReleased,
		AgentVersion:       jujuversion.Current,
		LatestAgentVersion: jujuversion.Current,
		ControllerUUID:     uuid.MustNewUUID(),
		Name:               "test-model",
		Qualifier:          "prod",
		Type:               coremodel.IAAS,
		Cloud:              "aws",
		CloudType:          "ec2",
		CloudRegion:        "us-east-1",
		CredentialOwner:    usertesting.GenNewName(c, "myowner"),
		CredentialName:     "mycredential",
	})
	c.Assert(err, tc.ErrorIsNil)
}

// TestWatchForMigration asserts that inserting or deleting a row in the
// model_migrating table for this model fires the watcher. The service-level
// predicate filtering (scope to this model's UUID) is covered in the service
// unit tests.
func (s *watcherSuite) TestWatchForMigration(c *tc.C) {
	svc := s.setupService(c)

	s.AssertChangeStreamIdle(c, "before watcher start")
	w, err := svc.WatchForMigration(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))

	// Initially there is nothing to report.
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Inserting a model_migrating row for this model fires the watcher.
	migratingUUID := uuid.MustNewUUID().String()
	harness.AddTest(c, func(c *tc.C) {
		err := s.ModelTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				"INSERT INTO model_migrating (uuid, model_uuid) VALUES (?, ?)",
				migratingUUID, s.modelUUID.String())
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Deleting the same row fires the watcher.
	harness.AddTest(c, func(c *tc.C) {
		err := s.ModelTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				"DELETE FROM model_migrating WHERE uuid = ?",
				migratingUUID)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) setupService(c *tc.C) *service.Service {
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "modelmigration")

	modelState := migrationstate.New(modelDB, s.modelUUID)

	noopInstanceGetter := func(context.Context) (service.InstanceProvider, error) {
		return nil, nil
	}
	noopResourceGetter := func(context.Context) (service.ResourceProvider, error) {
		return nil, nil
	}

	return service.NewService(
		nil,
		modelState,
		s.modelUUID.String(),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		providertracker.ProviderGetter[service.InstanceProvider](noopInstanceGetter),
		providertracker.ProviderGetter[service.ResourceProvider](noopResourceGetter),
	)
}
