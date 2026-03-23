// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	corenetwork "github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	deploymentcharm "github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type baseSuite struct {
	schematesting.ModelSuite
	state *State

	unitUUID string
	unitName string
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	modelUUID := tc.Must(c, model.NewUUID)
	s.query(c, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod", "iaas", "test-model", "ec2")
		`, modelUUID, coretesting.ControllerTag.Id())

	appState := applicationstate.NewState(s.TxnRunnerFactory(), modelUUID, clock.WallClock, loggertesting.WrapCheckLog(c))

	appArg := application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: "app",
				},
				Manifest: charm.Manifest{
					Bases: []charm.Base{{
						Name:          "ubuntu",
						Channel:       charm.Channel{Risk: charm.RiskStable},
						Architectures: []string{"amd64"},
					}},
				},
				ReferenceName: "app",
				Source:        charm.LocalSource,
				Architecture:  architecture.AMD64,
			},
		},
	}

	s.unitName = unittesting.GenNewName(c, "app/0").String()
	unitArgs := []application.AddIAASUnitArg{{}}

	ctx := c.Context()
	_, _, err := appState.CreateIAASApplication(ctx, "app", appArg, unitArgs)
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&s.unitUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Cleanup(func() {
		s.state = nil
		s.unitName = ""
		s.unitUUID = ""
	})
}

func (s *baseSuite) addUnitStateCharm(c *tc.C, key any, value string) {
	q := "INSERT INTO unit_state_charm VALUES (?, ?, ?)"
	s.query(c, q, s.unitUUID, key, value)
}

func (s *baseSuite) addCharm(c *tc.C) string {
	charmUUID := tc.Must(c, corecharm.NewID).String()
	s.query(c, `INSERT INTO charm (uuid, reference_name, create_time) VALUES (?, ?, ?)`,
		charmUUID, charmUUID, time.Now())
	return charmUUID
}

func (s *baseSuite) addApplicationWithName(c *tc.C, charmUUID, appName, spaceUUID string) string {
	appUUID := tc.Must(c, coreapplication.NewUUID).String()
	s.query(c, `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)`,
		appUUID, appName, life.Alive, charmUUID, spaceUUID)
	return appUUID
}

func (s *baseSuite) checkUnitUUID(c *tc.C, unitUUID string) {
	var uuid string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&uuid)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, unitUUID)
}

// Txn executes a transactional function within a database context,
// ensuring proper error handling and assertion.
func (s *baseSuite) Txn(c *tc.C, fn func(ctx context.Context, tx *sqlair.TX) error) error {
	db, err := s.state.DB(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	return db.Txn(c.Context(), fn)
}

// query executes a given SQL query with optional arguments within a
// transactional context using the test database.
func (s *baseSuite) query(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%v: query: %s (args: %v)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}

type commitHookBaseSuite struct {
	baseSuite

	fakeCharmUUID1                string
	fakeCharmUUID2                string
	fakeApplicationUUID1          string
	fakeApplicationUUID2          string
	fakeApplicationName1          string
	fakeApplicationName2          string
	fakeCharmRelationProvidesUUID string

	// relationCount helps generation of consecutive relation_id
	relationCount int
}

func (s *commitHookBaseSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.fakeApplicationName1 = "fake-application-1"
	s.fakeApplicationName2 = "fake-application-2"

	// Populate DB with one application and charm.
	s.fakeCharmUUID1 = s.addCharm(c)
	s.fakeCharmUUID2 = s.addCharm(c)
	s.fakeCharmRelationProvidesUUID = s.addCharmRelationWithDefaults(c, s.fakeCharmUUID1)
	s.fakeApplicationUUID1 = s.addApplicationWithName(c, s.fakeCharmUUID1, s.fakeApplicationName1, network.AlphaSpaceId.String())
	s.fakeApplicationUUID2 = s.addApplicationWithName(c, s.fakeCharmUUID2, s.fakeApplicationName2, network.AlphaSpaceId.String())

	c.Cleanup(func() {
		s.fakeCharmUUID1 = ""
		s.fakeCharmUUID2 = ""
		s.fakeApplicationName1 = ""
		s.fakeApplicationName2 = ""
		s.fakeApplicationUUID1 = ""
		s.fakeApplicationUUID2 = ""
		s.fakeCharmRelationProvidesUUID = ""
		s.relationCount = 0
	})
}

// addCharmRelation inserts a new charm relation into the database with the
// given UUID and attributes. Returns the relation UUID.
func (s *commitHookBaseSuite) addCharmRelation(c *tc.C, charmUUID string, r deploymentcharm.Relation) string {
	charmRelationUUID := tc.Must(c, uuid.NewUUID).String()
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, interface, optional, capacity, scope_id) 
VALUES (?, ?, ?,
       (SELECT id FROM charm_relation_role WHERE name = ?),
       ?, ?, ?,
       (SELECT id FROM charm_relation_scope WHERE name = ?))
`, charmRelationUUID, charmUUID, r.Name, r.Role, r.Interface, r.Optional, r.Limit, r.Scope)
	return charmRelationUUID
}

