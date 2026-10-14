// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/recovery"
)

type decodeSuite struct{}

func TestDecodeSuite(t *testing.T) { tc.Run(t, &decodeSuite{}) }

func (s *decodeSuite) TestTablePresence(c *tc.C) {
	for _, value := range []string{"null", "[]"} {
		controller, err := DecodeControllerDump([]byte("payload:\n  flag: " + value))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(controller.Present["flag"], tc.IsTrue)
		c.Check(controller.Present["user"], tc.IsFalse)
		c.Check(controller.ControllerFlag, tc.HasLen, 0)
		model, err := DecodeModelDump([]byte("payload:\n  sequence: " + value))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(model.Present["sequence"], tc.IsTrue)
		c.Check(model.Present["space"], tc.IsFalse)
		c.Check(model.ModelSequence, tc.HasLen, 0)
	}
}

func (s *decodeSuite) TestInvalidDumps(c *tc.C) {
	for _, test := range []struct{ data, message string }{
		{"[", "decoding dump:.*"},
		{"payload: {}", "decoding dump: empty payload"},
		{"payload: {unknown_table: []}", `unknown recovery table "unknown_table"`},
		{"payload: {flag: [{unknown_column: true}]}", `(?s)table "flag":.*field unknown_column not found.*`},
		{"payload: {flag: [{value: [1, 2]}]}", `(?s)table "flag":.*cannot unmarshal.*`},
		{"payload: {macaroon_root_key: [{root_key: [256]}]}", `table "macaroon_root_key": row 0 column "root_key": byte sequence element 0 is not a byte \(int\)`},
	} {
		_, err := DecodeControllerDump([]byte(test.data))
		c.Check(err, tc.ErrorMatches, test.message)
	}
}

func (s *decodeSuite) TestDefaults(c *tc.C) {
	dump, err := DecodeControllerDump([]byte(`payload:
  flag:
    - {name: absent}
    - {name: null, value: null}
    - {name: false, value: false}
    - {name: true, value: true}
`))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dump.ControllerFlag, tc.HasLen, 4)
	for i, want := range []bool{false, true, true, true} {
		c.Check(dump.ControllerFlag[i].HasValue, tc.Equals, want)
	}
	c.Check(dump.ControllerFlag[0].Value, tc.IsNil)
	c.Check(dump.ControllerFlag[1].Value, tc.IsNil)
	c.Assert(dump.ControllerFlag[2].Value, tc.NotNil)
	c.Check(*dump.ControllerFlag[2].Value, tc.IsFalse)
	c.Assert(dump.ControllerFlag[3].Value, tc.NotNil)
	c.Check(*dump.ControllerFlag[3].Value, tc.IsTrue)
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
		input := []row{{"root_key": test.value}}
		rows, err := decodeRows[recovery.ControllerMacaroonRootKey](input, "root_key")
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(rows, tc.HasLen, 1)
		c.Assert(rows[0].RootKey, tc.NotNil)
		c.Check(*rows[0].RootKey, tc.DeepEquals, test.want)
		c.Check(input[0]["root_key"], tc.DeepEquals, test.value)
	}
	for _, value := range []any{true, 12, []any{-1}, []any{1.5}, []any{"1"}} {
		_, err := decodeBinaryValue(value)
		c.Check(err, tc.NotNil)
	}
	for _, input := range []row{{}, {"root_key": nil}} {
		rows, err := decodeRows[recovery.ControllerMacaroonRootKey]([]row{input}, "root_key")
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(rows, tc.HasLen, 1)
		c.Check(rows[0].RootKey, tc.IsNil)
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
`))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(model.Present, tc.HasLen, 0)
}
