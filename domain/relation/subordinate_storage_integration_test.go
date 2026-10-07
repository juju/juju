// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	corecharmtesting "github.com/juju/juju/core/charm/testing"
	coredatabase "github.com/juju/juju/core/database"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	corenetwork "github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	corestatus "github.com/juju/juju/core/status"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	applicationstorageservice "github.com/juju/juju/domain/application/service/storage"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment/charm"
	relationservice "github.com/juju/juju/domain/relation/service"
	relationstate "github.com/juju/juju/domain/relation/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/statushistory"
	internalstorage "github.com/juju/juju/internal/storage"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	"github.com/juju/juju/internal/uuid"
)

// subordinateStorageIntegrationSuite tests the integration seam between the
// relation service and the real application storage service: entering scope
// of a container scoped relation creates a subordinate unit whose storage is
// provisioned by the real storage service, driven end to end against the
// model database.
type subordinateStorageIntegrationSuite struct {
	schematesting.ModelSuite

	// relationCount helps generation of consecutive relation_id values.
	relationCount int
}

func TestSubordinateStorageIntegrationSuite(t *testing.T) {
	tc.Run(t, &subordinateStorageIntegrationSuite{})
}

// TestEnterScopeSubordinateStorageProvisioned wires the real application
// storage service into the relation service, and asserts that entering
// scope of a container scoped relation with a unit of a principal
// application creates the subordinate unit with storage provisioned from
// the subordinate application's storage directives: the unit storage
// directive, storage instance, attachment, ownership and machine records
// all exist for the new subordinate unit.
func (s *subordinateStorageIntegrationSuite) TestEnterScopeSubordinateStorageProvisioned(c *tc.C) {
	// Arrange: principal application with a unit on a machine.
	principalCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, principalCharmUUID, false)
	principalApplicationUUID := s.addApplication(c, principalCharmUUID, "pri")
	principalUnitName := coreunittesting.GenNewName(c, "pri/0")
	principalUnitUUID := s.addUnit(c, principalUnitName, principalApplicationUUID, principalCharmUUID)
	principalMachineUUID := s.addMachineToUnit(c, principalUnitUUID.String())
	principalNetNodeUUID := s.getUnitNetNode(c, principalUnitUUID.String())

	// Arrange: provision the machine, as placing a unit on it requires the
	// machine to have hardware characteristics.
	s.query(c, `
INSERT INTO machine_cloud_instance (machine_uuid, life_id, instance_id, arch)
VALUES (?, 0, 'instance-0', 'amd64')
`, principalMachineUUID)

	// Arrange: subordinate application with a filesystem storage directive
	// backed by a machine scoped storage pool.
	subordinateCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, subordinateCharmUUID, true)
	subordinateApplicationUUID := s.addApplication(c, subordinateCharmUUID, "sub")
	poolUUID := s.addStoragePool(c)
	s.addCharmStorage(c, subordinateCharmUUID)
	s.addApplicationStorageDirective(c, subordinateApplicationUUID, subordinateCharmUUID, poolUUID)

	// Arrange: container scoped relation between the two applications. The
	// principal unit is not yet in scope: entering scope creates the
	// relation unit and the subordinate unit.
	relationUUID, _, _ := s.addContainerScopedRelation(
		c, principalApplicationUUID, principalCharmUUID,
		subordinateApplicationUUID, subordinateCharmUUID,
	)

	// Arrange: wire the relation service with the real relation state, the
	// real application storage service and a registry of dummy storage
	// providers backing the storage pool.
	svc := s.setupRelationService(c)

	// Act: enter scope with the principal unit, which creates the
	// subordinate unit and provisions its storage.
	err := svc.EnterScope(c.Context(), relationUUID, principalUnitName, map[string]string{
		"ingress": "x.x.x.x",
	})
	c.Assert(err, tc.ErrorIsNil)

	// Assert: the subordinate unit was created and keyed to the principal
	// unit.
	subordinateUnitUUID := s.getUnitUUIDByName(c, "sub/0")
	s.checkRowCount(c, "unit_principal", "unit_uuid", subordinateUnitUUID, 1)

	// Assert: the unit storage directive was created for the subordinate
	// unit from the application storage directive.
	s.checkRowCount(c, "unit_storage_directive", "unit_uuid", subordinateUnitUUID, 1)

	// Assert: one filesystem storage instance is owned by the subordinate
	// unit.
	s.checkStorageInstanceCount(c, subordinateUnitUUID, "data", 1)

	// Assert: the storage instance is attached to the subordinate unit.
	s.checkRowCount(c, "storage_attachment", "unit_uuid", subordinateUnitUUID, 1)

	// Assert: the filesystem is owned by the machine hosting the principal
	// unit.
	s.checkRowCount(c, "machine_filesystem", "machine_uuid", principalMachineUUID, 1)

	// Assert: the filesystem is attached to the machine's net node, the net
	// node of the principal unit.
	s.checkRowCount(c, "storage_filesystem_attachment", "net_node_uuid", principalNetNodeUUID, 1)
}