// addRelation inserts a new relation into the database with default relation
// and life IDs. Returns the relation UUID.
func (s *commitHookBaseSuite) addRelation(c *tc.C) corerelation.UUID {
	relationUUID := tc.Must(c, corerelation.NewUUID)
	s.query(c, `
INSERT INTO relation (uuid, life_id, relation_id, scope_id) 
VALUES (?,0,?,0)
`, relationUUID, s.relationCount)
	s.relationCount++
	return relationUUID
}

// addRelationUnit inserts a relation unit into the database using the
// provided UUIDs for relation. Returns the relation unit UUID.
func (s *commitHookBaseSuite) addRelationUnit(c *tc.C, unitUUID coreunit.UUID, relationEndpointUUID string) corerelation.UnitUUID {
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.query(c, `
INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid)
VALUES (?,?,?)
`, relationUnitUUID, relationEndpointUUID, unitUUID)
	return relationUnitUUID
}

// addRelationEndpoint inserts a relation endpoint into the database
// using the provided UUIDs for relation. Returns the endpoint UUID.
func (s *commitHookBaseSuite) addRelationEndpoint(
	c *tc.C, relationUUID corerelation.UUID, applicationEndpointUUID string,
) string {
	relationEndpointUUID := tc.Must(c, corerelation.NewEndpointUUID).String()
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?,?,?)
`, relationEndpointUUID, relationUUID, applicationEndpointUUID)
	return relationEndpointUUID
}

// addApplicationEndpoint inserts a new application endpoint into the
// database with the specified UUIDs. Returns the endpoint uuid.
func (s *commitHookBaseSuite) addApplicationEndpoint(
	c *tc.C, applicationUUID, charmRelationUUID string,
) string {
	applicationEndpointUUID := tc.Must(c, uuid.NewUUID).String()
	var spacePtr *string
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid, space_uuid)
VALUES (?, ?, ?, ?)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID, spacePtr)
	return applicationEndpointUUID
}

// addCharmRelationWithDefaults inserts a new charm relation into the database
// with the given UUID and predefined attributes. Returns the relation UUID.
func (s *commitHookBaseSuite) addCharmRelationWithDefaults(c *tc.C, charmUUID string) string {
	charmRelationUUID := tc.Must(c, uuid.NewUUID).String()
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name)
VALUES (?, ?, 0, 0, 'fake-provides')
`, charmRelationUUID, charmUUID)
	return charmRelationUUID
}

// addRelationApplicationSetting inserts a relation application setting into the database
// using the provided relation and application UUID.
func (s *commitHookBaseSuite) addRelationApplicationSetting(c *tc.C, relationEndpointUUID, key, value string) {
	s.query(c, `
INSERT INTO relation_application_setting (relation_endpoint_uuid, key, value)
VALUES (?,?,?)
`, relationEndpointUUID, key, value)
}

// addRelationUnitSetting inserts a relation unit setting into the database
// using the provided relationUnitUUID.
func (s *commitHookBaseSuite) addRelationUnitSetting(c *tc.C, relationUnitUUID corerelation.UnitUUID, key, value string) {
	s.query(c, `
INSERT INTO relation_unit_setting (relation_unit_uuid, key, value)
VALUES (?,?,?)
`, relationUnitUUID, key, value)
}

