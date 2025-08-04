// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstate "github.com/juju/juju/domain/application/state"
	objectstorestate "github.com/juju/juju/domain/objectstore/state"
	"github.com/juju/juju/domain/removal/service"
	statecontroller "github.com/juju/juju/domain/removal/state/controller"
	statemodel "github.com/juju/juju/domain/removal/state/model"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/environs"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	internalcharm "github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "test", "iaas", "prod", "test-model", "ec2")
		`, modelUUID.String(), internaltesting.ControllerTag.Id())
		if err != nil {
			return errors.Errorf("creating model: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO model_life (model_uuid, life_id)
VALUES (?, 0);
		`, modelUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchRemovals(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "some-model-uuid")

	log := loggertesting.WrapCheckLog(c)

	svc := service.NewWatchableService(
		statecontroller.NewState(func() (database.TxnRunner, error) { return s.NoopTxnRunner(), nil }, log),
		statemodel.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, log),
		domain.NewWatcherFactory(factory, log),
		nil,
		nil,
		model.UUID(s.ModelUUID()),
		clock.WallClock,
		log,
	)

	w, err := svc.WatchRemovals()
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))

	// Insert 2 new jobs and check that the watcher emits their UUIDs.
	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			q := `INSERT INTO removal (uuid, removal_type_id, entity_uuid) VALUES (?, ?, ?)`

			if _, err := tx.ExecContext(ctx, q, "job-uuid-1", 1, "rel-uuid-1"); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, q, "job-uuid-2", 1, "rel-uuid-2")
			return err
		})
		c.Assert(err, tc.ErrorIsNil)

	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("job-uuid-1", "job-uuid-2"))
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchEntityRemovals(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "some-model-uuid")

	log := loggertesting.WrapCheckLog(c)

	modelState := statemodel.NewState(func() (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, log)
	svc := service.NewWatchableService(
		statecontroller.NewState(func() (database.TxnRunner, error) { return s.NoopTxnRunner(), nil }, log),
		modelState,
		domain.NewWatcherFactory(factory, log),
		nil,
		nil,
		model.UUID(s.ModelUUID()),
		clock.WallClock,
		log,
	)

	w, err := svc.WatchEntityRemovals()
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))

	harness.AddTest(c, func(c *tc.C) {
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	var appUUID, unitUUID, machineUUID string

	harness.AddTest(c, func(c *tc.C) {
		factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
		svc := s.setupApplicationService(c, factory)
		appUUID = s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

		unitUUIDs, machineUUIDs := s.getAppUnitAndMachineUUIDs(c, appUUID)
		unitUUID = unitUUIDs[0]
		machineUUID = machineUUIDs[0]
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		// Removing a unit also removes the machine, as it's the last unit
		// on the machine.
		_, err := svc.RemoveUnit(c.Context(), unit.UUID(unitUUID), false, 0)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(
			"unit:"+unitUUID,
			"machine:"+machineUUID,
		))
	})

	harness.AddTest(c, func(c *tc.C) {
		err := svc.MarkUnitAsDead(c.Context(), unit.UUID(unitUUID))
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(
			"unit:" + unitUUID,
		))
	})

	harness.AddTest(c, func(c *tc.C) {
		err := modelState.DeleteUnit(c.Context(), unitUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) setupApplicationService(c *tc.C, factory domain.WatchableDBFactory) *applicationservice.WatchableService {
	modelDB := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	providerGetter := func(ctx context.Context) (applicationservice.Provider, error) {
		return appProvider{}, nil
	}
	caasProviderGetter := func(ctx context.Context) (applicationservice.CAASProvider, error) {
		return appProvider{}, nil
	}

	return applicationservice.NewWatchableService(
		applicationstate.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domaintesting.NoopLeaderEnsurer(),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		nil,
		providerGetter,
		caasProviderGetter,
		nil,
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *watcherSuite) createIAASApplication(c *tc.C, svc *applicationservice.WatchableService, name string, units ...applicationservice.AddIAASUnitArg) string {
	ch := &stubCharm{name: "test-charm"}
	appID, err := svc.CreateIAASApplication(c.Context(), name, ch, corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, applicationservice.AddApplicationArgs{
		ReferenceName: name,
		DownloadInfo: &charm.DownloadInfo{
			Provenance:  charm.ProvenanceDownload,
			DownloadURL: "http://example.com",
		},
		ResolvedResources: applicationservice.ResolvedResources{{
			Name:     "buzz",
			Revision: ptr(42),
			Origin:   charmresource.OriginStore,
		}},
	}, units...)
	c.Assert(err, tc.ErrorIsNil)

	s.setCharmObjectStoreMetadata(c, appID.String())

	return appID.String()
}

func (s *watcherSuite) setCharmObjectStoreMetadata(c *tc.C, appID string) {
	modelDB := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	objectStoreUUID, err := objectstorestate.NewState(modelDB).PutMetadata(c.Context(), coreobjectstore.Metadata{
		SHA256: fmt.Sprintf("%v-sha256", appID),
		SHA384: fmt.Sprintf("%v-sha384", appID),
		Path:   fmt.Sprintf("/path/to/%v", appID),
		Size:   100,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE charm
SET object_store_uuid = ?
WHERE uuid IN (
	SELECT charm_uuid
	FROM application
	WHERE uuid = ?
)`, objectStoreUUID, appID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) getAppUnitAndMachineUUIDs(c *tc.C, appUUID string) (units []string, machines []string) {
	result := make(map[string]string)
	err := s.ModelTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT u.uuid, m.uuid
FROM unit AS u
JOIN net_node AS nn ON nn.uuid = u.net_node_uuid
JOIN machine AS m ON m.net_node_uuid = nn.uuid
WHERE u.application_uuid = ?
`, appUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				unitUUID    string
				machineUUID string
			)
			if err := rows.Scan(&unitUUID, &machineUUID); err != nil {
				return err
			}
			result[unitUUID] = machineUUID
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)

	var allUnitUUIDs []string
	var allMachineUUIDs []string
	for unitUUID, machineUUID := range result {
		allUnitUUIDs = append(allUnitUUIDs, unitUUID)

		// If the machine UUID is empty, it means that the unit is not
		// associated with any machine.
		if machineUUID == "" {
			continue
		}
		allMachineUUIDs = append(allMachineUUIDs, machineUUID)
	}

	return allUnitUUIDs, allMachineUUIDs
}

type stubCharm struct {
	name        string
	subordinate bool
}

func (s *stubCharm) Meta() *internalcharm.Meta {
	name := s.name
	if name == "" {
		name = "test"
	}
	return &internalcharm.Meta{
		Name:        name,
		Subordinate: s.subordinate,
		Resources: map[string]charmresource.Meta{
			"buzz": {
				Name:        "buzz",
				Type:        charmresource.TypeFile,
				Path:        "/path/to/buzz.tgz",
				Description: "buzz description",
			},
		},
	}
}

func (s *stubCharm) Manifest() *internalcharm.Manifest {
	return &internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name: "ubuntu",
			Channel: internalcharm.Channel{
				Risk: internalcharm.Stable,
			},
			Architectures: []string{"amd64"},
		}},
	}
}

func (s *stubCharm) Config() *internalcharm.Config {
	return &internalcharm.Config{
		Options: map[string]internalcharm.Option{
			"foo": {
				Type:    "string",
				Default: "bar",
			},
		},
	}
}

func (s *stubCharm) Actions() *internalcharm.Actions {
	return nil
}

func (s *stubCharm) Revision() int {
	return 0
}

func (s *stubCharm) Version() string {
	return ""
}

type appProvider struct {
	applicationservice.Provider
	applicationservice.CAASProvider
}

func (appProvider) PrecheckInstance(ctx context.Context, args environs.PrecheckInstanceParams) error {
	return nil
}

func (appProvider) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	return constraints.NewValidator(), nil
}

func (appProvider) Application(string, caas.DeploymentType) caas.Application {
	return &caasApplication{}
}

type caasApplication struct {
	caas.Application
}

func (caasApplication) Units() ([]caas.Unit, error) {
	return []caas.Unit{{
		Id: "some-app-0",
	}}, nil
}

func ptr[T any](v T) *T {
	return &v
}
