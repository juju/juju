// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	exportcontroller "github.com/juju/juju/domain/export/state/controller"
	exportmodel "github.com/juju/juju/domain/export/state/model"
	networkstate "github.com/juju/juju/domain/network/state"
	"github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/domain/recovery/service"
	"github.com/juju/juju/domain/recovery/state"
	"github.com/juju/juju/domain/schema"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type controllerSuite struct{ schematesting.ControllerSuite }
type modelSuite struct{ schematesting.ModelSuite }

func TestControllerSuite(t *testing.T) { tc.Run(t, &controllerSuite{}) }
func TestModelSuite(t *testing.T)      { tc.Run(t, &modelSuite{}) }

// Exercise the real export representation, including all unconditionally
// emitted tables and binary strings, through YAML back into SQLair bindings.
func (s *controllerSuite) TestExportDecodeLoadRoundTrip(c *tc.C) {
	key := []byte{0, 128, 255}
	execSQL(c, s.DB(), `INSERT INTO macaroon_root_key VALUES (?, '2026-01-01', '2027-01-01', ?)`, key, key)
	execSQL(c, s.DB(), `INSERT INTO controller_ssh_host_key VALUES ('key', 0, 'ssh-key', ?)`, key)
	execSQL(c, s.DB(), `INSERT INTO controller_ssh_server_port VALUES (8022)`)
	seedBlob(c, s.DB(), "0")
	execSQL(c, s.DB(), `INSERT INTO controller_node_password (controller_id, password_hash) VALUES ('0', 'source')`)
	exporter := exportcontroller.NewState(s.TxnRunnerFactory())
	before, err := exporter.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	data, err := yaml.Marshal(map[string]any{"version": "4.1.0", "payload": before})
	c.Assert(err, tc.ErrorIsNil)
	dump, err := service.DecodeControllerDump(data)
	c.Assert(err, tc.ErrorIsNil)
	execSQL(c, s.DB(), `DELETE FROM macaroon_root_key`)
	execSQL(c, s.DB(), `DELETE FROM controller_ssh_server_port`)
	execSQL(c, s.DB(), `UPDATE controller_node_password SET password_hash = 'target'`)
	// Direct callers cannot override the exclusion policy through Present.
	dump.Present["controller_node_password"] = true
	dump.Present["schema"] = true
	c.Assert(state.NewState(s.TxnRunnerFactory()).LoadControllerDump(c.Context(), dump, "0"), tc.ErrorIsNil)
	after, err := exporter.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(after.MacaroonRootKey, tc.DeepEquals, before.MacaroonRootKey)
	c.Check(after.ControllerSshHostKey, tc.DeepEquals, before.ControllerSshHostKey)
	c.Check(after.ControllerSshServerPort, tc.DeepEquals, before.ControllerSshServerPort)
	c.Check(after.Schema, tc.DeepEquals, before.Schema)
	c.Check(after.ControllerNode, tc.DeepEquals, before.ControllerNode)
	c.Check(after.ObjectStoreMetadata, tc.DeepEquals, before.ObjectStoreMetadata)
	c.Check(after.ObjectStorePlacement, tc.DeepEquals, before.ObjectStorePlacement)
	checkString(c, s.DB(), `SELECT password_hash FROM controller_node_password WHERE controller_id = '0'`, "target")
	checkPlacement(c, s.DB(), "0")
	// Consumers bind the key ID as bytes, so restoring it as TEXT would make
	// the lookup fail even though a second export looks identical.
	var restored []byte
	err = s.DB().QueryRowContext(c.Context(), `SELECT root_key FROM macaroon_root_key WHERE id = ?`, key).Scan(&restored)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(restored, tc.DeepEquals, key)
	checkString(c, s.DB(), `SELECT typeof(root_key) FROM macaroon_root_key`, "blob")
	checkString(c, s.DB(), `SELECT typeof(public_key) FROM controller_ssh_host_key`, "blob")
}