// setupRelationService wires the relation service with the real relation
// state, the real storage pool provider and a registry of dummy storage
// providers, the same way the controller does.
func (s *subordinateStorageIntegrationSuite) setupRelationService(c *tc.C) *relationservice.Service {
	modelDB := func(ctx context.Context) (coredatabase.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	log := loggertesting.WrapCheckLog(c)

	// The relation domain creates subordinate units on behalf of the
	// application domain, using the real IAAS unit insertion state.
	unitState := applicationstate.NewInsertIAASUnitState(modelDB, clock.WallClock, log)
	relState := relationstate.NewState(modelDB, clock.WallClock, log, unitState)

	// The storage arguments for the subordinate units are made by the
	// relation service from the storage directives read by the relation
	// state, resolving the storage pools through the same pool provider
	// implementation as the application domain, backed by the dummy
	// storage providers.
	appState := applicationstate.NewState(
		modelDB, coremodel.UUID(s.ModelUUID()), clock.WallClock, log,
	)
	poolProvider := applicationstorageservice.NewStoragePoolProvider(
		corestorage.ConstModelStorageRegistry(
			func() internalstorage.ProviderRegistry {
				return dummystorage.StorageProviders()
			},
		),
		appState,
	)

	return relationservice.NewService(relState, poolProvider, noopStatusHistory{}, log)
}

// query executes a given SQL query with optional arguments within the model
// database.
func (s *subordinateStorageIntegrationSuite) query(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}

// addCharm inserts a new charm into the database and returns the UUID.
func (s *subordinateStorageIntegrationSuite) addCharm(c *tc.C) corecharm.ID {
	charmUUID := corecharmtesting.GenCharmID(c)
	// The UUID is also used as the reference_name as there is a unique
	// constraint on the reference_name, revision and source_id.
	s.query(c, `
INSERT INTO charm (uuid, reference_name, architecture_id)
VALUES (?, ?, 0)
`, charmUUID, charmUUID)
	return charmUUID
}

// addCharmMetadata inserts charm metadata, marking the charm subordinate or
// not.
func (s *subordinateStorageIntegrationSuite) addCharmMetadata(c *tc.C, charmUUID corecharm.ID, subordinate bool) {
	s.query(c, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate)
VALUES (?, ?, ?)
`, charmUUID, charmUUID, subordinate)
}

// addApplication adds a new application to the database with the specified
// charm UUID and application name. It returns the application UUID.
func (s *subordinateStorageIntegrationSuite) addApplication(c *tc.C, charmUUID corecharm.ID, appName string) coreapplication.UUID {
	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.query(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid)
VALUES (?, ?, ?, ?, ?)
`, appUUID, appName, 0 /* alive */, charmUUID.String(), corenetwork.AlphaSpaceId)
	return appUUID
}

