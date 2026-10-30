// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	exportcontroller "github.com/juju/juju/domain/export/state/controller"
	exportmodel "github.com/juju/juju/domain/export/state/model"
	"github.com/juju/juju/domain/recovery/service"
	"github.com/juju/juju/domain/recovery/state"
)

func (s *modelSuite) TestLookupKeyExportDecodeLoadRoundTrip(c *tc.C) {
	execSQL(c, s.DB(), `INSERT INTO agent_stream (id, name) VALUES (7, 'recovery-stream')`)
	exporter := exportmodel.NewState(s.TxnRunnerFactory())
	before, err := exporter.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	// Select only these mutable lookup tables. Other exported tables may be
	// immutable bootstrap fixtures and cannot be deleted during replay.
	data, err := yaml.Marshal(map[string]any{"payload": map[string]any{
		"agent_stream": before.AgentStream, "life": before.Life,
	}})
	c.Assert(err, tc.ErrorIsNil)
	dump, err := service.DecodeModelDump(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dump.AgentStream, tc.SameContents, before.AgentStream)
	c.Check(dump.Life, tc.SameContents, before.Life)
	execSQL(c, s.DB(), `DELETE FROM agent_stream WHERE id = 7`)
	c.Assert(state.NewState(s.TxnRunnerFactory()).LoadModelDump(c.Context(), dump, "target"), tc.ErrorIsNil)
	after, err := exporter.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(after.AgentStream, tc.SameContents, before.AgentStream)
	c.Check(after.Life, tc.SameContents, before.Life)
	checkString(c, s.DB(), `SELECT name FROM agent_stream WHERE id = 0`, "released")
	checkString(c, s.DB(), `SELECT name FROM agent_stream WHERE id = 7`, "recovery-stream")
	checkString(c, s.DB(), `SELECT group_concat(DISTINCT typeof(id)) FROM agent_stream`, "integer")
	checkString(c, s.DB(), `SELECT value FROM life WHERE id = 0`, "alive")
}

func (s *controllerSuite) TestLookupKeyExportDecodeLoadRoundTrip(c *tc.C) {
	execSQL(c, s.DB(), `INSERT INTO cloud_type (id, type) VALUES (999, 'recovery-provider')`)
	exporter := exportcontroller.NewState(s.TxnRunnerFactory())
	before, err := exporter.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	data, err := yaml.Marshal(map[string]any{"payload": map[string]any{
		"cloud_type": before.CloudType, "life": before.Life,
	}})
	c.Assert(err, tc.ErrorIsNil)
	dump, err := service.DecodeControllerDump(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dump.CloudType, tc.SameContents, before.CloudType)
	c.Check(dump.Life, tc.SameContents, before.Life)
	execSQL(c, s.DB(), `DELETE FROM cloud_type WHERE id = 999`)
	c.Assert(state.NewState(s.TxnRunnerFactory()).LoadControllerDump(c.Context(), dump, "target"), tc.ErrorIsNil)
	after, err := exporter.Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(after.CloudType, tc.SameContents, before.CloudType)
	c.Check(after.Life, tc.SameContents, before.Life)
	checkString(c, s.DB(), `SELECT type FROM cloud_type WHERE id = 0`, "kubernetes")
	checkString(c, s.DB(), `SELECT type FROM cloud_type WHERE id = 999`, "recovery-provider")
	checkString(c, s.DB(), `SELECT group_concat(DISTINCT typeof(id)) FROM cloud_type`, "integer")
	checkString(c, s.DB(), `SELECT value FROM life WHERE id = 0`, "alive")
}

func (s *modelSuite) TestMalformedLookupKeyDoesNotReplay(c *tc.C) {
	seedBlob(c, s.DB(), "original")
	execSQL(c, s.DB(), `INSERT INTO sequence VALUES ('original', 1)`)
	for _, id := range []string{"id: null, ", ""} {
		data := []byte(`payload:
  sequence: [{namespace: replacement, value: 2}]
  agent_stream: [{id: 0, name: released}, {` + id + `name: malformed}]
`)
		dump, err := service.DecodeModelDump(data)
		c.Assert(err, tc.NotNil)
		c.Check(dump, tc.IsNil)
		checkString(c, s.DB(), `SELECT namespace FROM sequence`, "original")
		checkString(c, s.DB(), `SELECT name FROM agent_stream WHERE id = 0`, "released")
		checkPlacement(c, s.DB(), "original")
	}
}

func (s *controllerSuite) TestMalformedLookupKeyDoesNotReplay(c *tc.C) {
	seedBlob(c, s.DB(), "original")
	execSQL(c, s.DB(), `INSERT INTO flag VALUES ('original', 1, 'original')`)
	for _, id := range []string{"id: null, ", ""} {
		data := []byte(`payload:
  flag: [{name: replacement, value: true, description: replacement}]
  cloud_type: [{id: 0, type: kubernetes}, {` + id + `type: malformed}]
`)
		dump, err := service.DecodeControllerDump(data)
		c.Assert(err, tc.NotNil)
		c.Check(dump, tc.IsNil)
		checkString(c, s.DB(), `SELECT name FROM flag`, "original")
		checkString(c, s.DB(), `SELECT type FROM cloud_type WHERE id = 0`, "kubernetes")
		checkPlacement(c, s.DB(), "original")
	}
}

func (s *modelSuite) TestLookupKeyReplayFailureRollsBack(c *tc.C) {
	seedBlob(c, s.DB(), "original")
	execSQL(c, s.DB(), `INSERT INTO sequence VALUES ('original', 1)`)
	data := []byte(`payload:
  agent_stream: [{id: 0, name: replacement}, {id: 0, name: duplicate}]
  sequence: [{namespace: replacement, value: 2}]
`)
	dump, err := service.DecodeModelDump(data)
	c.Assert(err, tc.ErrorIsNil)
	err = state.NewState(s.TxnRunnerFactory()).LoadModelDump(c.Context(), dump, "replacement")
	c.Check(err, tc.ErrorMatches, `(?s)inserting table "agent_stream":.*UNIQUE constraint failed: agent_stream.id.*`)
	checkString(c, s.DB(), `SELECT namespace FROM sequence`, "original")
	checkString(c, s.DB(), `SELECT name FROM agent_stream WHERE id = 0`, "released")
	checkString(c, s.DB(), `SELECT COUNT(*) FROM agent_stream`, "4")
	checkPlacement(c, s.DB(), "original")
}

func (s *controllerSuite) TestLookupKeyReplayFailureRollsBack(c *tc.C) {
	seedBlob(c, s.DB(), "original")
	execSQL(c, s.DB(), `INSERT INTO flag VALUES ('original', 1, 'original')`)
	data := []byte(`payload:
  cloud_type: [{id: 0, type: replacement}, {id: 0, type: duplicate}]
  flag: [{name: replacement, value: true, description: replacement}]
`)
	dump, err := service.DecodeControllerDump(data)
	c.Assert(err, tc.ErrorIsNil)
	err = state.NewState(s.TxnRunnerFactory()).LoadControllerDump(c.Context(), dump, "replacement")
	c.Check(err, tc.ErrorMatches, `(?s)inserting table "cloud_type":.*UNIQUE constraint failed: cloud_type.id.*`)
	checkString(c, s.DB(), `SELECT name FROM flag`, "original")
	checkString(c, s.DB(), `SELECT type FROM cloud_type WHERE id = 0`, "kubernetes")
	checkString(c, s.DB(), `SELECT type FROM cloud_type WHERE id = 1`, "lxd")
	checkPlacement(c, s.DB(), "original")
}