func (s *modelSuite) TestExportDecodeLoadRoundTrip(c *tc.C) {
	seedBlob(c, s.DB(), "source")
	execSQL(c, s.DB(), `INSERT INTO sequence (namespace, value) VALUES ('machine', 7)`)
	execSQL(c, s.DB(), `INSERT INTO space (uuid, name) VALUES ('space', 'restored')`)
	execSQL(c, s.DB(), `INSERT INTO provider_space (provider_id, space_uuid) VALUES ('provider', 'space')`)
	execSQL(c, s.DB(), `INSERT INTO net_node (uuid) VALUES ('node1'), ('node2'), ('node3')`)
	execSQL(c, s.DB(), `INSERT INTO machine (uuid, name, net_node_uuid, life_id, keep_instance)
VALUES ('machine1', '0', 'node1', 0, NULL),
       ('machine2', '1', 'node2', 0, FALSE),
       ('machine3', '2', 'node3', 0, TRUE)`)
	updated := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	execSQL(c, s.DB(), `UPDATE machine SET agent_started_at = ? WHERE uuid = 'machine3'`, updated)
	execSQL(c, s.DB(), `INSERT INTO machine_status (machine_uuid, status_id, data, updated_at)
VALUES ('machine1', 0, NULL, NULL), ('machine2', 1, ?, NULL), ('machine3', 2, ?, ?)`,
		[]byte{}, []byte{0, 128, 255}, updated)
	exporter := exportmodel.NewState(s.TxnRunnerFactory())
	before, err := exporter.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	data, err := yaml.Marshal(map[string]any{"version": "4.1.0", "payload": before})
	c.Assert(err, tc.ErrorIsNil)
	dump, err := service.DecodeModelDump(data)
	c.Assert(err, tc.ErrorIsNil)
	execSQL(c, s.DB(), `DELETE FROM sequence`)
	// No placement exists on a fresh model database. The caller supplies its
	// target explicitly, without needing a pre-existing blob to infer it.
	execSQL(c, s.DB(), `DELETE FROM object_store_placement`)
	dump.Present["object_store_placement"] = true
	dump.Present["schema"] = true
	c.Assert(state.NewState(s.TxnRunnerFactory()).LoadModelDump(c.Context(), dump, "replacement"), tc.ErrorIsNil)
	after, err := exporter.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(after.Sequence, tc.DeepEquals, before.Sequence)
	c.Check(after.Schema, tc.DeepEquals, before.Schema)
	c.Check(after.Space, tc.SameContents, before.Space)
	c.Check(after.ProviderSpace, tc.DeepEquals, before.ProviderSpace)
	c.Check(after.ObjectStoreMetadata, tc.DeepEquals, before.ObjectStoreMetadata)
	c.Check(after.Machine, tc.SameContents, before.Machine)
	c.Check(after.MachineStatus, tc.SameContents, before.MachineStatus)
	checkString(c, s.DB(), `SELECT typeof(data) FROM machine_status WHERE machine_uuid = 'machine1'`, "null")
	checkString(c, s.DB(), `SELECT typeof(data) FROM machine_status WHERE machine_uuid = 'machine2'`, "blob")
	checkString(c, s.DB(), `SELECT typeof(data) FROM machine_status WHERE machine_uuid = 'machine3'`, "blob")
	checkPlacement(c, s.DB(), "replacement")
}

func (s *controllerSuite) TestExplicitNullAndBooleanValues(c *tc.C) {
	dump, err := service.DecodeControllerDump([]byte(`payload:
  flag:
    - {name: explicit-null, value: null, description: null-value}
    - {name: explicit-false, value: false, description: false-value}
    - {name: explicit-true, value: true, description: true-value}
`))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(state.NewState(s.TxnRunnerFactory()).LoadControllerDump(c.Context(), dump, "target"), tc.ErrorIsNil)
	for _, test := range []struct {
		name string
		want sql.NullBool
	}{
		{"explicit-null", sql.NullBool{}},
		{"explicit-false", sql.NullBool{Valid: true}},
		{"explicit-true", sql.NullBool{Bool: true, Valid: true}},
	} {
		// Dqlite maps a NULL BOOLEAN to false on reads. Query nullness
		// explicitly to distinguish storage from that driver conversion.
		var got, isNull bool
		err := s.DB().QueryRowContext(c.Context(), `SELECT value IS NULL, COALESCE(value, FALSE) FROM flag WHERE name = ?`, test.name).Scan(&isNull, &got)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(isNull, tc.Equals, !test.want.Valid)
		c.Check(got, tc.Equals, test.want.Bool)
	}
}

func (s *controllerSuite) TestBinaryYAMLRoundTrip(c *tc.C) {
	for _, value := range []string{"!!binary AID/", "[0, 128, 255]", "plain"} {
		data := "payload:\n  macaroon_root_key:\n    - id: key\n      created_at: 2026-01-01T00:00:00Z\n      expires_at: 2027-01-01T00:00:00Z\n      root_key: " + value
		dump, err := service.DecodeControllerDump([]byte(data))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(state.NewState(s.TxnRunnerFactory()).LoadControllerDump(c.Context(), dump, "target"), tc.ErrorIsNil)
		var got []byte
		err = s.DB().QueryRowContext(c.Context(), `SELECT root_key FROM macaroon_root_key`).Scan(&got)
		c.Assert(err, tc.ErrorIsNil)
		want := []byte{0, 128, 255}
		if value == "plain" {
			want = []byte("plain")
		}
		c.Check(got, tc.DeepEquals, want)
	}
}

func (s *modelSuite) TestForeignKeyFailureRollsBack(c *tc.C) {
	s.DB().SetMaxOpenConns(1)
	seedBlob(c, s.DB(), "original")
	execSQL(c, s.DB(), `INSERT INTO sequence VALUES ('original', 1)`)
	dump, err := service.DecodeModelDump([]byte(`payload:
  sequence: [{namespace: replacement, value: 2}]
  provider_space: [{provider_id: broken, space_uuid: missing}]
`))
	c.Assert(err, tc.ErrorIsNil)
	err = state.NewState(s.TxnRunnerFactory()).LoadModelDump(c.Context(), dump, "replacement")
	c.Check(err, tc.ErrorMatches, `loaded database fails foreign key integrity: provider_space row 1 referencing space \(foreign key 0\)`)
	checkString(c, s.DB(), `SELECT namespace FROM sequence`, "original")
	checkPlacement(c, s.DB(), "original")
	// Failed replay must leave FK enforcement enabled on the connection.
	_, err = s.DB().ExecContext(c.Context(), `INSERT INTO provider_space VALUES ('broken', 'missing')`)
	c.Check(err, tc.NotNil)
	checkString(c, s.DB(), `PRAGMA foreign_keys`, "1")
	checkString(c, s.DB(), `PRAGMA defer_foreign_keys`, "0")
}