// addUnit adds a new unit of the given application to the database with the
// given name. It returns the unit UUID.
func (s *subordinateStorageIntegrationSuite) addUnit(
	c *tc.C, unitName coreunit.Name, appUUID coreapplication.UUID, charmUUID corecharm.ID,
) coreunit.UUID {
	unitUUID := coreunittesting.GenUnitUUID(c)
	netNodeUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO net_node (uuid)
VALUES (?)
ON CONFLICT DO NOTHING
`, netNodeUUID)

	s.query(c, `
INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid)
VALUES (?, ?, ?, ?, ?, ?)
`, unitUUID, unitName, 0 /* alive */, appUUID, charmUUID, netNodeUUID)
	return unitUUID
}

// addMachineToUnit adds a machine sharing the net node of the given unit.
// It returns the machine UUID.
func (s *subordinateStorageIntegrationSuite) addMachineToUnit(c *tc.C, unitUUID string) string {
	machineUUID := tc.Must(c, coremachine.NewUUID).String()
	s.query(c, `
INSERT INTO machine (uuid, name, life_id, net_node_uuid)
SELECT ?, '0', 0, net_node_uuid
FROM unit
WHERE uuid = ?
`, machineUUID, unitUUID)
	return machineUUID
}

// addStoragePool adds a storage pool backed by the machine scoped dummy
// storage provider. It returns the pool UUID.
func (s *subordinateStorageIntegrationSuite) addStoragePool(c *tc.C) domainstorage.StoragePoolUUID {
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	s.query(c, `
INSERT INTO storage_pool (uuid, name, type)
VALUES (?, 'test-pool', 'machinescoped')
`, poolUUID)
	return poolUUID
}

// addCharmStorage adds a filesystem charm storage definition named "data"
// to the given charm.
func (s *subordinateStorageIntegrationSuite) addCharmStorage(c *tc.C, charmUUID corecharm.ID) {
	s.query(c, `
INSERT INTO charm_storage (charm_uuid, name, description, storage_kind_id, shared, read_only, count_min, count_max, minimum_size_mib, location)
VALUES (?, 'data', 'data', 1, false, false, 1, 1, 1024, '/')
`, charmUUID)
}

// addApplicationStorageDirective adds a storage directive named "data" to
// the given application, backed by the given storage pool.
func (s *subordinateStorageIntegrationSuite) addApplicationStorageDirective(
	c *tc.C, appUUID coreapplication.UUID, charmUUID corecharm.ID, poolUUID domainstorage.StoragePoolUUID,
) {
	s.query(c, `
INSERT INTO application_storage_directive (application_uuid, charm_uuid, storage_name, storage_pool_uuid, size_mib, count)
VALUES (?, ?, 'data', ?, 1024, 1)
`, appUUID, charmUUID, poolUUID)
}

// addContainerScopedRelation adds a container scoped relation between the
// two applications. It returns the relation UUID and the relation endpoint
// UUIDs of the first and second application, in that order.
func (s *subordinateStorageIntegrationSuite) addContainerScopedRelation(
	c *tc.C,
	app1ID coreapplication.UUID, charm1UUID corecharm.ID,
	app2ID coreapplication.UUID, charm2UUID corecharm.ID,
) (corerelation.UUID, string, string) {
	endpoint1 := charm.Relation{
		Name:      "fake-endpoint-name-1",
		Role:      charm.RoleProvider,
		Interface: "database",
		Scope:     charm.ScopeContainer,
	}
	endpoint2 := charm.Relation{
		Name:      "fake-endpoint-name-2",
		Role:      charm.RoleRequirer,
		Interface: "database",
		Scope:     charm.ScopeContainer,
	}
	charmRelationUUID1 := s.addCharmRelation(c, charm1UUID, endpoint1)
	charmRelationUUID2 := s.addCharmRelation(c, charm2UUID, endpoint2)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, app1ID, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, app2ID, charmRelationUUID2)
	relationUUID := s.addRelation(c, charm.ScopeContainer)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	relationEndpointUUID2 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	return relationUUID, relationEndpointUUID1, relationEndpointUUID2
}

// addCharmRelation inserts a new charm relation into the database with the
// given attributes. It returns the charm relation UUID.
func (s *subordinateStorageIntegrationSuite) addCharmRelation(c *tc.C, charmUUID corecharm.ID, r charm.Relation) string {
	charmRelationUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, interface, optional, capacity, scope_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, charmRelationUUID, charmUUID, r.Name, s.encodeRoleID(r.Role), r.Interface, r.Optional, r.Limit, s.encodeScopeID(r.Scope))
	return charmRelationUUID
}

// addApplicationEndpoint inserts a new application endpoint into the
// database. It returns the endpoint UUID.
func (s *subordinateStorageIntegrationSuite) addApplicationEndpoint(
	c *tc.C, applicationUUID coreapplication.UUID, charmRelationUUID string,
) string {
	applicationEndpointUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid, space_uuid)
VALUES (?, ?, ?, ?)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID, corenetwork.AlphaSpaceId)
	return applicationEndpointUUID
}

// addRelation inserts a new relation with the given scope. It returns the
// relation UUID.
func (s *subordinateStorageIntegrationSuite) addRelation(c *tc.C, scope charm.RelationScope) corerelation.UUID {
	relationUUID := corerelationtesting.GenRelationUUID(c)
	s.query(c, `
