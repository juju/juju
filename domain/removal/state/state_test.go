// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/life"
	objectstorestate "github.com/juju/juju/domain/objectstore/state"
	"github.com/juju/juju/domain/removal"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/environs"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	internalcharm "github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ModelSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) TestGetAllJobsNoRows(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	jobs, err := st.GetAllJobs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(jobs, tc.HasLen, 0)
}

func (s *stateSuite) TestGetAllJobsWithData(c *tc.C) {
	ins := `
INSERT INTO removal (uuid, removal_type_id, entity_uuid, force, scheduled_for, arg) 
VALUES (?, ?, ?, ?, ?, ?)`

	jID1, _ := removal.NewUUID()
	jID2, _ := removal.NewUUID()
	now := time.Now().UTC()

	_, err := s.DB().Exec(ins, jID1, 0, "rel-1", 0, now, nil)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec(ins, jID2, 0, "rel-2", 1, now, `{"special-key":"special-value"}`)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	jobs, err := st.GetAllJobs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobs, tc.HasLen, 2)

	c.Check(jobs[0], tc.DeepEquals, removal.Job{
		UUID:         jID1,
		RemovalType:  removal.RelationJob,
		EntityUUID:   "rel-1",
		Force:        false,
		ScheduledFor: now,
	})

	c.Check(jobs[1], tc.DeepEquals, removal.Job{
		UUID:         jID2,
		RemovalType:  removal.RelationJob,
		EntityUUID:   "rel-2",
		Force:        true,
		ScheduledFor: now,
		Arg: map[string]any{
			"special-key": "special-value",
		},
	})
}

func (s *stateSuite) TestDeleteJob(c *tc.C) {
	ins := `
INSERT INTO removal (uuid, removal_type_id, entity_uuid, force, scheduled_for, arg) 
VALUES (?, ?, ?, ?, ?, ?)`

	jID1, _ := removal.NewUUID()
	now := time.Now().UTC()
	_, err := s.DB().Exec(ins, jID1, 0, "rel-1", 0, now, nil)
	c.Assert(err, tc.ErrorIsNil)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.DeleteJob(c.Context(), jID1.String())
	c.Assert(err, tc.ErrorIsNil)

	row := s.DB().QueryRow("SELECT count(*) FROM removal where uuid = ?", jID1)
	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	// Idempotent.
	err = st.DeleteJob(c.Context(), jID1.String())
	c.Assert(err, tc.ErrorIsNil)
}

type baseSuite struct {
	changestreamtesting.ModelSuite
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "prod", "test-model", "ec2")
		`, modelUUID.String(), internaltesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) setupService(c *tc.C, factory domain.WatchableDBFactory) *applicationservice.WatchableService {
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
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return provider.CommonStorageProviders()
		}),
		"",
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		nil,
		providerGetter,
		caasProviderGetter,
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *baseSuite) createIAASApplication(c *tc.C, svc *applicationservice.WatchableService, name string, units ...applicationservice.AddIAASUnitArg) coreapplication.ID {
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

	s.setCharmObjectStoreMetadata(c, appID)

	return appID
}

func (s *baseSuite) createIAASSubordinateApplication(c *tc.C, svc *applicationservice.WatchableService, name string, units ...applicationservice.AddIAASUnitArg) coreapplication.ID {
	ch := &stubCharm{name: "test-charm", subordinate: true}
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

	s.setCharmObjectStoreMetadata(c, appID)

	return appID
}

func (s *baseSuite) createCAASApplication(c *tc.C, svc *applicationservice.WatchableService, name string, units ...applicationservice.AddUnitArg) coreapplication.ID {
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
		ResolvedResources: applicationservice.ResolvedResources{{
			Name:     "buzz",
			Revision: ptr(42),
			Origin:   charmresource.OriginStore,
		}},
	}, units...)
	c.Assert(err, tc.ErrorIsNil)

	if len(units) == 0 {
		return appID
	}

	// Register a unit for the CAAS application if units were provided.
	_, _, err = svc.RegisterCAASUnit(c.Context(), application.RegisterCAASUnitParams{
		ApplicationName: name,
		ProviderID:      name + "-0",
	})
	c.Assert(err, tc.ErrorIsNil)

	s.setCharmObjectStoreMetadata(c, appID)

	return appID
}

func (s *baseSuite) setCharmObjectStoreMetadata(c *tc.C, appID coreapplication.ID) {
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

func (s *baseSuite) getAllUnitUUIDs(c *tc.C, appID coreapplication.ID) []unit.UUID {
	var unitUUIDs []unit.UUID
	err := s.ModelTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT uuid FROM unit WHERE application_uuid = ? ORDER BY uuid`, appID)
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

func (s *baseSuite) getAllMachineUUIDs(c *tc.C) []machine.UUID {
	var machineUUIDs []machine.UUID
	err := s.ModelTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT uuid FROM machine ORDER BY uuid`)
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
	return machineUUIDs
}

func (s *baseSuite) getAllUnitAndMachineUUIDs(c *tc.C) ([]unit.UUID, []machine.UUID) {
	result := make(map[unit.UUID]machine.UUID)
	err := s.ModelTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT u.uuid, m.uuid 
FROM unit AS u
JOIN net_node AS nn ON nn.uuid = u.net_node_uuid
JOIN machine AS m ON m.net_node_uuid = nn.uuid
`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				unitUUID    unit.UUID
				machineUUID machine.UUID
			)
			if err := rows.Scan(&unitUUID, &machineUUID); err != nil {
				return err
			}
			result[unitUUID] = machineUUID
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)

	var allUnitUUIDs []unit.UUID
	var allMachineUUIDs []machine.UUID
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

func (s *baseSuite) getUnitMachineUUID(c *tc.C, unitUUID unit.UUID) machine.UUID {
	var machineUUIDs []machine.UUID
	err := s.ModelTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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

func (s *baseSuite) checkNoCharmsExist(c *tc.C) {
	// Ensure that there are no charms in the database.
	row := s.DB().QueryRow("SELECT COUNT(*) FROM charm")
	var count int
	err := row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *baseSuite) checkCharmsCount(c *tc.C, expectedCount int) {
	// Ensure that there are no charms in the database.
	row := s.DB().QueryRow("SELECT COUNT(*) FROM charm")
	var count int
	err := row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, expectedCount)
}

func (s *baseSuite) advanceUnitLife(c *tc.C, unitUUID unit.UUID, newLife life.Life) {
	_, err := s.DB().Exec("UPDATE unit SET life_id = ? WHERE uuid = ?", newLife, unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
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