func (s *modelSuite) TestForeignKeyDeferralResetsAfterCommit(c *tc.C) {
	s.DB().SetMaxOpenConns(1)
	// Generated replay inserts provider_space before its parent space.
	// Source change history is skipped, but target triggers still run.
	dump, err := service.DecodeModelDump([]byte(`payload:
  provider_space: [{provider_id: provider, space_uuid: restored-space}]
  space: [{uuid: restored-space, name: restored}]
  change_log: [{changed: source-history}]
`))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(state.NewState(s.TxnRunnerFactory()).LoadModelDump(c.Context(), dump, "target"), tc.ErrorIsNil)
	checkString(c, s.DB(), `SELECT space_uuid FROM provider_space`, "restored-space")
	checkString(c, s.DB(), `PRAGMA foreign_keys`, "1")
	checkString(c, s.DB(), `PRAGMA defer_foreign_keys`, "0")
	_, err = s.DB().ExecContext(c.Context(), `INSERT INTO provider_space VALUES ('broken', 'missing')`)
	c.Check(err, tc.NotNil)
	checkString(c, s.DB(), `SELECT COUNT(*) FROM change_log WHERE changed = 'source-history'`, "0")
	checkString(c, s.DB(), `
SELECT COUNT(*)
FROM change_log AS cl
JOIN change_log_namespace AS cln ON cl.namespace_id = cln.id
WHERE cln.namespace = 'space' AND cl.changed = 'restored-space'
AND cl.edit_type_id = 1`, "1")
}

func (s *modelSuite) TestForeignKeyDiagnosticsAreBounded(c *tc.C) {
	var rows []map[string]string
	for i := range 12 {
		rows = append(rows, map[string]string{
			"provider_id": fmt.Sprintf("provider-%d", i), "space_uuid": fmt.Sprintf("missing-%d", i),
		})
	}
	data, err := yaml.Marshal(map[string]any{"payload": map[string]any{"provider_space": rows}})
	c.Assert(err, tc.ErrorIsNil)
	dump, err := service.DecodeModelDump(data)
	c.Assert(err, tc.ErrorIsNil)
	err = state.NewState(s.TxnRunnerFactory()).LoadModelDump(c.Context(), dump, "target")
	c.Assert(err, tc.NotNil)
	c.Check(strings.Count(err.Error(), "provider_space row"), tc.Equals, 10)
	c.Check(strings.HasSuffix(err.Error(), "; and more"), tc.IsTrue)
	checkString(c, s.DB(), `SELECT COUNT(*) FROM provider_space`, "0")
}

func (s *controllerSuite) TestInsertFailureRollsBack(c *tc.C) {
	execSQL(c, s.DB(), `INSERT INTO flag VALUES ('original', 1, 'original')`)
	dump, err := service.DecodeControllerDump([]byte(`payload:
  flag:
    - {name: duplicate, value: false, description: first}
    - {name: duplicate, value: true, description: second}
`))
	c.Assert(err, tc.ErrorIsNil)
	err = state.NewState(s.TxnRunnerFactory()).LoadControllerDump(c.Context(), dump, "target")
	c.Check(err, tc.ErrorMatches, `(?s)inserting table "flag":.*UNIQUE constraint failed: flag.name.*`)
	checkString(c, s.DB(), `SELECT name FROM flag`, "original")
}

func (s *controllerSuite) TestMissingNodeRejectedBeforeWrites(c *tc.C) {
	execSQL(c, s.DB(), `INSERT INTO flag VALUES ('original', 1, 'original')`)
	dump, err := service.DecodeControllerDump([]byte(`payload: {flag: []}`))
	c.Assert(err, tc.ErrorIsNil)
	err = state.NewState(s.TxnRunnerFactory()).LoadControllerDump(c.Context(), dump, "")
	c.Check(err, tc.ErrorMatches, "target object store node ID is required")
	checkString(c, s.DB(), `SELECT name FROM flag`, "original")
}

func (s *controllerSuite) TestNullBinaryDoesNotUseDefault(c *tc.C) {
	execSQL(c, s.DB(), `INSERT INTO controller_ssh_host_key VALUES ('original', 0, 'key', ?)`, []byte{1})
	dump, err := service.DecodeControllerDump([]byte(`payload:
  controller_ssh_host_key:
    - {id: replacement, algorithm_type_id: 0, ssh_key: key, public_key: null}
`))
	c.Assert(err, tc.ErrorIsNil)
	err = state.NewState(s.TxnRunnerFactory()).LoadControllerDump(c.Context(), dump, "target")
	c.Check(err, tc.ErrorMatches, `(?s)inserting table "controller_ssh_host_key":.*NOT NULL constraint failed: controller_ssh_host_key.public_key.*`)
	checkString(c, s.DB(), `SELECT id FROM controller_ssh_host_key`, "original")
}

func (s *modelSuite) TestMissingNodeRejected(c *tc.C) {
	err := state.NewState(s.TxnRunnerFactory()).LoadModelDump(c.Context(), &recovery.ModelDump{}, "")
	c.Check(err, tc.ErrorMatches, "target object store node ID is required")
}

func (s *modelSuite) TestCancellation(c *tc.C) {
	execSQL(c, s.DB(), `INSERT INTO sequence VALUES ('original', 1)`)
	dump, err := service.DecodeModelDump([]byte(`payload: {sequence: []}`))
	c.Assert(err, tc.ErrorIsNil)
	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	err = state.NewState(s.TxnRunnerFactory()).LoadModelDump(ctx, dump, "target")
	c.Check(err, tc.ErrorIs, context.Canceled)
	checkString(c, s.DB(), `SELECT namespace FROM sequence`, "original")
}

