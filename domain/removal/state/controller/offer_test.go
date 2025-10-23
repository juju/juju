// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/offer"
	crossmodelrelationstate "github.com/juju/juju/domain/crossmodelrelation/state/controller"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type offerSuite struct {
	baseSuite
}

func TestOfferSuite(t *testing.T) {
	tc.Run(t, &offerSuite{})
}

func (s *offerSuite) TestDeleteOfferAccessNoop(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteOfferAccess(c.Context(), "some-offer-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *offerSuite) TestDeleteOfferAccess(c *tc.C) {
	cmrSt := crossmodelrelationstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	permissionUUID := tc.Must(c, uuid.NewUUID)
	offerUUID := tc.Must(c, offer.NewUUID)

	var ownerUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM user").Scan(&ownerUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	err = cmrSt.CreateOfferAccess(c.Context(), permissionUUID, offerUUID, tc.Must1(c, uuid.UUIDFromString, ownerUUID))
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteOfferAccess(c.Context(), offerUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	var count int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM permission WHERE grant_on = ?", offerUUID).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}
