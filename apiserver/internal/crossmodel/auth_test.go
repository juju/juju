// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"fmt"
	"testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type authSuite struct {
	accessService *MockAccessService
	keyPair       *bakery.KeyPair

	controllerUUID string
	modelUUID      model.UUID
}

func TestAuthSuite(t *testing.T) {
	tc.Run(t, &authSuite{})
}

func (s *authSuite) TestOfferThirdPartyKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authContext := s.newAuthContext(c)
	c.Assert(authContext.OfferThirdPartyKey(), tc.Equals, s.keyPair)
}

func (s *authSuite) TestCheckOfferAccessCaveatNotOfferPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckOfferAccessCaveat(c.Context(), "other-caveat")
	c.Assert(err, tc.ErrorIs, checkers.ErrCaveatNotRecognized)
}

func (s *authSuite) TestCheckOfferAccessCaveatInvalidYAML(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckOfferAccessCaveat(c.Context(), "has-offer-permission invalid-yaml")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *authSuite) TestCheckOfferAccessCaveat(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authContext := s.newAuthContext(c)
	details, err := authContext.CheckOfferAccessCaveat(c.Context(), "has-offer-permission "+s.newAccessCaveat(s.modelUUID.String()))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(details, tc.DeepEquals, OfferAccessDetails{
		SourceModelUUID: s.modelUUID.String(),
		User:            "mary",
		OfferUUID:       "mysql-uuid",
		Relation:        "mediawiki:db mysql:server",
		Permission:      "consume",
	})
}

func (s *authSuite) TestCheckOfferAccessCaveatInvalidSourceModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckOfferAccessCaveat(c.Context(), "has-offer-permission "+s.newAccessCaveat("blah"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *authSuite) TestCheckOfferAccessCaveatInvalidUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckOfferAccessCaveat(c.Context(), "has-offer-permission "+s.newAccessCaveatWithUser("!not-a-user"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *authSuite) TestCheckOfferAccessCaveatInvalidPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authContext := s.newAuthContext(c)
	_, err := authContext.CheckOfferAccessCaveat(c.Context(), "has-offer-permission "+s.newAccessCaveatWithPermission("blah"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *authSuite) TestCheckLocalAccessRequest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authContext := s.newAuthContext(c)
	caveats, err := authContext.CheckLocalAccessRequest(c.Context(), s.newOfferAccessDetails())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(caveats, tc.DeepEquals, []checkers.Caveat{})
}

func (s *authSuite) newAccessCaveat(modelUUID string) string {
	return fmt.Sprintf(`
source-model-uuid: %s
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], modelUUID)
}

func (s *authSuite) newAccessCaveatWithUser(user string) string {
	return fmt.Sprintf(`
source-model-uuid: %s
username: %s
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: consume
`[1:], s.modelUUID.String(), user)
}

func (s *authSuite) newAccessCaveatWithPermission(permission string) string {
	return fmt.Sprintf(`
source-model-uuid: %s
username: mary
offer-uuid: mysql-uuid
relation-key: mediawiki:db mysql:server
permission: %s
`[1:], s.modelUUID.String(), permission)
}

func (s *authSuite) newOfferAccessDetails() OfferAccessDetails {
	return OfferAccessDetails{
		SourceModelUUID: s.modelUUID.String(),
		User:            "mary",
		OfferUUID:       "mysql-uuid",
		Relation:        "mediawiki:db mysql:server",
		Permission:      "consume",
	}
}

func (s *authSuite) newAuthContext(c *tc.C) *AuthContext {
	return NewAuthContext(
		s.accessService,
		s.keyPair,
		s.controllerUUID,
		s.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *authSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.accessService = NewMockAccessService(ctrl)

	s.keyPair = bakery.MustGenerateKey()

	s.controllerUUID = uuid.MustNewUUID().String()
	s.modelUUID = modeltesting.GenModelUUID(c)

	c.Cleanup(func() {
		s.accessService = nil
		s.keyPair = nil
	})

	return ctrl
}