func (s *modelSuite) TestAbsentAndEmptyTables(c *tc.C) {
	for _, value := range []string{"null", "[]"} {
		execSQL(c, s.DB(), `INSERT INTO sequence VALUES ('original', 1)`)
		dump, err := service.DecodeModelDump([]byte("payload: {sequence: " + value + "}"))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(state.NewState(s.TxnRunnerFactory()).LoadModelDump(c.Context(), dump, "target"), tc.ErrorIsNil)
		var count int
		err = s.DB().QueryRowContext(c.Context(), `SELECT COUNT(*) FROM sequence`).Scan(&count)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(count, tc.Equals, 0)
		checkString(c, s.DB(), `SELECT name FROM space`, "alpha")
	}
}

func seedBlob(c *tc.C, db *sql.DB, node string) {
	execSQL(c, db, `INSERT INTO object_store_metadata VALUES ('blob', 'sha256', 'sha384', 3)`)
	execSQL(c, db, `INSERT INTO object_store_placement VALUES ('blob', ?)`, node)
}

func checkPlacement(c *tc.C, db *sql.DB, node string) {
	checkString(c, db, `SELECT node_id FROM object_store_placement WHERE uuid = 'blob'`, node)
	var count int
	c.Assert(db.QueryRowContext(c.Context(), `SELECT COUNT(*) FROM object_store_placement`).Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

func checkString(c *tc.C, db *sql.DB, query, want string) {
	var got string
	c.Assert(db.QueryRowContext(c.Context(), query).Scan(&got), tc.ErrorIsNil)
	c.Check(got, tc.Equals, want)
}

func execSQL(c *tc.C, db *sql.DB, query string, args ...any) {
	_, err := db.ExecContext(c.Context(), query, args...)
	c.Assert(err, tc.ErrorIsNil)
}

type loadSuite struct {
	schematesting.ControllerModelSuite
}

func TestLoadSuite(t *testing.T) {
	tc.Run(t, &loadSuite{})
}

const (
	targetControllerUUID = "11111111-0000-0000-0000-000000000000"
	sourceControllerUUID = "22222222-0000-0000-0000-000000000000"
	sourceModelUUID      = "33333333-0000-0000-0000-000000000000"
	sourceCloudUUID      = "44444444-0000-0000-0000-000000000000"
	targetBlobUUID       = "55555555-0000-0000-0000-000000000000"
	sourceBlobUUID       = "66666666-0000-0000-0000-000000000000"
)

// seedTargetController simulates what bootstrap wrote before the recovery
// load runs: the controller row, and one object blob with its placement.
func (s *loadSuite) seedTargetController(c *tc.C) {
	_, err := s.DB().Exec(
		"INSERT INTO controller (uuid, model_uuid, target_version) VALUES (?, ?, ?)",
		targetControllerUUID, "00000000-0000-0000-0000-000000000000", "4.1.0")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().Exec(
		"INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size) VALUES (?, ?, ?, ?)",
		targetBlobUUID, "aa", "bb", 2)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().Exec(
		"INSERT INTO object_store_placement (uuid, node_id) VALUES (?, ?)",
		targetBlobUUID, "bootstrap-node")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *loadSuite) controllerDump() *testDump {
	return &testDump{
		Version: "4.1.0",
		Tables: map[string][]testRow{
			"controller": {{
				"uuid":            sourceControllerUUID,
				"model_uuid":      sourceModelUUID,
				"target_version":  "4.1.0",
				"api_port":        "17070",
				"cert":            nil,
				"ca_cert":         "source-ca",
				"private_key":     nil,
				"ca_private_key":  nil,
				"system_identity": nil,
			}},
			"cloud": {{
				"uuid":              sourceCloudUUID,
				"name":              "lxd",
				"cloud_type_id":     int64(1),
				"endpoint":          "",
				"identity_endpoint": nil,
				"storage_endpoint":  nil,
				"skip_tls_verify":   false,
			}},
			"model": {{
				"uuid":                  sourceModelUUID,
				"activated":             true,
				"cloud_uuid":            sourceCloudUUID,
				"cloud_region_uuid":     nil,
				"cloud_credential_uuid": nil,
				"model_type_id":         int64(0),
				"life_id":               int64(0),
				"name":                  "controller",
				"qualifier":             "",
			}},
			"controller_config": {{
				"key":   "controller-name",
				"value": "source-ctrl",
			}},
			// Binary column values appear in dumps in two shapes:
			// []byte-typed export fields marshal as sequences of
			// integers, and string-typed fields holding binary marshal
			// as !!binary, which the YAML decoder returns as a string
			// of raw bytes. Both load back as BLOB storage.
			"controller_ssh_host_key": {{
				"id":                "host-key-uuid",
				"algorithm_type_id": int64(2),
				"ssh_key":           "-----BEGIN OPENSSH PRIVATE KEY-----",
				"public_key":        []any{int(0), int(0), int(0), int(11), int(115)},
			}},
			"bakery_config": {{
				"local_users_private_key":                "\x00\xff\x02",
				"local_users_public_key":                 "\x00\xff\x02",
				"local_users_third_party_private_key":    "\x00\xff\x02",
				"local_users_third_party_public_key":     "\x00\xff\x02",
				"external_users_third_party_private_key": "\x00\xff\x02",
				"external_users_third_party_public_key":  "\x00\xff\x02",
				"offers_third_party_private_key":         "\x00\xff\x02",
				"offers_third_party_public_key":          "\x00\xff\x02",
			}},
			"object_store_metadata": {{
				"uuid":    sourceBlobUUID,
				"sha_256": "cc",
				"sha_384": "dd",
				"size":    int64(2),
			}},
			"object_store_placement": {{
				"uuid":    sourceBlobUUID,
				"node_id": "source-node-5",
			}},
			// Lookup tables are replaced: the archive carries every
			// seeded row (idempotent under the exact-version gate).
			"life": {
				{"id": int64(0), "value": "alive"},
				{"id": int64(1), "value": "dying"},
				{"id": int64(2), "value": "dead"},
			},
			"controller_node": {{
				"controller_id":       "99",
				"life_id":             int64(0),
				"dqlite_node_id":      int64(1234),
				"dqlite_bind_address": "10.0.0.9",
			}},
		},
	}
}

func (s *loadSuite) TestLoadControllerDump(c *tc.C) {
	s.seedTargetController(c)

	err := s.recoveryService(c).LoadControllerDump(c.Context(), dumpYAML(c, s.controllerDump()))
	c.Assert(err, tc.ErrorIsNil)

	// The controller row carries the source identity.
	var uuid, caCert string
	err = s.DB().QueryRow("SELECT uuid, ca_cert FROM controller").Scan(&uuid, &caCert)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, sourceControllerUUID)
	c.Check(caCert, tc.Equals, "source-ca")

	// The binary column survived as BLOB storage with its raw bytes.
	var publicKey []byte
	err = s.DB().QueryRow(
		"SELECT public_key FROM controller_ssh_host_key WHERE id = ?",
		"host-key-uuid").Scan(&publicKey)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(publicKey, tc.DeepEquals, []byte{0, 0, 0, 11, 115})

	// The !!binary-decoded string fields load as BLOB storage, ready
	// for byte-expecting scanners like the bakery key scanner.
	var keyClass string
	err = s.DB().QueryRow(
		"SELECT typeof(local_users_third_party_private_key) FROM bakery_config",
	).Scan(&keyClass)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keyClass, tc.Equals, "blob")

	// Cloud and model rows are recovered.
	var cloudName string
	err = s.DB().QueryRow("SELECT name FROM cloud WHERE uuid = ?", sourceCloudUUID).Scan(&cloudName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cloudName, tc.Equals, "lxd")

	// Lookup rows are replaced: the dump carries the full set
	// (idempotent under the exact-version gate).
	var lifeRows int
	err = s.DB().QueryRow("SELECT COUNT(*) FROM life").Scan(&lifeRows)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeRows, tc.Equals, 3)

	// The replacement's controller node row is untouched.
	var nodeID int64
	err = s.DB().QueryRow(
		"SELECT dqlite_node_id FROM controller_node").Scan(&nodeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nodeID, tc.Equals, int64(schematesting.DqliteNodeID))

	// Placements are rebuilt onto the replacement's node for every
	// surviving metadata row. The fresh bootstrap blob row is replaced
	// by the archived one and its placement is rebuilt; an orphaned
	// on-disk file is handled by LoadObjects later.
	rows, err := s.DB().Query("SELECT uuid, node_id FROM object_store_placement ORDER BY uuid")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()
	placements := map[string]string{}
	for rows.Next() {
		var u, n string
		c.Assert(rows.Scan(&u, &n), tc.ErrorIsNil)
		placements[u] = n
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Check(placements, tc.DeepEquals, map[string]string{
		sourceBlobUUID: "bootstrap-node",
	})
}