// getRelationUnitSettings gets the relation application settings.
func (s *commitHookBaseSuite) getRelationUnitSettings(c *tc.C, relationUnitUUID corerelation.UnitUUID) map[string]string {
	settings := map[string]string{}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT key, value
FROM relation_unit_setting
WHERE relation_unit_uuid = ?
`, relationUnitUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		var (
			key, value string
		)
		for rows.Next() {
			if err := rows.Scan(&key, &value); err != nil {
				return errors.Capture(err)
			}
			settings[key] = value
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) getting relation settings: %s",
		errors.ErrorStack(err)))
	return settings
}

func (s *commitHookBaseSuite) getRelationUnitSettingsHash(c *tc.C, relationUnitUUID corerelation.UnitUUID) string {
	var hash string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT sha256
FROM   relation_unit_settings_hash
WHERE  relation_unit_uuid = ?
`, relationUnitUUID).Scan(&hash)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return hash
}

// getRelationApplicationSettings gets the relation application settings.
func (s *commitHookBaseSuite) getRelationApplicationSettings(c *tc.C, relationEndpointUUID string) map[string]string {
	settings := map[string]string{}
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT key, value
FROM relation_application_setting 
WHERE relation_endpoint_uuid = ?
`, relationEndpointUUID)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		var (
			key, value string
		)
		for rows.Next() {
			if err := rows.Scan(&key, &value); err != nil {
				return errors.Capture(err)
			}
			settings[key] = value
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) getting relation settings: %s",
		errors.ErrorStack(err)))
	return settings
}

func (s *commitHookBaseSuite) getRelationApplicationSettingsHash(c *tc.C, relationEndpointUUID string) string {
	var hash string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT sha256
FROM   relation_application_settings_hash
WHERE  relation_endpoint_uuid = ?
`, relationEndpointUUID).Scan(&hash)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return hash
}

func (s *commitHookBaseSuite) addCharm(c *tc.C) string {
	charmUUID := tc.Must(c, uuid.NewUUID).String()
	s.query(c, `INSERT INTO charm (uuid, reference_name, create_time) VALUES (?, ?, ?)`,
		charmUUID, charmUUID, time.Now())
	return charmUUID
}

