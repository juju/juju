// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/life"
	domaintesting "github.com/juju/juju/domain/testing"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type unitSuite struct {
	changestreamtesting.ModelSuite
}

func TestUnitSuite(t *testing.T) {
	tc.Run(t, &unitSuite{})
}

func (s *unitSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), internaltesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestUnitExists(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.UnitExists(context.Background(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)

	exists, err = st.UnitExists(context.Background(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *unitSuite) TestEnsureUnitNotAliveNormalSuccessLastUnit(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	unitMachineUUID := s.getUnitMachineUUID(c, unitUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	machineUUID, err := st.EnsureUnitNotAlive(context.Background(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(machineUUID, tc.Equals, unitMachineUUID.String())

	// Unit had life "alive" and should now be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM unit where uuid = ?", unitUUID.String())
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)

	// The last machine had life "alive" and should now be "dying".
	row = s.DB().QueryRow("SELECT life_id FROM machine where uuid = ?", machineUUID)
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *unitSuite) TestEnsureUnitNotAliveNormalSuccessLastUnitMachineAlreadyDying(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddUnitArg{},
	)

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	unitMachineUUID := s.getUnitMachineUUID(c, unitUUID)
	// Set the machine to "dying" manually.
	_, err := s.DB().Exec("UPDATE machine SET life_id = 1 WHERE uuid = ?", unitMachineUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	machineUUID, err := st.EnsureUnitNotAlive(context.Background(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(machineUUID, tc.Equals, unitMachineUUID.String())

	// Unit had life "alive" and should now be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM unit where uuid = ?", unitUUID.String())
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)

	// The last machine had life "alive" and should now be "dying".
	row = s.DB().QueryRow("SELECT life_id FROM machine where uuid = ?", machineUUID)
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *unitSuite) TestEnsureUnitNotAliveNormalSuccess(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddUnitArg{},
		applicationservice.AddUnitArg{
			// Place this unit on the same machine as the first one.
			Placement: instance.MustParsePlacement("0"),
		},
	)

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 2)
	unitUUID := unitUUIDs[0]

	unitMachineUUID := s.getUnitMachineUUID(c, unitUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	machineUUID, err := st.EnsureUnitNotAlive(context.Background(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// This isn't the last unit on the machine, so we don't expect a machine
	// UUID.
	c.Assert(machineUUID, tc.Equals, "")

	// Unit had life "alive" and should now be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM unit where uuid = ?", unitUUID.String())
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)

	// Don't set the machine life to "dying" if there are other units on it.
	row = s.DB().QueryRow("SELECT life_id FROM machine where uuid = ?", unitMachineUUID)
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 0)
}

func (s *unitSuite) TestEnsureUnitNotAliveDyingSuccess(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.EnsureUnitNotAlive(context.Background(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Unit was already "dying" and should be unchanged.
	row := s.DB().QueryRow("SELECT life_id FROM unit where uuid = ?", unitUUID.String())
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *unitSuite) TestEnsureUnitNotAliveNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// We don't care if it's already gone.
	_, err := st.EnsureUnitNotAlive(context.Background(), "some-unit-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestUnitRemovalNormalSuccess(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.UnitScheduleRemoval(
		context.Background(), "removal-uuid", unitUUID.String(), false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	row := s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal where uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalTypeID, tc.Equals, 1)
	c.Check(rUUID, tc.Equals, unitUUID.String())
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *unitSuite) TestUnitRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.UnitScheduleRemoval(
		context.Background(), "removal-uuid", "some-unit-uuid", true, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	// It doesn't matter that the unit does not exist.
	// We rely on the worker to handle that fact.
	row := s.DB().QueryRow(`
SELECT t.name, r.entity_uuid, r.force, r.scheduled_for 
FROM   removal r JOIN removal_type t ON r.removal_type_id = t.id
where  r.uuid = ?`, "removal-uuid",
	)

	var (
		removalType  string
		rUUID        string
		force        bool
		scheduledFor time.Time
	)
	err = row.Scan(&removalType, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalType, tc.Equals, "unit")
	c.Check(rUUID, tc.Equals, "some-unit-uuid")
	c.Check(force, tc.Equals, true)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *unitSuite) TestGetUnitLifeSuccess(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	// Set the unit to "dying" manually.
	_, err := s.DB().Exec("UPDATE unit SET life_id = 1 WHERE uuid = ?", unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetUnitLife(context.Background(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Dying)
}

func (s *unitSuite) TestGetUnitLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetUnitLife(context.Background(), "some-unit-uuid")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitSuite) TestDeleteIAASUnit(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteUnit(context.Background(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The unit should be gone.
	exists, err := st.UnitExists(context.Background(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *unitSuite) TestDeleteCAASUnit(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createCAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.expectK8sPodCount(c, unitUUID, 1)

	err := st.DeleteUnit(context.Background(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The unit should be gone.
	exists, err := st.UnitExists(context.Background(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	s.expectK8sPodCount(c, unitUUID, 0)
}

func (s *unitSuite) expectK8sPodCount(c *tc.C, unitUUID unit.UUID, expected int) {
	var count int
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM k8s_pod WHERE unit_uuid = ?`, unitUUID.String())
		if err := row.Scan(&count); err != nil {
			return err
		}
		return row.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, expected)
}

func (s *unitSuite) setupService(c *tc.C, factory domain.WatchableDBFactory) *applicationservice.WatchableService {
	modelDB := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	notSupportedProviderGetter := func(ctx context.Context) (applicationservice.Provider, error) {
		return nil, coreerrors.NotSupported
	}
	notSupportedFeatureProviderGetter := func(ctx context.Context) (applicationservice.SupportedFeatureProvider, error) {
		return nil, coreerrors.NotSupported
	}
	notSupportedCAASApplicationproviderGetter := func(ctx context.Context) (applicationservice.CAASApplicationProvider, error) {
		return caasApplicationProvider{}, nil
	}

	return applicationservice.NewWatchableService(
		applicationstate.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domaintesting.NoopLeaderEnsurer(),
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return provider.CommonStorageProviders()
		}),
		"",
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		nil, notSupportedProviderGetter,
		notSupportedFeatureProviderGetter, notSupportedCAASApplicationproviderGetter, nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *unitSuite) createIAASApplication(c *tc.C, svc *applicationservice.WatchableService, name string, units ...applicationservice.AddUnitArg) coreapplication.ID {
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
	}, units...)
	c.Assert(err, tc.ErrorIsNil)

	return appID
}

func (s *unitSuite) createCAASApplication(c *tc.C, svc *applicationservice.WatchableService, name string, units ...applicationservice.AddUnitArg) coreapplication.ID {
	ch := &stubCharm{name: "test-charm"}
	appID, err := svc.CreateCAASApplication(c.Context(), name, ch, corecharm.Origin{
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
	}, units...)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = svc.RegisterCAASUnit(c.Context(), application.RegisterCAASUnitParams{
		ApplicationName: name,
		ProviderID:      name + "-0",
	})
	c.Assert(err, tc.ErrorIsNil)

	return appID
}

func (s *unitSuite) getAllUnitUUIDs(c *tc.C, appID coreapplication.ID) []unit.UUID {
	var unitUUIDs []unit.UUID
	err := s.ModelTxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT uuid FROM unit WHERE application_uuid = ?`, appID)
		if err != nil {
			return err
		}

		defer rows.Close()
		for rows.Next() {
			var unitUUID unit.UUID
			if err := rows.Scan(&unitUUID); err != nil {
				return err
			}
			unitUUIDs = append(unitUUIDs, unitUUID)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	return unitUUIDs
}

func (s *unitSuite) getUnitMachineUUID(c *tc.C, unitUUID unit.UUID) machine.UUID {
	var machineUUIDs []machine.UUID
	err := s.ModelTxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT m.uuid
FROM   machine AS m
JOIN   net_node AS nn ON nn.uuid = m.net_node_uuid
JOIN   unit AS u ON u.net_node_uuid = nn.uuid
WHERE u.uuid = ?
`, unitUUID)
		if err != nil {
			return err
		}

		defer rows.Close()
		for rows.Next() {
			var machineUUID machine.UUID
			if err := rows.Scan(&machineUUID); err != nil {
				return err
			}
			machineUUIDs = append(machineUUIDs, machineUUID)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(machineUUIDs), tc.Equals, 1)
	return machineUUIDs[0]
}

type stubCharm struct {
	name string
}

func (s *stubCharm) Meta() *internalcharm.Meta {
	name := s.name
	if name == "" {
		name = "test"
	}
	return &internalcharm.Meta{
		Name: name,
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

type caasApplicationProvider struct{}

func (caasApplicationProvider) Application(string, caas.DeploymentType) caas.Application {
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