func (s *loadSuite) TestLoadControllerDumpForeignKeyViolation(c *tc.C) {
	dump := s.controllerDump()
	dump.Tables["model"] = []testRow{{
		"uuid":                  sourceModelUUID,
		"activated":             true,
		"cloud_uuid":            "99999999-0000-0000-0000-000000000000",
		"cloud_region_uuid":     nil,
		"cloud_credential_uuid": nil,
		"model_type_id":         int64(0),
		"life_id":               int64(0),
		"name":                  "broken",
		"qualifier":             "",
	}}
	delete(dump.Tables, "cloud")

	err := s.recoveryService(c).LoadControllerDump(c.Context(), dumpYAML(c, dump))
	c.Assert(err, tc.ErrorMatches, ".*fails foreign key integrity.*")

	// The failed load rolled back: no model row remains.
	var count int
	err = s.DB().QueryRow("SELECT COUNT(*) FROM model").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *loadSuite) openModelDB(c *tc.C, modelUUID string) *sql.DB {
	runner, db := s.OpenDBForNamespace(c, modelUUID, true)
	s.DqliteSuite.ApplyDDLForRunner(c, &schematesting.SchemaApplier{
		Schema: schema.ModelDDL(),
	}, runner)
	return db
}

func (s *loadSuite) modelDump() *testDump {
	return &testDump{
		Version: "4.1.0",
		Tables: map[string][]testRow{
			"net_node": {{"uuid": "nn-0"}},
			"machine": {{
				"uuid":                       "machine-uuid-0",
				"name":                       "0",
				"net_node_uuid":              "nn-0",
				"life_id":                    int64(0),
				"nonce":                      nil,
				"password_hash_algorithm_id": nil,
				"password_hash":              nil,
				"force_destroyed":            nil,
				"agent_started_at":           nil,
				"hostname":                   nil,
				"keep_instance":              nil,
			}},
			"machine_cloud_instance": {{
				"machine_uuid":           "machine-uuid-0",
				"life_id":                int64(0),
				"instance_id":            "source-instance",
				"display_name":           "source-display",
				"arch":                   nil,
				"availability_zone_uuid": nil,
				"cpu_cores":              nil,
				"cpu_power":              nil,
				"mem":                    nil,
				"root_disk":              nil,
				"root_disk_source":       nil,
				"virt_type":              nil,
			}},
			// Spaces are replaced: the archive carries every row,
			// including the deterministic alpha seed.
			"space": {
				{"uuid": "656b4a82-e28c-53d6-a014-f0dd53417eb6", "name": "alpha"},
				{"uuid": "77777777-0000-0000-0000-000000000000", "name": "dmz"},
			},
		},
	}
}

