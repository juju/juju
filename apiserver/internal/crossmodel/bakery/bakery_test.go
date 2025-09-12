// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakery

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
)

type bakerySuite struct {
	baseBakerySuite
}

func TestBakerySuite(t *testing.T) {
	tc.Run(t, &bakerySuite{})
}

func (s *bakerySuite) TestGetOfferRequiredValues(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := baseBakery{}
	m, err := bakery.GetOfferRequiredValues(s.modelUUID.String(), "offer-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(m, tc.DeepEquals, map[string]string{
		sourceModelKey: s.modelUUID.String(),
		offerUUIDKey:   "offer-uuid",
	})
}

func (s *bakerySuite) TestGetOfferRequiredValuesEmptySourceModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := baseBakery{}
	_, err := bakery.GetOfferRequiredValues("", "offer-uuid")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *bakerySuite) TestGetOfferRequiredValuesEmptyOfferUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := baseBakery{}
	_, err := bakery.GetOfferRequiredValues(s.modelUUID.String(), "")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *bakerySuite) TestGetRelationRequiredValues(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := baseBakery{}
	m, err := bakery.GetRelationRequiredValues(s.modelUUID.String(), "offer-uuid", "relation-key")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(m, tc.DeepEquals, map[string]string{
		sourceModelKey: s.modelUUID.String(),
		offerUUIDKey:   "offer-uuid",
		relationKey:    "relation-key",
	})
}

func (s *bakerySuite) TestGetRelationRequiredValuesEmptySourceModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := baseBakery{}
	_, err := bakery.GetRelationRequiredValues("", "offer-uuid", "relation-key")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *bakerySuite) TestGetRelationRequiredValuesEmptyOfferUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := baseBakery{}
	_, err := bakery.GetRelationRequiredValues(s.modelUUID.String(), "", "relation-key")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *bakerySuite) TestGetRelationRequiredValuesEmptyRelationKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := baseBakery{}
	_, err := bakery.GetRelationRequiredValues(s.modelUUID.String(), "offer-uuid", "")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}
