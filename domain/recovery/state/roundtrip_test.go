// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	domainexport "github.com/juju/juju/domain/export"
	controllerexport "github.com/juju/juju/domain/export/state/controller"
	modelexport "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/macaroon"
	macaroonstate "github.com/juju/juju/domain/macaroon/state"
	recoveryservice "github.com/juju/juju/domain/recovery/service"
	recoverystate "github.com/juju/juju/domain/recovery/state"
	"github.com/juju/juju/internal/errors"
)

func (s *loadSuite) TestControllerExportYAMLRoundTripAndKeyConsumers(c *tc.C) {
	s.SeedControllerUUID(c)
	consumer := macaroonstate.NewState(s.TxnRunnerFactory())
	var pair bakery.KeyPair
	for i := range pair.Public.Key {
		pair.Public.Key[i] = 'a'   // Valid UTF-8 must still bind as bytes.
		pair.Private.Key[i] = 0xff // The exporter produces !!binary.
	}
	c.Assert(consumer.InitialiseBakeryConfig(c.Context(), &pair, &pair, &pair, &pair), tc.ErrorIsNil)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	key := macaroon.RootKey{
		ID: []byte("valid-utf8-id"), RootKey: []byte{0, 0xff, 2},
		Created: now.Add(-time.Hour), Expires: now.Add(time.Hour),
	}
	c.Assert(consumer.InsertKey(c.Context(), key), tc.ErrorIsNil)
	payload, err := controllerexport.NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	data, err := yaml.Marshal(domainexport.ControllerExport{Version: domainexport.LatestControllerExportVersion(), Payload: payload})
	c.Assert(err, tc.ErrorIsNil)

	targetRunner, targetDB := s.OpenDB(c)
	s.ApplyDDLForRunner(c, targetRunner)
	_, err = targetDB.Exec("INSERT INTO controller_node_nonce (controller_id, nonce) VALUES ('0', 'replacement-nonce')")
	c.Assert(err, tc.ErrorIsNil)
	factory := func(context.Context) (database.TxnRunner, error) { return targetRunner, nil }
	svc, err := recoveryservice.NewService(recoverystate.NewState(factory), func(context.Context, string) (recoveryservice.State, error) {
		return nil, errors.New("unexpected model open")
	}, "0")
	c.Assert(err, tc.ErrorIsNil)
	// Replaying twice also exercises deletion of existing referenced seeds.
	for range 2 {
		c.Assert(svc.LoadControllerDump(c.Context(), data), tc.ErrorIsNil)
	}
	got, err := controllerexport.NewState(factory).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.Controller, tc.DeepEquals, payload.Controller)
	c.Check(got.Life, tc.DeepEquals, payload.Life)
	c.Check(got.BakeryConfig, tc.DeepEquals, payload.BakeryConfig)
	c.Check(got.MacaroonRootKey, tc.DeepEquals, payload.MacaroonRootKey)
	c.Assert(got.ControllerNodeNonce, tc.HasLen, 1)
	c.Check(got.ControllerNodeNonce[0].Nonce, tc.Equals, "replacement-nonce")

	targetConsumer := macaroonstate.NewState(factory)
	restoredPair, err := targetConsumer.GetLocalUsersKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(restoredPair, tc.DeepEquals, &pair)
	restoredKey, err := targetConsumer.GetKey(c.Context(), key.ID, now)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(restoredKey, tc.DeepEquals, key)
	var idClass, keyClass string
	c.Assert(targetDB.QueryRow("SELECT typeof(id), typeof(root_key) FROM macaroon_root_key").Scan(&idClass, &keyClass), tc.ErrorIsNil)
	c.Check(idClass, tc.Equals, "blob")
	c.Check(keyClass, tc.Equals, "blob")
}