func (s *loadSuite) TestLoadModelDump(c *tc.C) {
	db := s.openModelDB(c, sourceModelUUID)
	defer db.Close()

	err := s.recoveryService(c).LoadModelDump(c.Context(), coremodel.UUID(sourceModelUUID), dumpYAML(c, s.modelDump()))
	c.Assert(err, tc.ErrorIsNil)

	var machineName string
	err = db.QueryRow("SELECT name FROM machine WHERE uuid = ?", "machine-uuid-0").Scan(&machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineName, tc.Equals, "0")

	// Spaces are replaced: the archive includes the deterministic alpha
	// seed and the user-created space.
	var spaceCount int
	err = db.QueryRow("SELECT COUNT(*) FROM space").Scan(&spaceCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(spaceCount, tc.Equals, 2)
}

func (s *loadSuite) TestPatchControllerMachine(c *tc.C) {
	db := s.openModelDB(c, sourceModelUUID)
	defer db.Close()
	c.Assert(s.recoveryService(c).LoadModelDump(c.Context(), coremodel.UUID(sourceModelUUID), dumpYAML(c, s.modelDump())), tc.ErrorIsNil)

	err := s.modelState(c).PatchControllerMachine(c.Context(), recovery.MachinePatch{
		MachineName: "0",
		InstanceID:  "replacement-instance",
		DisplayName: "replacement-display",
		Arch:        "amd64",
		MemMB:       8192,
	})
	c.Assert(err, tc.ErrorIsNil)

	var instanceID, displayName, arch string
	var mem int64
	err = db.QueryRow(
		"SELECT instance_id, display_name, arch, mem FROM machine_cloud_instance WHERE machine_uuid = ?",
		"machine-uuid-0").Scan(&instanceID, &displayName, &arch, &mem)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceID, tc.Equals, "replacement-instance")
	c.Check(displayName, tc.Equals, "replacement-display")
	c.Check(arch, tc.Equals, "amd64")
	c.Check(mem, tc.Equals, int64(8192))
}

func (s *loadSuite) TestPatchControllerMachineMissing(c *tc.C) {
	db := s.openModelDB(c, sourceModelUUID)
	defer db.Close()
	c.Assert(s.recoveryService(c).LoadModelDump(c.Context(), coremodel.UUID(sourceModelUUID), dumpYAML(c, s.modelDump())), tc.ErrorIsNil)

	err := s.modelState(c).PatchControllerMachine(c.Context(), recovery.MachinePatch{
		MachineName: "9",
		InstanceID:  "x",
	})
	c.Assert(err, tc.ErrorMatches, `archived controller machine not found`)
}

func (s *loadSuite) TestLoadControllerDumpRejectsUnknownVersion(c *tc.C) {
	s.seedTargetController(c)

	dump := s.controllerDump()
	dump.Version = "3.6.0"
	err := s.recoveryService(c).LoadControllerDump(c.Context(), dumpYAML(c, dump))
	c.Assert(err, tc.ErrorMatches, "controller dump: unsupported dump format version \"3.6.0\".*")

	dump.Version = ""
	err = s.recoveryService(c).LoadControllerDump(c.Context(), dumpYAML(c, dump))
	c.Assert(err, tc.ErrorMatches, "controller dump: dump records no format version")
}

func (s *loadSuite) TestLoadControllerExcludesSourceLeases(c *tc.C) {
	s.seedTargetController(c)
	dump := s.controllerDump()
	dump.Tables["lease_type"] = []testRow{
		{"id": 0, "type": "singular-controller"},
		{"id": 1, "type": "application-leadership"},
		{"id": 2, "type": "backup-creation"},
	}
	dump.Tables["lease"] = []testRow{
		{"uuid": "backup", "lease_type_id": 2, "holder": "request", "model_uuid": sourceModelUUID},
		{"uuid": "leadership", "lease_type_id": 1, "holder": "app/0", "model_uuid": sourceModelUUID},
	}
	dump.Tables["lease_pin"] = []testRow{
		{"uuid": "backup-pin", "lease_uuid": "backup", "entity_id": "request"},
		{"uuid": "leader-pin", "lease_uuid": "leadership", "entity_id": "unit"},
	}
	err := s.recoveryService(c).LoadControllerDump(c.Context(), dumpYAML(c, dump))
	c.Assert(err, tc.ErrorIsNil)
	// Replay policy never restores source leases or their pins: ownership
	// held by the lost controller can never block backups on the replacement.
	var leases int
	err = s.DB().QueryRowContext(c.Context(), "SELECT COUNT(*) FROM lease").Scan(&leases)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leases, tc.Equals, 0)
	var pins int
	err = s.DB().QueryRowContext(c.Context(), "SELECT COUNT(*) FROM lease_pin").Scan(&pins)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pins, tc.Equals, 0)
	// Lease types are lookup rows and still import, so leases freshly
	// written by the replacement resolve their types.
	var leaseType string
	err = s.DB().QueryRowContext(c.Context(), "SELECT type FROM lease_type WHERE id = 2").Scan(&leaseType)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leaseType, tc.Equals, "backup-creation")
}

