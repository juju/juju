// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	removalerrors "github.com/juju/juju/domain/removal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type offerSuite struct {
	baseSuite
}

func TestOfferSuite(t *testing.T) {
	tc.Run(t, &offerSuite{})
}

func (s *offerSuite) TestOfferExistsFalse(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.OfferExists(c.Context(), "some-offer-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *offerSuite) TestOfferExists(c *tc.C) {
	offerUUID := s.createOffer(c, "foo")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.OfferExists(c.Context(), offerUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

func (s *offerSuite) TestDeleteOffer(c *tc.C) {
	offerUUID := s.createOffer(c, "foo")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteOffer(c.Context(), offerUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	exists, err := st.OfferExists(c.Context(), offerUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *offerSuite) TestDeleteOfferSuperfluousForce(c *tc.C) {
	offerUUID := s.createOffer(c, "foo")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteOffer(c.Context(), offerUUID.String(), true)
	c.Assert(err, tc.ErrorIsNil)

	exists, err := st.OfferExists(c.Context(), offerUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *offerSuite) TestDeleteOfferFailsWithRelations(c *tc.C) {
	_, _, offerUUID := s.createRelationWithRemoteConsumer(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteOffer(c.Context(), offerUUID.String(), false)
	c.Check(err, tc.ErrorIs, removalerrors.OfferHasRelations)
	c.Check(err, tc.ErrorIs, removalerrors.ForceRequired)
}

func (s *offerSuite) TestDeleteOfferForceWithRelations(c *tc.C) {
	_, synthAppUUID, offerUUID := s.createRelationWithRemoteConsumer(c)
	secondAppUUID, _ := s.createRemoteApplicationConsumer(c, "second", offerUUID)
	otherOfferUUID := s.createOffer(c, "other-offer")
	otherAppUUID, _ := s.createRemoteApplicationConsumer(c, "other", otherOfferUUID)
	consumers := []secretRemoteUnitConsumerRow{
		{"secret-1", synthAppUUID.String(), "foo/0", 1},
		// Tracking does not require a corresponding synthetic unit.
		{"secret-1", synthAppUUID.String(), "foo/99", 2},
		{"secret-2", secondAppUUID.String(), "second/99", 3},
		{"secret-1", otherAppUUID.String(), "other/0", 4},
	}
	s.addSecretRemoteUnitConsumers(c, consumers...)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteOffer(c.Context(), offerUUID.String(), false)
	c.Assert(err, tc.ErrorIs, removalerrors.OfferHasRelations)
	c.Check(err, tc.ErrorIs, removalerrors.ForceRequired)
	c.Check(s.secretRemoteUnitConsumers(c), tc.SameContents, consumers)

	err = st.DeleteOffer(c.Context(), offerUUID.String(), true)
	c.Assert(err, tc.ErrorIsNil)

	exists, err := st.OfferExists(c.Context(), offerUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	var (
		relCount       int
		remoteAppCount int
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM relation").Scan(&relCount)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM application_remote_consumer").Scan(&remoteAppCount)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(relCount, tc.Equals, 1)
	c.Check(remoteAppCount, tc.Equals, 1)
	c.Check(s.secretRemoteUnitConsumers(c), tc.SameContents, consumers[3:])
	var connections int
	err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM offer_connection WHERE offer_uuid = ?`, offerUUID.String()).Scan(&connections)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(connections, tc.Equals, 0)
	err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM offer_connection WHERE uuid = ?`, otherAppUUID.String()).Scan(&connections)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(connections, tc.Equals, 1)
	exists, err = st.OfferExists(c.Context(), otherOfferUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
}