func (s *loadSuite) TestModelExportYAMLRoundTrip(c *tc.C) {
	db := s.openModelDB(c, sourceModelUUID)
	_, err := db.Exec(`INSERT INTO net_node (uuid) VALUES ('node'), ('node2'), ('node3');
INSERT INTO machine (uuid, name, net_node_uuid, life_id, keep_instance)
VALUES ('machine', '0', 'node', 1, NULL),
('machine2', '1', 'node2', 0, TRUE), ('machine3', '2', 'node3', 2, FALSE);
INSERT INTO machine_cloud_instance (machine_uuid, life_id, instance_id, display_name)
VALUES ('machine', 0, 'instance', 'display');
INSERT INTO space (uuid, name) VALUES ('space', 'user-space');
INSERT INTO provider_space (space_uuid, provider_id) VALUES ('space', 'provider-space');`)
	c.Assert(err, tc.ErrorIsNil)
	started := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	_, err = db.Exec("UPDATE machine SET agent_started_at = ? WHERE uuid = 'machine2'", started)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec(`INSERT INTO machine_status
(machine_uuid, status_id, message, data, updated_at) VALUES ('machine', 2, 'pending', ?, ?)`,
		[]byte(`{"attempt":1}`), started)
	c.Assert(err, tc.ErrorIsNil)
	runner := s.ModelTxnRunner(c, sourceModelUUID)
	factory := func(context.Context) (database.TxnRunner, error) { return runner, nil }
	payload, err := modelexport.NewState(factory).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	data, err := yaml.Marshal(domainexport.ModelExport{Version: domainexport.LatestSupportedPayloadVersion(), Payload: payload})
	c.Assert(err, tc.ErrorIsNil)
	// Replaying onto the existing model tests full parent/child replacement.
	svc := s.recoveryService(c)
	c.Assert(svc.LoadModelDump(c.Context(), coremodel.UUID(sourceModelUUID), data), tc.ErrorIsNil)
	got, err := modelexport.NewState(factory).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.Machine, tc.DeepEquals, payload.Machine)
	c.Check(got.MachineCloudInstance, tc.DeepEquals, payload.MachineCloudInstance)
	c.Check(got.MachineStatus, tc.DeepEquals, payload.MachineStatus)
	c.Check(got.ProviderSpace, tc.DeepEquals, payload.ProviderSpace)
	c.Check(got.Space, tc.DeepEquals, payload.Space)
	c.Check(got.Life, tc.DeepEquals, payload.Life)
	var storageClass string
	c.Assert(db.QueryRow("SELECT typeof(data) FROM machine_status").Scan(&storageClass), tc.ErrorIsNil)
	c.Check(storageClass, tc.Equals, "blob")
}

func (s *loadSuite) TestExplicitNullDoesNotUseDefaultAndRollsBack(c *tc.C) {
	s.seedTargetController(c)
	dump := s.controllerDump()
	// This column has a default but is NOT NULL. Explicit NULL must fail
	// rather than fall back to the default for an omitted column.
	dump.Tables["controller_ssh_host_key"][0]["public_key"] = nil
	err := s.recoveryService(c).LoadControllerDump(c.Context(), dumpYAML(c, dump))
	c.Assert(err, tc.ErrorMatches, ".*NOT NULL constraint failed.*")
	var controller string
	c.Assert(s.DB().QueryRow("SELECT uuid FROM controller").Scan(&controller), tc.ErrorIsNil)
	c.Check(controller, tc.Equals, targetControllerUUID)
	var foreignKeys, deferred int
	c.Assert(s.DB().QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys), tc.ErrorIsNil)
	c.Assert(s.DB().QueryRow("PRAGMA defer_foreign_keys").Scan(&deferred), tc.ErrorIsNil)
	c.Check(foreignKeys, tc.Equals, 1)
	c.Check(deferred, tc.Equals, 0)
}

func (s *loadSuite) TestDecodeFailuresDoNotChangeTarget(c *tc.C) {
	s.seedTargetController(c)
	for _, extra := range []struct {
		table string
		rows  []testRow
	}{
		{"unknown_table", nil},
		{"controller_config", []testRow{{"unknown_column": "value"}}},
		{"controller_ssh_host_key", []testRow{{"public_key": []any{256}}}},
	} {
		dump := s.controllerDump()
		dump.Tables[extra.table] = extra.rows
		err := s.recoveryService(c).LoadControllerDump(c.Context(), dumpYAML(c, dump))
		c.Assert(err, tc.NotNil)
		var controller string
		c.Assert(s.DB().QueryRow("SELECT uuid FROM controller").Scan(&controller), tc.ErrorIsNil)
		c.Check(controller, tc.Equals, targetControllerUUID)
	}
}

func (s *loadSuite) TestAbsentAndEmptyTables(c *tc.C) {
	s.seedTargetController(c)
	_, err := s.DB().Exec("INSERT INTO controller_config (key, value) VALUES ('example', 'original')")
	c.Assert(err, tc.ErrorIsNil)
	svc := s.recoveryService(c)
	dump := s.controllerDump()
	delete(dump.Tables, "controller_config")
	c.Assert(svc.LoadControllerDump(c.Context(), dumpYAML(c, dump)), tc.ErrorIsNil)
	var value string
	c.Assert(s.DB().QueryRow("SELECT value FROM controller_config WHERE key = 'example'").Scan(&value), tc.ErrorIsNil)
	c.Check(value, tc.Equals, "original")
	dump.Tables["controller_config"] = nil
	c.Assert(svc.LoadControllerDump(c.Context(), dumpYAML(c, dump)), tc.ErrorIsNil)
	var count int
	c.Assert(s.DB().QueryRow("SELECT COUNT(*) FROM controller_config").Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}