func (s *loadSuite) TestLoadModelDumpRejectsUnknownVersion(c *tc.C) {
	db := s.openModelDB(c, sourceModelUUID)
	defer db.Close()

	dump := s.modelDump()
	dump.Version = "4.2.0"
	err := s.recoveryService(c).LoadModelDump(c.Context(), coremodel.UUID(sourceModelUUID), dumpYAML(c, dump))
	c.Assert(err, tc.ErrorMatches, "model dump: unsupported dump format version \"4.2.0\".*")
}

// seedControllerUnitApplication seeds the model tables a controller
// application and unit need, including one observed ip_address row.
func (s *loadSuite) seedControllerUnitApplication(c *tc.C, db *sql.DB) {
	insert := func(query string, args ...any) {
		_, err := db.Exec(query, args...)
		c.Assert(err, tc.ErrorIsNil)
	}
	insert("INSERT INTO net_node (uuid) VALUES (?)", "nn-0")
	insert(`INSERT INTO charm (uuid, reference_name, revision, source_id, architecture_id)
		VALUES (?, 'controller', 1, 1, 0)`, "charm-uuid-0")
	insert(`INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid)
		VALUES (?, 'controller', 0, 'charm-uuid-0', '656b4a82-e28c-53d6-a014-f0dd53417eb6')`, "app-uuid-0")
	insert("INSERT INTO application_controller (application_uuid) VALUES (?)", "app-uuid-0")
	insert(`INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid)
		VALUES (?, 'controller/0', 0, 'app-uuid-0', 'nn-0', 'charm-uuid-0')`, "unit-uuid-0")
	insert(`INSERT INTO link_layer_device (uuid, net_node_uuid, name, device_type_id, virtual_port_type_id)
		VALUES (?, 'nn-0', 'eth0', 2, 0)`, "lld-uuid-0")
	insert(`INSERT INTO ip_address (uuid, net_node_uuid, device_uuid, address_value, type_id, config_type_id, origin_id, scope_id)
		VALUES (?, 'nn-0', 'lld-uuid-0', '10.0.0.5/24', 0, 0, 0, 0)`, "ip-uuid-0")
}

func (s *loadSuite) TestPatchControllerUnitAddressesInsertsMissing(c *tc.C) {
	db := s.openModelDB(c, sourceModelUUID)
	defer db.Close()
	s.seedControllerUnitApplication(c, db)

	// The replacement exposes two addresses but the archive recorded
	// one: the old observation is replaced and the device is retained.
	patch, err := service.NewControllerUnitAddressPatch([]string{"10.1.0.1", "10.1.0.2"})
	c.Assert(err, tc.ErrorIsNil)
	err = s.modelState(c).PatchControllerUnitAddresses(c.Context(), patch)
	c.Assert(err, tc.ErrorIsNil)

	type addrRow struct {
		uuid, addressValue, deviceUUID string
	}
	var rows []addrRow
	result, err := db.Query(
		"SELECT uuid, address_value, device_uuid FROM ip_address WHERE net_node_uuid = 'nn-0' ORDER BY address_value")
	c.Assert(err, tc.ErrorIsNil)
	defer result.Close()
	for result.Next() {
		var row addrRow
		c.Assert(result.Scan(&row.uuid, &row.addressValue, &row.deviceUUID), tc.ErrorIsNil)
		rows = append(rows, row)
	}
	c.Assert(result.Err(), tc.ErrorIsNil)
	c.Assert(rows, tc.HasLen, 2)
	c.Check(rows[0].addressValue, tc.Equals, "10.1.0.1")
	c.Check(rows[1].addressValue, tc.Equals, "10.1.0.2")
	c.Check(rows[1].uuid, tc.Not(tc.Equals), "ip-uuid-0")
	c.Check(rows[1].deviceUUID, tc.Equals, "lld-uuid-0")
}

