// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	exportcontroller "github.com/juju/juju/domain/export/state/controller"
	exportmodel "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/domain/recovery/service"
	"github.com/juju/juju/domain/recovery/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
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
	execSQL(c, s.DB(), `UPDATE controller_node_password SET password_hash = 'target'`)
	// Direct callers cannot override the exclusion policy through Present.
	dump.Present["controller_node_password"] = true
	dump.Present["schema"] = true
	c.Assert(state.NewState(s.TxnRunnerFactory()).LoadControllerDump(c.Context(), dump, "0"), tc.ErrorIsNil)
	after, err := exporter.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(after.MacaroonRootKey, tc.DeepEquals, before.MacaroonRootKey)
	c.Check(after.Schema, tc.DeepEquals, before.Schema)
	c.Check(after.ControllerNode, tc.DeepEquals, before.ControllerNode)
	c.Check(after.ObjectStoreMetadata, tc.DeepEquals, before.ObjectStoreMetadata)
	c.Check(after.ObjectStorePlacement, tc.DeepEquals, before.ObjectStorePlacement)
	checkString(c, s.DB(), `SELECT password_hash FROM controller_node_password WHERE controller_id = '0'`, "target")
	checkPlacement(c, s.DB(), "0")
}

func (s *modelSuite) TestExportDecodeLoadRoundTrip(c *tc.C) {
	seedBlob(c, s.DB(), "source")
	execSQL(c, s.DB(), `INSERT INTO sequence (namespace, value) VALUES ('machine', 7)`)
	execSQL(c, s.DB(), `INSERT INTO space (uuid, name) VALUES ('space', 'restored')`)
	execSQL(c, s.DB(), `INSERT INTO provider_space (provider_id, space_uuid) VALUES ('provider', 'space')`)
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
	checkPlacement(c, s.DB(), "replacement")
}

func (s *controllerSuite) TestDefaultsAndExplicitNull(c *tc.C) {
	dump, err := service.DecodeControllerDump([]byte(`payload:
  flag:
    - {name: missing, description: default}
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
		{"missing", sql.NullBool{Valid: true}},
		{"explicit-null", sql.NullBool{}},
		{"explicit-false", sql.NullBool{Valid: true}},
		{"explicit-true", sql.NullBool{Bool: true, Valid: true}},
	} {
		var got sql.NullBool
		err := s.DB().QueryRowContext(c.Context(), `SELECT value FROM flag WHERE name = ?`, test.name).Scan(&got)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(got, tc.Equals, test.want)
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
	seedBlob(c, s.DB(), "original")
	execSQL(c, s.DB(), `INSERT INTO sequence VALUES ('original', 1)`)
	dump, err := service.DecodeModelDump([]byte(`payload:
  sequence: [{namespace: replacement, value: 2}]
  provider_space: [{provider_id: broken, space_uuid: missing}]
`))
	c.Assert(err, tc.ErrorIsNil)
	err = state.NewState(s.TxnRunnerFactory()).LoadModelDump(c.Context(), dump, "replacement")
	c.Check(err, tc.ErrorMatches, `loaded database fails foreign key integrity: provider_space row referencing space`)
	checkString(c, s.DB(), `SELECT namespace FROM sequence`, "original")
	checkPlacement(c, s.DB(), "original")
	// Failed replay must leave FK enforcement enabled on the connection.
	_, err = s.DB().ExecContext(c.Context(), `INSERT INTO provider_space VALUES ('broken', 'missing')`)
	c.Check(err, tc.NotNil)
}

func (s *controllerSuite) TestInsertFailureRollsBack(c *tc.C) {
	execSQL(c, s.DB(), `INSERT INTO flag VALUES ('original', 1, 'original')`)
	dump, err := service.DecodeControllerDump([]byte(`payload:
  flag: [{name: broken}]
`))
	c.Assert(err, tc.ErrorIsNil)
	err = state.NewState(s.TxnRunnerFactory()).LoadControllerDump(c.Context(), dump, "target")
	c.Check(err, tc.ErrorMatches, `(?s)inserting table "flag":.*NOT NULL constraint failed: flag.description.*`)
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
