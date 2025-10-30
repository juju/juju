// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/relation"
	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	corestatus "github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstorageservice "github.com/juju/juju/domain/application/service/storage"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/crossmodelrelation"
	crossmodelrelationstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	"github.com/juju/juju/domain/life"
	machineservice "github.com/juju/juju/domain/machine/service"
	machinestate "github.com/juju/juju/domain/machine/state"
	objectstorestate "github.com/juju/juju/domain/objectstore/state"
	domainrelation "github.com/juju/juju/domain/relation"
	relationservice "github.com/juju/juju/domain/relation/service"
	relationstate "github.com/juju/juju/domain/relation/state"
	"github.com/juju/juju/domain/removal"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/environs"
	internalcharm "github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
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
	schematesting.ModelSuite

	nextOperationID func() string
	now             time.Time
}

// sequenceGenerator returns a function that generates unique string values in
// ascending order starting from "0".
func sequenceGenerator() func() string {
	id := 0
	return func() string {
		next := fmt.Sprint(id)
		id++
		return next
	}
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.nextOperationID = sequenceGenerator()
	s.now = time.Now().UTC()

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

func (s *baseSuite) setupMachineService(c *tc.C) *machineservice.ProviderService {
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	return machineservice.NewProviderService(
		machinestate.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		func(context.Context) (machineservice.Provider, error) { return machineservice.NewNoopProvider(), nil },
		nil,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *baseSuite) setupApplicationService(c *tc.C) *applicationservice.ProviderService {
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	providerGetter := func(ctx context.Context) (applicationservice.Provider, error) {
		return appProvider{}, nil
	}
	caasProviderGetter := func(ctx context.Context) (applicationservice.CAASProvider, error) {
		return appProvider{}, nil
	}
	storageProviderRegistryGetter := corestorage.ConstModelStorageRegistry(
		func() internalstorage.ProviderRegistry {
			return internalstorage.NotImplementedProviderRegistry{}
		},
	)
	state := applicationstate.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c))
	storageSvc := applicationstorageservice.NewService(
		state, applicationstorageservice.NewStoragePoolProvider(
			storageProviderRegistryGetter, state,
		),
	)

	return applicationservice.NewProviderService(
		applicationstate.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		storageSvc,
		domaintesting.NoopLeaderEnsurer(),
		nil,
		providerGetter,
		caasProviderGetter,
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *baseSuite) setupRelationService(c *tc.C) *relationservice.Service {
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	return relationservice.NewService(
		relationstate.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		loggertesting.WrapCheckLog(c),
	)
}

func (s *baseSuite) createIAASApplication(c *tc.C, svc *applicationservice.ProviderService, name string, units ...applicationservice.AddIAASUnitArg) coreapplication.UUID {
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

func (s *baseSuite) createIAASSubordinateApplication(c *tc.C, svc *applicationservice.ProviderService, name string, units ...applicationservice.AddIAASUnitArg) coreapplication.UUID {
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

func (s *baseSuite) createCAASApplication(c *tc.C, svc *applicationservice.ProviderService, name string, units ...applicationservice.AddUnitArg) coreapplication.UUID {
	ch := &stubCharm{name: "test-charm"}
	s.createSubnetForCAASModel(c)
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

func (s *baseSuite) createOffer(c *tc.C, offerName string) offer.UUID {
	cmrState := crossmodelrelationstate.NewState(
		s.TxnRunnerFactory(), coremodel.UUID(s.ModelUUID()), testclock.NewClock(s.now), loggertesting.WrapCheckLog(c),
	)
	s.createIAASApplication(c, s.setupApplicationService(c), offerName)
	offerUUID := tc.Must(c, offer.NewUUID)

	err := cmrState.CreateOffer(c.Context(), crossmodelrelation.CreateOfferArgs{
		UUID:            offerUUID,
		ApplicationName: offerName,
		Endpoints:       []string{"foo", "bar"},
		OfferName:       offerName,
	})
	c.Assert(err, tc.ErrorIsNil)

	return offerUUID
}

func (s *baseSuite) createOfferForApplication(c *tc.C, appName string, offerName string) offer.UUID {
	cmrState := crossmodelrelationstate.NewState(
		s.TxnRunnerFactory(), coremodel.UUID(s.ModelUUID()), testclock.NewClock(s.now), loggertesting.WrapCheckLog(c),
	)
	offerUUID := tc.Must(c, offer.NewUUID)

	err := cmrState.CreateOffer(c.Context(), crossmodelrelation.CreateOfferArgs{
		UUID:            offerUUID,
		ApplicationName: appName,
		Endpoints:       []string{"foo", "bar"},
		OfferName:       offerName,
	})
	c.Assert(err, tc.ErrorIsNil)

	return offerUUID
}

func (s *baseSuite) createRemoteApplicationOfferer(
	c *tc.C,
	name string,
) (coreapplication.UUID, coreremoteapplication.UUID) {
	cmrState := crossmodelrelationstate.NewState(
		s.TxnRunnerFactory(), coremodel.UUID(s.ModelUUID()), testclock.NewClock(s.now), loggertesting.WrapCheckLog(c),
	)

	ch := charm.Charm{
		Metadata: charm.Metadata{
			Name: name,
			Provides: map[string]charm.Relation{
				"foo": {
					Name:      "foo",
					Interface: "rel",
					Role:      charm.RoleProvider,
					Scope:     charm.ScopeGlobal,
				},
				"bar": {
					Name:      "bar",
					Interface: "rel",
					Role:      charm.RoleProvider,
					Scope:     charm.ScopeGlobal,
				},
			},
		},
		Manifest:      s.minimalManifest(),
		ReferenceName: name,
		Source:        charm.CMRSource,
		Revision:      42,
		Hash:          "hash",
		Architecture:  architecture.ARM64,
	}

	remoteAppUUID := tc.Must(c, coreremoteapplication.NewUUID)
	appUUID := tc.Must(c, coreapplication.NewUUID)
	err := cmrState.AddRemoteApplicationOfferer(c.Context(), name, crossmodelrelation.AddRemoteApplicationOffererArgs{
		RemoteApplicationUUID: remoteAppUUID.String(),
		ApplicationUUID:       appUUID.String(),
		CharmUUID:             tc.Must(c, uuid.NewUUID).String(),
		Charm:                 ch,
		OfferUUID:             tc.Must(c, uuid.NewUUID).String(),
		OfferURL:              tc.Must1(c, crossmodel.ParseOfferURL, fmt.Sprintf("controller:qualifier/model.%s", name)).String(),
		OffererModelUUID:      tc.Must(c, uuid.NewUUID).String(),
		EncodedMacaroon:       []byte("macaroon"),
	})
	c.Assert(err, tc.ErrorIsNil)

	return appUUID, remoteAppUUID
}

func (s *baseSuite) createRemoteApplicationConsumer(
	c *tc.C,
	name string,
	offerUUID offer.UUID,
) (coreapplication.UUID, coreremoteapplication.UUID) {
	cmrState := crossmodelrelationstate.NewState(
		s.TxnRunnerFactory(), coremodel.UUID(s.ModelUUID()), testclock.NewClock(s.now), loggertesting.WrapCheckLog(c),
	)

	ch := charm.Charm{
		Metadata: charm.Metadata{
			Name: name,
			Provides: map[string]charm.Relation{
				"foo": {
					Name:      "foo",
					Interface: "rel",
					Role:      charm.RoleProvider,
					Scope:     charm.ScopeGlobal,
				},
			},
		},
		Manifest:      s.minimalManifest(),
		ReferenceName: name,
		Source:        charm.CMRSource,
		Revision:      42,
		Hash:          "hash",
		Architecture:  architecture.ARM64,
	}

	remoteAppUUID := tc.Must(c, coreremoteapplication.NewUUID)
	appUUID := tc.Must(c, coreapplication.NewUUID)
	relationUUID := tc.Must(c, relation.NewUUID)
	err := cmrState.AddConsumedRelation(c.Context(), name, crossmodelrelation.AddRemoteApplicationConsumerArgs{
		SynthApplicationUUID:        remoteAppUUID.String(),
		ConsumerApplicationUUID:     appUUID.String(),
		ConsumerApplicationEndpoint: "foo",
		CharmUUID:                   tc.Must(c, uuid.NewUUID).String(),
		Charm:                       ch,
		OfferUUID:                   offerUUID.String(),
		OfferEndpointName:           "bar",
		RelationUUID:                relationUUID.String(),
	})
	c.Assert(err, tc.ErrorIsNil)

	return appUUID, remoteAppUUID
}

func (s *baseSuite) minimalManifest() charm.Manifest {
	return charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.RiskStable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}
}

// createRelation creates a relation between two applications.
func (s *baseSuite) createRelation(c *tc.C) relation.UUID {
	appSvc := s.setupApplicationService(c)
	s.createIAASApplication(c, appSvc, "app1")
	s.createIAASApplication(c, appSvc, "app2")

	relSvc := s.setupRelationService(c)
	ep1, ep2, err := relSvc.AddRelation(c.Context(), "app1:foo", "app2:bar")
	c.Assert(err, tc.ErrorIsNil)
	relUUID, err := relSvc.GetRelationUUIDForRemoval(c.Context(), domainrelation.GetRelationUUIDForRemovalArgs{
		Endpoints: []string{ep1.String(), ep2.String()},
	})
	c.Assert(err, tc.ErrorIsNil)

	return relUUID
}

// createRemoteRelation creates a remote relation. This is done by creating
// a synthetic app & a regular app, and then establishing a relation between
// them. We also add some units to the each app for good measure. Returns the
// relation UUID and the synthetic app UUID.
func (s *baseSuite) createRemoteRelation(c *tc.C) (relation.UUID, coreapplication.UUID) {
	synthAppUUID, _ := s.createRemoteApplicationOfferer(c, "foo")
	s.createIAASApplication(c, s.setupApplicationService(c), "bar",
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{},
	)

	relSvc := s.setupRelationService(c)
	ep1, ep2, err := relSvc.AddRelation(c.Context(), "foo:foo", "bar:bar")
	c.Assert(err, tc.ErrorIsNil)
	relUUID, err := relSvc.GetRelationUUIDForRemoval(c.Context(), domainrelation.GetRelationUUIDForRemovalArgs{
		Endpoints: []string{ep1.String(), ep2.String()},
	})
	c.Assert(err, tc.ErrorIsNil)

	cmrState := crossmodelrelationstate.NewState(
		s.TxnRunnerFactory(), coremodel.UUID(s.ModelUUID()), testclock.NewClock(s.now), loggertesting.WrapCheckLog(c),
	)
	// Call twice to ensure some units share a net node, but not all.
	cmrState.EnsureUnitsExist(c.Context(), synthAppUUID.String(), []string{"foo/0", "foo/1"})
	cmrState.EnsureUnitsExist(c.Context(), synthAppUUID.String(), []string{"foo/2"})

	return relUUID, synthAppUUID
}

func (s *baseSuite) createRemoteRelationBetween(c *tc.C, synthAppName, appName string) relation.UUID {
	relSvc := s.setupRelationService(c)

	ep1Name := fmt.Sprintf("%s:foo", synthAppName)
	ep2Name := fmt.Sprintf("%s:bar", appName)
	ep1, ep2, err := relSvc.AddRelation(c.Context(), ep1Name, ep2Name, "0.0.0.0/0")
	c.Assert(err, tc.ErrorIsNil)
	relUUID, err := relSvc.GetRelationUUIDForRemoval(c.Context(), domainrelation.GetRelationUUIDForRemovalArgs{
		Endpoints: []string{ep1.String(), ep2.String()},
	})
	c.Assert(err, tc.ErrorIsNil)

	cmrState := crossmodelrelationstate.NewState(
		s.TxnRunnerFactory(), coremodel.UUID(s.ModelUUID()), testclock.NewClock(s.now), loggertesting.WrapCheckLog(c),
	)

	unit0name := fmt.Sprintf("%s/0", synthAppName)
	unit1name := fmt.Sprintf("%s/1", synthAppName)
	unit2name := fmt.Sprintf("%s/2", synthAppName)

	var synthAppUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT uuid FROM application WHERE name = ?`, synthAppName).Scan(&synthAppUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	// Call twice to ensure some units share a net node, but not all.
	cmrState.EnsureUnitsExist(c.Context(), synthAppUUID, []string{unit0name, unit1name})
	cmrState.EnsureUnitsExist(c.Context(), synthAppUUID, []string{unit2name})

	return relUUID
}

func (s *baseSuite) createSubnetForCAASModel(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Only insert the subnet it if doesn't exist.
		var rowCount int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM subnet`).Scan(&rowCount); err != nil {
			return err
		}
		if rowCount != 0 {
			return nil
		}

		subnetUUID := uuid.MustNewUUID().String()
		_, err := tx.ExecContext(ctx, "INSERT INTO subnet (uuid, cidr) VALUES (?, ?)", subnetUUID, "0.0.0.0/0")
		if err != nil {
			return err
		}
		subnetUUID2 := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(ctx, "INSERT INTO subnet (uuid, cidr) VALUES (?, ?)", subnetUUID2, "::/0")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) setCharmObjectStoreMetadata(c *tc.C, appID coreapplication.UUID) {
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
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

func (s *baseSuite) getAllUnitUUIDs(c *tc.C, appID coreapplication.UUID) []unit.UUID {
	var unitUUIDs []unit.UUID
	err := s.ModelTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT uuid FROM unit WHERE application_uuid = ? ORDER BY name`, appID)
		if err != nil {
			return err
		}

		defer func() { _ = rows.Close() }()
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
		defer func() { _ = rows.Close() }()
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

		defer func() { _ = rows.Close() }()
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

func (s *baseSuite) getMachineUUIDFromApp(c *tc.C, appUUID coreapplication.UUID) machine.UUID {
	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	return s.getUnitMachineUUID(c, unitUUID)
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

func (s *baseSuite) advanceApplicationLife(c *tc.C, appUUID coreapplication.UUID, newLife life.Life) {
	_, err := s.DB().Exec("UPDATE application SET life_id = ? WHERE uuid = ?", newLife, appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) advanceUnitLife(c *tc.C, unitUUID unit.UUID, newLife life.Life) {
	_, err := s.DB().Exec("UPDATE unit SET life_id = ? WHERE uuid = ?", newLife, unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) advanceRelationLife(c *tc.C, relationUUID relation.UUID, newLife life.Life) {
	_, err := s.DB().Exec("UPDATE relation SET life_id = ? WHERE uuid = ?", newLife, relationUUID.String())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) advanceMachineLife(c *tc.C, machineUUID machine.UUID, newLife life.Life) {
	_, err := s.DB().Exec("UPDATE machine SET life_id = ? WHERE uuid = ?", newLife, machineUUID.String())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) advanceInstanceLife(c *tc.C, machineUUID machine.UUID, newLife life.Life) {
	_, err := s.DB().Exec("UPDATE machine_cloud_instance SET life_id = ? WHERE machine_uuid = ?", newLife, machineUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) advanceModelLife(c *tc.C, modelUUID string, newLife life.Life) {
	_, err := s.DB().Exec("UPDATE model_life SET life_id = ? WHERE model_uuid = ?", newLife, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) checkApplicationLife(c *tc.C, appUUID string, expectedLife life.Life) {
	s.checkLife(c, "SELECT life_id FROM application WHERE uuid = ?", appUUID, expectedLife)
}

func (s *baseSuite) checkUnitLife(c *tc.C, unitUUID string, expectedLife life.Life) {
	s.checkLife(c, "SELECT life_id FROM unit WHERE uuid = ?", unitUUID, expectedLife)
}

func (s *baseSuite) checkMachineLife(c *tc.C, machineUUID string, expectedLife life.Life) {
	s.checkLife(c, "SELECT life_id FROM machine WHERE uuid = ?", machineUUID, expectedLife)
}

func (s *baseSuite) checkInstanceLife(c *tc.C, machineUUID string, expectedLife life.Life) {
	s.checkLife(c, "SELECT life_id FROM machine_cloud_instance WHERE machine_uuid = ?", machineUUID, expectedLife)
}

func (s *baseSuite) checkModelLife(c *tc.C, modelUUID string, expectedLife life.Life) {
	s.checkLife(c, "SELECT life_id FROM model_life WHERE model_uuid = ?", modelUUID, expectedLife)
}

func (s *baseSuite) checkStorageInstanceLife(c *tc.C, uuid string, expectedLife life.Life) {
	s.checkLife(c, "SELECT life_id FROM storage_instance WHERE uuid = ?", uuid, expectedLife)
}

func (s *baseSuite) checkStorageAttachmentLife(c *tc.C, uuid string, expectedLife life.Life) {
	s.checkLife(c, "SELECT life_id FROM storage_attachment WHERE uuid = ?", uuid, expectedLife)
}

func (s *baseSuite) checkFileSystemLife(c *tc.C, uuid string, expectedLife life.Life) {
	s.checkLife(c, "SELECT life_id FROM storage_filesystem WHERE uuid = ?", uuid, expectedLife)
}

func (s *baseSuite) checkFileSystemAttachmentLife(c *tc.C, uuid string, expectedLife life.Life) {
	s.checkLife(c, "SELECT life_id FROM storage_filesystem_attachment WHERE uuid = ?", uuid, expectedLife)
}

func (s *baseSuite) checkVolumeLife(c *tc.C, uuid string, expectedLife life.Life) {
	s.checkLife(c, "SELECT life_id FROM storage_volume WHERE uuid = ?", uuid, expectedLife)
}

func (s *baseSuite) checkVolumeAttachmentLife(c *tc.C, uuid string, expectedLife life.Life) {
	s.checkLife(c, "SELECT life_id FROM storage_volume_attachment WHERE uuid = ?", uuid, expectedLife)
}

func (s *baseSuite) checkVolumeAttachmentPlanLife(c *tc.C, uuid string, expectedLife life.Life) {
	s.checkLife(c, "SELECT life_id FROM storage_volume_attachment_plan WHERE uuid = ?", uuid, expectedLife)
}

func (s *baseSuite) checkLife(c *tc.C, qry, uuid string, expectedLife life.Life) {
	row := s.DB().QueryRow(qry, uuid)
	var lifeID int
	err := row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, int(expectedLife), tc.Commentf("q: %s", qry))
}

func (s *baseSuite) advanceRemoteApplicationOffererLife(c *tc.C, remoteAppUUID string, newLife life.Life) {
	_, err := s.DB().Exec(`UPDATE application_remote_offerer SET life_id = ? WHERE uuid = ?`, newLife, remoteAppUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) checkRemoteApplicationOffererLife(c *tc.C, remoteAppUUID string, expectedLife life.Life) {
	row := s.DB().QueryRow("SELECT life_id FROM application_remote_offerer WHERE uuid = ?", remoteAppUUID)
	var lifeID int
	err := row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, int(expectedLife))
}

// addCharm inserts a new charm record into the database and returns its UUID as a string.
func (s *baseSuite) addCharm(c *tc.C) string {
	charmUUID := uuid.MustNewUUID().String()
	_, err := s.DB().Exec(`INSERT INTO charm (uuid, reference_name, create_time) VALUES (?, ?, ?)`,
		charmUUID, charmUUID, time.Now().UTC())
	c.Assert(err, tc.ErrorIsNil)
	return charmUUID
}

// addMachine inserts a new machine record and its associated net_node into the
// database, returning the machine UUID.
func (s *baseSuite) addMachine(c *tc.C, name string) string {
	netNodeUUID := uuid.MustNewUUID().String()
	machineUUID := uuid.MustNewUUID().String()
	_, err := s.DB().Exec(`INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().Exec("INSERT INTO machine (uuid, net_node_uuid, name, life_id) VALUES (?, ?, ? ,?)",
		machineUUID, netNodeUUID, name, 0)
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}

// addSubnet adds a subnet and its associated space to the database and returns the subnet UUID.
func (s *baseSuite) addSubnet(c *tc.C, subnet string, spaceName string) string {
	subnetUUID := uuid.MustNewUUID().String()
	spaceUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO space (uuid, name)
VALUES (?, ?)`, spaceUUID, spaceName)
		if err != nil {
			return errors.Errorf("failed to insert space: %v", err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO subnet (uuid, cidr, space_uuid)
VALUES (?, ?, ?)`, subnetUUID, subnet, spaceUUID)
		if err != nil {
			return errors.Errorf("failed to insert subnet: %v", err)
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return subnetUUID
}

// addLinkLayerDevice adds a link layer device to the given machine and returns its UUID.
func (s *baseSuite) addLinkLayerDevice(c *tc.C, name, machineUUID string) string {
	lldUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var netNodeUUID string
		err := tx.QueryRowContext(ctx, `
SELECT net_node_uuid FROM machine WHERE uuid = ?`, machineUUID).Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id)
VALUES (?, ?, ?, ?, ?, ?, ?)`, lldUUID, netNodeUUID, name, 1500, "00:11:22:33:44:55", 0, 0)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO provider_link_layer_device (provider_id, device_uuid)
VALUES (?, ?)`, "provider-id-"+name, lldUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return lldUUID
}

// addLinkLayerDeviceParent adds a parent-child relationship between two link-layer devices.
func (s *baseSuite) addLinkLayerDeviceParent(c *tc.C, parentUUID, childUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO link_layer_device_parent (parent_uuid, device_uuid)
VALUES (?, ?)`, parentUUID, childUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

// addIPAddress adds an IP address to the given machine/link-layer device/subnet and returns its UUID.
func (s *baseSuite) addIPAddress(c *tc.C, machineUUID, lldUUID, subnetUUID string) string {
	ipAddrUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var netNodeUUID string
		err := tx.QueryRowContext(ctx, `
SELECT net_node_uuid FROM machine WHERE uuid = ?`, machineUUID).Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, type_id, scope_id, origin_id, config_type_id, subnet_uuid)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, ipAddrUUID, lldUUID, "10.16.42.9/24", netNodeUUID, 0, 0, 0, 0, subnetUUID)
		if err != nil {
			return errors.Errorf("failed to insert ip_address: %v", err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return ipAddrUUID
}

func (s *baseSuite) addIPAddressProviderID(c *tc.C, providerID, addrUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO provider_ip_address (provider_id, address_uuid)
VALUES (?, ?)`, providerID, addrUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

}

// addUnit inserts a new unit record into the database and returns the generated unit UUID.
func (s *baseSuite) addUnit(c *tc.C, charmUUID string) string {
	return s.addUnitWithName(c, charmUUID, "")
}

// addUnit inserts a new unit record into the database and returns the generated unit UUID.
func (s *baseSuite) addUnitWithName(c *tc.C, charmUUID, name string) string {
	appUUID := uuid.MustNewUUID().String()
	nodeUUID := uuid.MustNewUUID().String()
	unitUUID := uuid.MustNewUUID().String()
	if name == "" {
		name = unitUUID
	}
	_, err := s.DB().Exec(`INSERT INTO net_node (uuid) VALUES (?)`, nodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().Exec(`INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)`,
		appUUID, appUUID, life.Alive, charmUUID, network.AlphaSpaceId)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().Exec(`INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)`,
		unitUUID, name, life.Alive, appUUID, charmUUID, nodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	return unitUUID
}

// addOperation inserts a minimal operation row, returning the new operation UUID.
func (s *baseSuite) addOperation(c *tc.C) string {
	opUUID := uuid.MustNewUUID().String()
	opID := s.nextOperationID()
	enqueued := time.Now().UTC()
	_, err := s.DB().Exec(`INSERT INTO operation (uuid, operation_id, summary, enqueued_at)
VALUES (?, ?, ?, ?)`, opUUID, opID,
		"", enqueued)
	c.Assert(err, tc.ErrorIsNil)
	return opUUID
}

// addOperationAction create a fake operation action linked to a charm action.
// Create all required dependencies and return the operation UUID.
func (s *baseSuite) addOperationAction(c *tc.C, charmUUID, key string) string {
	operationUUID := s.addOperation(c)
	_, err := s.DB().Exec(`INSERT INTO charm_action (charm_uuid, "key") VALUES (?, ?) ON CONFLICT DO NOTHING`, charmUUID, key)
	c.Assert(err, tc.ErrorIsNil)
	// Insert operation_action
	_, err = s.DB().Exec(`INSERT INTO operation_action (operation_uuid, charm_uuid, charm_action_key) VALUES (?, ?, ?)`, operationUUID, charmUUID, key)
	c.Assert(err, tc.ErrorIsNil)
	// Insert operation_parameter
	_, err = s.DB().Exec(`INSERT INTO operation_parameter (operation_uuid, "key", value) VALUES (?, ?, ?)`,
		operationUUID, key, charmUUID)
	c.Assert(err, tc.ErrorIsNil)
	return operationUUID
}

// addOperationTask inserts an operation_task for the given operation,
// with a bunch of dependencies
func (s *baseSuite) addOperationTask(c *tc.C, operationUUID string) string {
	taskUUID := uuid.MustNewUUID().String()
	taskID := s.nextOperationID()
	enqueued := time.Now().UTC()
	_, err := s.DB().Exec(`INSERT INTO operation_task (uuid, operation_uuid, task_id, enqueued_at) VALUES (?, ?, ?, ?)`, taskUUID, operationUUID, taskID, enqueued)
	c.Assert(err, tc.ErrorIsNil)

	s.addOperationTaskOutputWithPath(c, taskUUID, path.Join("op", operationUUID, "task", taskUUID, "output"))
	s.addOperationTaskStatus(c, taskUUID, corestatus.Running)
	s.addOperationTaskLog(c, taskUUID, "test log")
	return taskUUID
}

// addOperationUnitTask links a unit task to an operation. This task is
// created with a bunch of dependencies (output, results, logs)
func (s *baseSuite) addOperationUnitTask(c *tc.C, operationUUID, unitUUID string) string {
	taskUUID := s.addOperationTask(c, operationUUID)
	_, err := s.DB().Exec(`INSERT INTO operation_unit_task (task_uuid, unit_uuid) VALUES (?, ?)`, taskUUID, unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	return taskUUID
}

// addOperationMachineTask links a machine task to an operation. This task is
// // created with a bunch of dependencies (output, results, logs)
func (s *baseSuite) addOperationMachineTask(c *tc.C, operationUUID, machineUUID string) string {
	taskUUID := s.addOperationTask(c, operationUUID)
	_, err := s.DB().Exec(`INSERT INTO operation_machine_task (task_uuid, machine_uuid) VALUES (?, ?)`, taskUUID, machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	return taskUUID
}

func (s *baseSuite) addFakeMetadataStore(c *tc.C, size int) string {
	storeUUID := uuid.MustNewUUID().String()
	_, err := s.DB().Exec(`
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES (?, ?, ?, ?)`, storeUUID, storeUUID, storeUUID, size)
	c.Assert(err, tc.ErrorIsNil)
	return storeUUID
}

// addOperationTaskOutput links a task to an object store metadata, return the
// store metadata uuid
func (s *baseSuite) addOperationTaskOutputWithPath(c *tc.C, taskUUID string, path string) string {
	storeUUID := s.addFakeMetadataStore(c, 42)
	s.linkMetadataStorePath(c, storeUUID, path)
	s.linkOperationTaskOutput(c, taskUUID, path)
	return storeUUID
}

// linkMetadataStorePath links a store metadata to a path
func (s *baseSuite) linkMetadataStorePath(c *tc.C, storeUUID, path string) {
	_, err := s.DB().Exec(`INSERT INTO object_store_metadata_path (path, metadata_uuid) VALUES (?, ?)`, path, storeUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// linkOperationTaskOutput links an operation task to an object store metadata entry in the database.
func (s *baseSuite) linkOperationTaskOutput(c *tc.C, taskUUID, path string) {
	_, err := s.DB().Exec(`INSERT INTO operation_task_output (task_uuid, store_path) VALUES (?, ?)`, taskUUID, path)
	c.Assert(err, tc.ErrorIsNil)
}

// addOperationTaskStatus sets a status for the task with the given textual status name.
func (s *baseSuite) addOperationTaskStatus(c *tc.C, taskUUID string, status corestatus.Status) {
	beforeCount := s.getRowCount(c, "operation_task_status")
	// Map status to id via the table
	_, err := s.DB().Exec(`INSERT INTO operation_task_status (task_uuid, status_id, updated_at)
		SELECT ?, id, ? FROM operation_task_status_value WHERE status = ?`, taskUUID, time.Now().UTC(), status)
	c.Assert(err, tc.ErrorIsNil)
	afterCount := s.getRowCount(c, "operation_task_status")
	c.Assert(afterCount, tc.Equals, beforeCount+1, tc.Commentf("status %q is not valid, is any of %v", status,
		s.selectDistinctValues(c, "status", "operation_task_status_value")))
}

// addOperationTaskLog inserts a log message for a task.
func (s *baseSuite) addOperationTaskLog(c *tc.C, taskUUID, content string) {
	_, err := s.DB().Exec(`INSERT INTO operation_task_log (task_uuid, content, created_at) VALUES (?, ?, ?)`,
		taskUUID, content, time.Now().UTC())
	c.Assert(err, tc.ErrorIsNil)
}

// getRowCount returns the number of rows in a table.
func (s *baseSuite) getRowCount(c *tc.C, table string) int {
	var obtained int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		return tx.QueryRowContext(ctx, query).Scan(&obtained)
	})
	c.Assert(err, tc.IsNil, tc.Commentf("counting rows in table %q", table))
	return obtained
}

// selectDistinctValues retrieves distinct values for a given field from a table.
func (s *baseSuite) selectDistinctValues(c *tc.C, field, table string) []string {
	var obtained []string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		query := fmt.Sprintf("SELECT DISTINCT %q FROM %q", field, table)
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var val *string
			if err := rows.Scan(&val); err != nil {
				return err
			}
			if val == nil {
				obtained = append(obtained, "")
			} else {
				obtained = append(obtained, *val)
			}
		}
		return nil
	})
	c.Assert(err, tc.IsNil, tc.Commentf("fetching distinct %q from table %q", field, table))
	return obtained
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
		Provides: map[string]internalcharm.Relation{
			"foo": {
				Name:      "foo",
				Role:      internalcharm.RoleProvider,
				Interface: "rel",
				Scope:     internalcharm.ScopeGlobal,
			},
		},
		Requires: map[string]internalcharm.Relation{
			"bar": {
				Name:      "bar",
				Role:      internalcharm.RoleRequirer,
				Interface: "rel",
				Scope:     internalcharm.ScopeGlobal,
			},
		},
		Devices: map[string]internalcharm.Device{
			"bitcoinminer": {
				Type: "nvidia.com/gpu",
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

func (s *stubCharm) Config() *internalcharm.ConfigSpec {
	return &internalcharm.ConfigSpec{
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
	}, {
		Id: "some-otherapp-0",
	}}, nil
}

func ptr[T any](v T) *T {
	return &v
}