func (s *loadSuite) TestPatchControllerUnitAddressesRepresentations(c *tc.C) {
	db := s.openModelDB(c, sourceModelUUID)
	defer db.Close()
	s.seedControllerUnitApplication(c, db)
	insert := func(query string, args ...any) {
		_, err := db.ExecContext(c.Context(), query, args...)
		c.Assert(err, tc.ErrorIsNil)
	}
	insert(`INSERT INTO model
		(uuid, controller_uuid, name, qualifier, type, cloud, cloud_type, is_controller_model)
		VALUES (?, ?, 'controller', 'admin', 'caas', 'test', 'kubernetes', true)`,
		sourceModelUUID, sourceControllerUUID)
	insert("INSERT INTO net_node (uuid) VALUES ('service-node')")
	insert(`INSERT INTO k8s_service (uuid, application_uuid, net_node_uuid, provider_id)
		VALUES ('service', 'app-uuid-0', 'service-node', 'source-service')`)
	insert(`INSERT INTO fqdn_address (uuid, address, scope_id) VALUES
		('shared', 'shared.example.com', 1), ('old', 'old.example.com', 1)`)
	insert(`INSERT INTO net_node_fqdn_address (net_node_uuid, address_uuid) VALUES
		('nn-0', 'shared'), ('service-node', 'shared'), ('nn-0', 'old')`)
	runner := s.ModelTxnRunner(c, sourceModelUUID)
	consumer := networkstate.NewState(func(context.Context) (database.TxnRunner, error) {
		return runner, nil
	}, loggertesting.WrapCheckLog(c))
	for _, values := range [][]string{
		{"pod.example.com", "10.1.0.1", "2001:db8::1", "pod.example.com"},
		{"next.example.com"},
		{"10.1.0.2", "2001:db8::2"},
	} {
		patch, err := service.NewControllerUnitAddressPatch(values)
		c.Assert(err, tc.ErrorIsNil)
		err = s.modelState(c).PatchControllerUnitAddresses(c.Context(), patch)
		c.Assert(err, tc.ErrorIsNil)
		observed, err := consumer.GetControllerUnitNetwork(c.Context(), "controller/0")
		c.Assert(err, tc.ErrorIsNil)
		actual, expected := make(map[string]network.AddressType), make(map[string]network.AddressType)
		for _, address := range observed {
			actual[address.Value] = address.Type
			c.Check(address.Scope, tc.Equals, network.ScopeCloudLocal)
		}
		for _, value := range values {
			expected[value] = network.DeriveAddressType(value)
		}
		c.Check(actual, tc.DeepEquals, expected)
		service, err := consumer.GetControllerServiceAddresses(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(service, tc.HasLen, 1)
		c.Check(service[0].Value, tc.Equals, "shared.example.com")
	}
	var stale int
	err := db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM fqdn_address WHERE address IN ('old.example.com', 'pod.example.com', 'next.example.com')").Scan(&stale)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(stale, tc.Equals, 0)
}

func (s *loadSuite) TestPatchControllerUnitAddressesWithoutArchivedDevice(c *tc.C) {
	db := s.openModelDB(c, sourceModelUUID)
	defer db.Close()
	s.seedControllerUnitApplication(c, db)
	_, err := db.ExecContext(c.Context(), "DELETE FROM ip_address")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(), "DELETE FROM link_layer_device")
	c.Assert(err, tc.ErrorIsNil)
	patch, err := service.NewControllerUnitAddressPatch([]string{"pod.example.com"})
	c.Assert(err, tc.ErrorIsNil)
	err = s.modelState(c).PatchControllerUnitAddresses(c.Context(), patch)
	c.Assert(err, tc.ErrorIsNil)
	patch, err = service.NewControllerUnitAddressPatch([]string{"10.1.0.1"})
	c.Assert(err, tc.ErrorIsNil)
	err = s.modelState(c).PatchControllerUnitAddresses(c.Context(), patch)
	c.Assert(err, tc.ErrorIsNil)
	var deviceUUID string
	err = db.QueryRowContext(c.Context(),
		"SELECT device_uuid FROM ip_address WHERE net_node_uuid = 'nn-0'").Scan(&deviceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(deviceUUID, tc.Equals, patch.DeviceUUID)
	patch.Addresses[0].UUID = ""
	patch.Addresses = append(patch.Addresses, patch.Addresses[0])
	err = s.modelState(c).PatchControllerUnitAddresses(c.Context(), patch)
	c.Assert(err, tc.ErrorMatches, ".*UNIQUE constraint failed.*")
	// A failed replacement retains the previously committed observations.
	err = db.QueryRowContext(c.Context(), "SELECT device_uuid FROM ip_address").Scan(&deviceUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(deviceUUID, tc.Equals, patch.DeviceUUID)
}

func (s *loadSuite) TestPatchControllerUnitAddressesMissingUnit(c *tc.C) {
	db := s.openModelDB(c, sourceModelUUID)
	defer db.Close()
	patch, err := service.NewControllerUnitAddressPatch(nil)
	c.Assert(err, tc.ErrorIsNil)
	err = s.modelState(c).PatchControllerUnitAddresses(c.Context(), patch)
	c.Assert(err, tc.ErrorIsNil)
	_, err = service.NewControllerUnitAddressPatch([]string{""})
	c.Assert(err, tc.ErrorMatches, "empty controller unit address")
	patch, err = service.NewControllerUnitAddressPatch([]string{"pod.example.com"})
	c.Assert(err, tc.ErrorIsNil)
	err = s.modelState(c).PatchControllerUnitAddresses(c.Context(), patch)
	c.Assert(err, tc.ErrorMatches, "controller unit not found")
}

func dumpYAML(c *tc.C, dump *testDump) []byte {
	data, err := yaml.Marshal(struct {
		Version string               `yaml:"version"`
		Payload map[string][]testRow `yaml:"payload"`
	}{dump.Version, dump.Tables})
	c.Assert(err, tc.ErrorIsNil)
	return data
}

func (s *loadSuite) modelState(c *tc.C) *state.State {
	runner := s.ModelTxnRunner(c, sourceModelUUID)
	return state.NewState(func(context.Context) (database.TxnRunner, error) { return runner, nil })
}

func (s *loadSuite) recoveryService(c *tc.C) *service.Service {
	svc, err := service.NewService(state.NewState(s.TxnRunnerFactory()),
		func(context.Context, string) (service.State, error) { return s.modelState(c), nil }, "bootstrap-node")
	c.Assert(err, tc.ErrorIsNil)
	return svc
}

// testDump and testRow describe YAML fixtures, not service/state contracts.
type testDump struct {
	Version string
	Tables  map[string][]testRow
}

type testRow map[string]any