func (s *commitHookBaseSuite) addNetNode(c *tc.C) string {
	netNodeUUID := tc.Must(c, uuid.NewUUID).String()
	s.query(c, "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
	return netNodeUUID
}

func (s *commitHookBaseSuite) addSpace(c *tc.C) string {
	spaceUUID := tc.Must(c, uuid.NewUUID).String()
	s.query(c, `INSERT INTO space (uuid, name) VALUES (?, ?)`,
		spaceUUID, spaceUUID)
	return spaceUUID
}

func (s *commitHookBaseSuite) addApplication(c *tc.C, charmUUID, spaceUUID string) string {
	appUUID := tc.Must(c, uuid.NewUUID).String()
	s.query(c, `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)`,
		appUUID, appUUID, life.Alive, charmUUID, spaceUUID)
	return appUUID
}

func (s *commitHookBaseSuite) addUnit(c *tc.C, appUUID, charmUUID, nodeUUID string) coreunit.UUID {
	unitUUID := tc.Must(c, coreunit.NewUUID)
	s.query(c, `INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)`,
		unitUUID, unitUUID, life.Alive, appUUID, charmUUID, nodeUUID)
	return unitUUID
}

// addUnitAndNetNode adds a new unit to the specified application in the
// database with the given UUID and name. A netnode is created for the unit.
// Returns the unit uuid.
func (s *commitHookBaseSuite) addUnitAndNetNode(c *tc.C, unitName coreunit.Name, appUUID, charmUUID string) coreunit.UUID {
	unitUUID := tc.Must(c, coreunit.NewUUID)
	netNodeUUID := tc.Must(c, uuid.NewUUID).String()
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

// addApplicationExtraEndpoint inserts a new application extra endpoint into the
// database with the specified UUIDs. Returns the endpoint uuid.
func (s *commitHookBaseSuite) addApplicationExtraEndpoint(
	c *tc.C, applicationUUID coreapplication.UUID, charmRelationUUID string, boundSpaceUUID string) {
	s.query(c, `
INSERT INTO application_extra_endpoint (application_uuid, charm_extra_binding_uuid,space_uuid)
VALUES (?, ?, ?)
`, applicationUUID, charmRelationUUID, nilZeroPtr(boundSpaceUUID))
}

// addApplicationExposedEndpoint inserts a record linking an application,
// its exposed endpoint, and the associated space into the database.
func (s *commitHookBaseSuite) addApplicationExposedEndpoint(c *tc.C, applicationUUID, endpointUUID, boundSpaceUUID string) {
	s.query(c, `INSERT INTO application_exposed_endpoint_space (application_uuid, application_endpoint_uuid, space_uuid) 
			VALUES (?, ?, ?)`, applicationUUID, endpointUUID, boundSpaceUUID)
}

// addCharmExtraBinding inserts a new record into the charm_extra_binding table
// and returns the generated UUID.
func (s *commitHookBaseSuite) addCharmExtraBinding(c *tc.C, charmUUID corecharm.ID, name string) string {
	uuid := tc.Must(c, uuid.NewUUID).String()
	s.query(c, `
INSERT INTO charm_extra_binding (uuid, charm_uuid, name) VALUES (?, ?, ?)`, uuid, charmUUID, name)
	return uuid
}
func (s *commitHookBaseSuite) addMachine(c *tc.C, name, netNodeUUID string) machine.UUID {
	machineUUID := machinetesting.GenUUID(c)
	s.query(c, "INSERT INTO machine (uuid, net_node_uuid, name, life_id) VALUES (?, ?, ? ,?)",
		machineUUID.String(), netNodeUUID, name, 0)
	return machineUUID
}

func (s *commitHookBaseSuite) addSpaceWithName(c *tc.C, name string) string {
	spaceUUID := tc.Must(c, uuid.NewUUID).String()
	s.query(c, `INSERT INTO space (uuid, name) VALUES (?, ?)`,
		spaceUUID, name)
	return spaceUUID
}

// addLinkLayerDevice adds a link layer device to the database and returns its UUID.
func (s *commitHookBaseSuite) addLinkLayerDevice(
	c *tc.C, netNodeUUID, name, macAddress string, deviceType corenetwork.LinkLayerDeviceType,
) string {
	deviceUUID := "device-" + name + "-uuid"

	mtu := int64(1500)

	s.query(c, `
INSERT INTO link_layer_device (
	uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id, 
    is_auto_start, is_enabled, is_default_gateway, gateway_address, vlan_tag)
VALUES (?, ?, ?, ?, ?,
       (SELECT id FROM link_layer_device_type WHERE name = ?),
       (SELECT id FROM virtual_port_type WHERE name = ""),
       ?, ?, ?, ?, ?)
	`, deviceUUID, netNodeUUID, name, mtu, macAddress, deviceType, true,
		true, false, nil, 0)

	return deviceUUID
}

func (s *commitHookBaseSuite) addSubnet(c *tc.C, cidr string, spaceUUID string) string {
	uuid := "subnet-" + cidr + "-uuid"
	s.query(c, `
INSERT INTO subnet (uuid, cidr, space_uuid) 
VALUES (?, ?, ?)`, uuid, cidr, spaceUUID)
	return uuid
}

// addIPAddressWithSubnet adds an IP address to the database and returns its UUID.
func (s *commitHookBaseSuite) addIPAddressWithSubnetAndScope(c *tc.C, deviceUUID, netNodeUUID,
	subnetUUID, addressValue string, scope corenetwork.Scope) string {
	addressUUID := "address-" + addressValue + "-uuid"

	s.query(c, `
		INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, subnet_uuid, type_id, config_type_id, origin_id, scope_id)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, scope.id
		FROM ip_address_scope AS scope
		WHERE scope.name = ?
	`, addressUUID, deviceUUID, addressValue, netNodeUUID, subnetUUID, 0, 4, 1, string(scope))

	return addressUUID
}

// addK8sService inserts a new Kubernetes service into the database with the associated node, application, and
// provider ID.
func (s *commitHookBaseSuite) addK8sService(c *tc.C, nodeUUID, appUUID string) {
	svcUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO k8s_service (uuid, net_node_uuid, application_uuid, provider_id) VALUES (?, ?, ?, ?)`,
		svcUUID, nodeUUID, appUUID, "provider-id")
}

// addIPAddressWithSubnet adds an IP address to the database and returns its UUID.
func (s *commitHookBaseSuite) addIPAddressWithSubnet(c *tc.C, deviceUUID, netNodeUUID,
	subnetUUID, addressValue string) string {
	addressUUID := "address-" + addressValue + "-uuid"

	s.query(c, `
		INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, subnet_uuid, type_id, config_type_id, origin_id, scope_id, is_secondary, is_shadow)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, addressUUID, deviceUUID, addressValue, netNodeUUID, subnetUUID, 0, 4, 1, 0,
		false, false)

	return addressUUID
}

func nilZeroPtr[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
