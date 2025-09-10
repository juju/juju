// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	coreerrors "github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
)

type localBakerySuite struct {
	baseBakerySuite
}

func TestLocalBakerySuite(t *testing.T) {
	tc.Run(t, &localBakerySuite{})
}

func (s *localBakerySuite) TestNewLocalOfferBakery(c *tc.C) {
	defer s.setupMocks(c).Finish()

	checker := checkers.New(internalmacaroon.MacaroonNamespace)
	bakery, err := NewLocalOfferBakery(
		s.keyPair,
		"juju model",
		s.store,
		checker,
		s.authorizer,
		s.clock,
		loggertesting.WrapCheckLog(c),
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(bakery, tc.Not(tc.IsNil))
}

func (s *localBakerySuite) TestParseCaveatNoOfferPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := LocalOfferBakery{}
	_, err := bakery.ParseCaveat("naughty-caveat")
	c.Check(err, tc.Equals, checkers.ErrCaveatNotRecognized)
}

func (s *localBakerySuite) TestParseCaveatInvalidYAML(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := LocalOfferBakery{}
	_, err := bakery.ParseCaveat("has-offer-permission invalid-yaml")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *localBakerySuite) TestParseCaveat(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakery := LocalOfferBakery{}
	details, err := bakery.ParseCaveat("has-offer-permission " + s.newAccessCaveat(s.modelUUID.String()))
	c.Check(err, tc.ErrorIsNil)
	c.Check(details, tc.DeepEquals, OfferAccessDetails{
		SourceModelUUID: s.modelUUID.String(),
		User:            "mary",
		OfferUUID:       "mysql-uuid",
		Relation:        "mediawiki:db mysql:server",
		Permission:      "consume",
	})
}

func (s *localBakerySuite) TestGetConsumeOfferCaveats(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)
	s.clock.EXPECT().Now().Return(now)

	bakery := LocalOfferBakery{
		clock: s.clock,
	}
	caveats := bakery.GetConsumeOfferCaveats("mysql-uuid", s.modelUUID.String(), "mary", "")

	c.Check(caveats, tc.SameContents, s.caveats(now))
}

func (s *localBakerySuite) TestGetConsumeOfferCaveatsWithRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)
	s.clock.EXPECT().Now().Return(now)

	bakery := LocalOfferBakery{
		clock: s.clock,
	}
	caveats := bakery.GetConsumeOfferCaveats("mysql-uuid", s.modelUUID.String(), "mary", "mediawiki:db mysql:server")

	c.Check(caveats, tc.SameContents, s.caveatWithRelation(now))
}

func (s *localBakerySuite) TestGetRemoteRelationCaveats(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)
	s.clock.EXPECT().Now().Return(now)

	bakery := LocalOfferBakery{
		clock: s.clock,
	}
	caveats := bakery.GetRemoteRelationCaveats("mysql-uuid", s.modelUUID.String(), "mary", "mediawiki:db mysql:server")

	c.Check(caveats, tc.SameContents, s.caveatWithRelation(now))
}

func (s *localBakerySuite) TestInferDeclaredFromMacaroon(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Local bakery should not infer any declared values.

	mac := newMacaroon(c, "test")

	bakery := LocalOfferBakery{}
	m := bakery.InferDeclaredFromMacaroon(macaroon.Slice{mac}, map[string]string{"relation-key": "mediawiki:db mysql:server"})
	c.Check(m, tc.DeepEquals, map[string]string{})
}

func (s *localBakerySuite) TestCreateDischargeMacaroon(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().Truncate(time.Second)
	s.clock.EXPECT().Now().Return(now)

	expectedMac := newBakeryMacaroon(c, "test")
	op := bakery.Op{Action: "consume", Entity: "mysql-uuid"}

	var caveats []checkers.Caveat
	s.oven.EXPECT().NewMacaroon(
		gomock.Any(),
		bakery.LatestVersion,
		gomock.Any(),
		op,
	).DoAndReturn(func(ctx context.Context, v bakery.Version, c []checkers.Caveat, o ...bakery.Op) (*bakery.Macaroon, error) {
		caveats = c
		return expectedMac, nil
	})

	localBakery := LocalOfferBakery{
		oven:  s.oven,
		clock: s.clock,
	}

	mac, err := localBakery.CreateDischargeMacaroon(
		c.Context(),
		"http://offer-access",
		"mary",
		map[string]string{
			sourceModelKey: s.modelUUID.String(),
			relationKey:    "mediawiki:db mysql:server",
			offerUUIDKey:   "mysql-uuid",
		},
		map[string]string{},
		op,
		bakery.LatestVersion,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mac, tc.Equals, expectedMac)

	c.Check(caveats, tc.DeepEquals, []checkers.Caveat{
		checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Location:  "http://offer-access",
				Condition: "has-offer-permission " + s.newAccessCaveat(s.modelUUID.String()),
			},
			"offer-uuid", "relation-key", "source-model-uuid", "username",
		),
		checkers.TimeBeforeCaveat(now.Add(offerPermissionExpiryTime)),
	})
}

func (s *localBakerySuite) caveats(now time.Time) []checkers.Caveat {
	return []checkers.Caveat{
		checkers.DeclaredCaveat(sourceModelKey, s.modelUUID.String()),
		checkers.DeclaredCaveat(offerUUIDKey, "mysql-uuid"),
		checkers.DeclaredCaveat(usernameKey, "mary"),
		checkers.TimeBeforeCaveat(now.Add(offerPermissionExpiryTime)),
	}
}

func (s *localBakerySuite) caveatWithRelation(now time.Time) []checkers.Caveat {
	return append(s.caveats(now), checkers.DeclaredCaveat(relationKey, "mediawiki:db mysql:server"))
}

func (s *localBakerySuite) newAccessCaveat(modelUUID string) string {
	return fmt.Sprintf(`
source-model-uuid: %s
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], modelUUID)
}
