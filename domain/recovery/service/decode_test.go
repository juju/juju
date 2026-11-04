// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"

	controllertypes "github.com/juju/juju/domain/export/types/controller/v4_1_0"
	modeltypes "github.com/juju/juju/domain/export/types/v4_1_0"
)

type decodeSuite struct{}

func TestDecodeSuite(t *testing.T) { tc.Run(t, &decodeSuite{}) }

func (s *decodeSuite) TestTablePresence(c *tc.C) {
	for _, value := range []string{"null", "[]"} {
		controller, err := DecodeControllerDump([]byte("payload:\n  flag: " + value))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(controller.Present["flag"], tc.IsTrue)
		c.Check(controller.Present["user"], tc.IsFalse)
		c.Check(controller.Flag, tc.HasLen, 0)
		model, err := DecodeModelDump([]byte("payload:\n  sequence: " + value))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(model.Present["sequence"], tc.IsTrue)
		c.Check(model.Present["space"], tc.IsFalse)
		c.Check(model.Sequence, tc.HasLen, 0)
	}
}

func (s *decodeSuite) TestInvalidDumps(c *tc.C) {
	for _, test := range []struct{ data, message string }{
		{"[", "decoding dump:.*"},
		{"payload: {}", "decoding dump: empty payload"},
		{"payload: {unknown_table: []}", `unknown recovery table "unknown_table"`},
		{"payload: {flag: [{name: test, value: false, description: test, unknown_column: true}]}", `(?s)table "flag":.*field unknown_column not found.*`},
		{"payload: {flag: [{name: test, value: [1, 2], description: test}]}", `(?s)table "flag":.*cannot unmarshal.*`},
		{"payload: {macaroon_root_key: [{id: key, created_at: 2026-01-01, expires_at: 2027-01-01, root_key: [256]}]}", `table "macaroon_root_key": row 0 column "root_key": byte sequence element 0 is not a byte \(int\)`},
	} {
		_, err := DecodeControllerDump([]byte(test.data))
		c.Check(err, tc.ErrorMatches, test.message)
	}
}

func (s *decodeSuite) TestNullAndBooleanValues(c *tc.C) {
	dump, err := DecodeControllerDump([]byte(`payload:
  flag:
    - {name: explicit-null, value: null, description: test}
    - {name: explicit-false, value: false, description: test}
    - {name: explicit-true, value: true, description: test}
`))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dump.Flag, tc.HasLen, 3)
	c.Check(dump.Flag[0].Value, tc.IsNil)
	c.Assert(dump.Flag[1].Value, tc.NotNil)
	c.Check(*dump.Flag[1].Value, tc.IsFalse)
	c.Assert(dump.Flag[2].Value, tc.NotNil)
	c.Check(*dump.Flag[2].Value, tc.IsTrue)
}

func (s *decodeSuite) TestBinaryRepresentations(c *tc.C) {
	for _, test := range []struct {
		value any
		want  []byte
	}{
		{"plain", []byte("plain")},
		{string([]byte{0, 128, 255}), []byte{0, 128, 255}},
		{[]byte{0, 128, 255}, []byte{0, 128, 255}},
		{[]any{0, 128, 255}, []byte{0, 128, 255}},
		{[]any{}, []byte{}},
	} {
		input := []row{{"id": "key", "created_at": time.Time{}, "expires_at": time.Time{}, "root_key": test.value}}
		rows, err := decodeRows[controllertypes.MacaroonRootKey](input, "root_key")
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(rows, tc.HasLen, 1)
		c.Check(rows[0].RootKey, tc.DeepEquals, test.want)
		c.Check(input[0]["root_key"], tc.DeepEquals, test.value)
	}
	for _, value := range []any{true, 12, []any{-1}, []any{1.5}, []any{"1"}} {
		_, err := decodeBinaryValue(value)
		c.Check(err, tc.NotNil)
	}
}

func (s *decodeSuite) TestMissingAndNullColumns(c *tc.C) {
	for _, test := range []struct{ data, message string }{
		{"{name: test, description: test}", `table "flag": row 0 is missing column "value"`},
		{"{name: test, value: false}", `table "flag": row 0 is missing column "description"`},
		{"{name: test, value: false, description: null}", `table "flag": row 0 column "description" cannot be null`},
	} {
		_, err := DecodeControllerDump([]byte("payload: {flag: [" + test.data + "]}"))
		c.Check(err, tc.ErrorMatches, test.message)
	}
}

func (s *decodeSuite) TestNullableBinaryText(c *tc.C) {
	for _, value := range []any{nil, "", "plain", []byte{0, 128, 255}} {
		input := []row{{"machine_uuid": "machine", "status_id": 0, "message": nil, "data": value, "updated_at": nil}}
		rows, err := decodeRows[modeltypes.MachineStatus](input, "data")
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(rows, tc.HasLen, 1)
		if value == nil {
			c.Check(rows[0].Data, tc.IsNil)
		} else {
			c.Assert(rows[0].Data, tc.NotNil)
			want, err := decodeBinaryValue(value)
			c.Assert(err, tc.ErrorIsNil)
			c.Check([]byte(*rows[0].Data), tc.DeepEquals, want)
		}
	}
}

func (s *decodeSuite) TestNativeBinary(c *tc.C) {
	for _, value := range []any{nil, "", "plain", []byte{0, 128, 255}, []any{0, 128, 255}} {
		input := []row{{"id": "key", "algorithm_type_id": 0, "ssh_key": "key", "public_key": value}}
		rows, err := decodeRows[controllertypes.ControllerSshHostKey](input, "public_key")
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(rows, tc.HasLen, 1)
		if value == nil {
			c.Check(rows[0].PublicKey, tc.IsNil)
		} else {
			want, err := decodeBinaryValue(value)
			c.Assert(err, tc.ErrorIsNil)
			c.Check(rows[0].PublicKey, tc.DeepEquals, want)
		}
		c.Check(input[0]["public_key"], tc.DeepEquals, value)
	}
}

func (s *decodeSuite) TestExcludedTables(c *tc.C) {
	dump, err := DecodeControllerDump([]byte(`payload:
  schema: [{version: 999}]
  controller_node: [{controller_id: source}]
  controller_node_password: [{controller_id: source}]
  controller_node_nonce: [{controller_id: source}]
  controller_node_agent_version: []
  controller_api_address: []
  controller_agent_address: []
  controller_client_address: []
  controller_peer_address: []
  lease: []
  lease_pin: []
  upgrade_info: []
  upgrade_info_controller_node: []
  change_log: []
  change_log_witness: []
  change_log_namespace: []
  change_log_edit_type: []
  object_store_placement: [{uuid: blob, node_id: source}]
`))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dump.Present, tc.HasLen, 0)
	model, err := DecodeModelDump([]byte(`payload:
  schema: []
  change_log: []
  change_log_witness: []
  change_log_namespace: []
  change_log_edit_type: []
  object_store_placement: []
  log_transfer_progress: []
`))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(model.Present, tc.HasLen, 0)
}
