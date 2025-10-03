// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"
)

type macaroonSuite struct {
	baseSuite
}

func TestMacaroonSuite(t *testing.T) {
	tc.Run(t, &macaroonSuite{})
}

func (s *macaroonSuite) TestSaveMacaroonForRelation(c *tc.C) {
	relationUUID := s.addRelation(c)

	s.DumpTable(c, "relation")

	err := s.state.SaveMacaroonForRelation(c.Context(), relationUUID.String(), []byte("macaroon"))
	c.Assert(err, tc.ErrorIsNil)

	var bytes []byte
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT macaroon FROM application_remote_offerer_relation_macaroon WHERE relation_uuid = ?`, relationUUID,
		).Scan(&bytes)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bytes, tc.DeepEquals, []byte("macaroon"))
}

func (s *macaroonSuite) TestSaveMacaroonForRelationCalledMultipleTimes(c *tc.C) {
	relationUUID := s.addRelation(c)

	s.DumpTable(c, "relation")

	err := s.state.SaveMacaroonForRelation(c.Context(), relationUUID.String(), []byte("macaroon"))
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SaveMacaroonForRelation(c.Context(), relationUUID.String(), []byte("meshuggah"))
	c.Assert(err, tc.ErrorIsNil)

	var bytes []byte
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT macaroon FROM application_remote_offerer_relation_macaroon WHERE relation_uuid = ?`, relationUUID,
		).Scan(&bytes)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bytes, tc.DeepEquals, []byte("meshuggah"))
}