INSERT INTO relation (uuid, life_id, relation_id, scope_id)
VALUES (?, 0, ?, ?)
`, relationUUID, s.relationCount, s.encodeScopeID(scope))
	s.relationCount++
	return relationUUID
}

// addRelationEndpoint inserts a new relation endpoint into the database. It
// returns the relation endpoint UUID.
func (s *subordinateStorageIntegrationSuite) addRelationEndpoint(
	c *tc.C, relationUUID corerelation.UUID, applicationEndpointUUID string,
) string {
	relationEndpointUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)
`, relationEndpointUUID, relationUUID, applicationEndpointUUID)
	return relationEndpointUUID
}

// encodeRoleID returns the ID used in the database for the given charm
// relation role.
func (s *subordinateStorageIntegrationSuite) encodeRoleID(role charm.RelationRole) int {
	return map[charm.RelationRole]int{
		charm.RoleProvider: 0,
		charm.RoleRequirer: 1,
		charm.RolePeer:     2,
	}[role]
}

// encodeScopeID returns the ID used in the database for the given charm
// scope.
func (s *subordinateStorageIntegrationSuite) encodeScopeID(scope charm.RelationScope) int {
	return map[charm.RelationScope]int{
		charm.ScopeGlobal:    0,
		charm.ScopeContainer: 1,
	}[scope]
}

// getUnitNetNode returns the net node UUID of the given unit.
func (s *subordinateStorageIntegrationSuite) getUnitNetNode(c *tc.C, unitUUID string) string {
	var netNodeUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(
			ctx, "SELECT net_node_uuid FROM unit WHERE uuid = ?", unitUUID,
		).Scan(&netNodeUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	return netNodeUUID
}

// getUnitUUIDByName returns the UUID of the unit with the given name.
func (s *subordinateStorageIntegrationSuite) getUnitUUIDByName(c *tc.C, name string) string {
	var unitUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(
			ctx, "SELECT uuid FROM unit WHERE name = ?", name,
		).Scan(&unitUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	return unitUUID
}

// checkRowCount checks the number of rows in the given table matching the
// given column value.
func (s *subordinateStorageIntegrationSuite) checkRowCount(c *tc.C, table, column, value string, expected int) {
	row := s.DB().QueryRow(
		fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s = ?`, table, column), value)
	var count int
	err := row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, expected,
		tc.Commentf("table %q, column %q, value %q", table, column, value))
}

// checkStorageInstanceCount checks the number of storage instances with the
// given storage name owned by the given unit.
func (s *subordinateStorageIntegrationSuite) checkStorageInstanceCount(c *tc.C, unitUUID, storageName string, expected int) {
	row := s.DB().QueryRow(`
SELECT count(*)
FROM   storage_instance AS si
JOIN   storage_unit_owner AS suo ON suo.storage_instance_uuid = si.uuid
WHERE  suo.unit_uuid = ?
AND    si.storage_name = ?
`, unitUUID, storageName)
	var count int
	err := row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, expected,
		tc.Commentf("unit %q storage %q", unitUUID, storageName))
}

// noopStatusHistory is a [relationservice.StatusHistory] that discards
// recorded statuses; the assertions are made on the database rows.
type noopStatusHistory struct{}

// RecordStatus is defined on [relationservice.StatusHistory].
func (noopStatusHistory) RecordStatus(
	_ context.Context, _ statushistory.Namespace, _ corestatus.StatusInfo,
) error {
	return nil
}
