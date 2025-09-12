// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"errors"
	"testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	crossmodelbakery "github.com/juju/juju/apiserver/internal/crossmodel/bakery"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type authenticatorSuite struct {
	bakery *MockOfferBakery

	modelUUID coremodel.UUID
}

func TestAuthenticatorSuite(t *testing.T) {
	tc.Run(t, &authenticatorSuite{})
}

func (s *authenticatorSuite) TestCheckOfferMacaroons(c *tc.C) {
	defer s.setupMocks(c).Finish()

	slice := macaroon.Slice{
		newMacaroon(c, "test"),
	}
	requiredValues := map[string]string{
		"offer-uuid":        "offer-uuid",
		"source-model-uuid": s.modelUUID.String(),
	}

	exp := s.bakery.EXPECT()
	exp.GetOfferRequiredValues(s.modelUUID.String(), "offer-uuid").Return(requiredValues, nil)

	declaredValues := crossmodelbakery.NewDeclaredValues("mary", s.modelUUID.String(), "offer-uuid", "relation-key")
	exp.InferDeclaredFromMacaroon(gomock.Any(), requiredValues).Return(declaredValues)

	exp.AllowedAuth(gomock.Any(), crossModelConsumeOp("offer-uuid"), slice).Return([]string{"does-not-matter"}, nil)

	auth := s.newAuthenticator(c)
	result, err := auth.CheckOfferMacaroons(c.Context(), s.modelUUID.String(), "offer-uuid", slice, bakery.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]string{
		"username":          "mary",
		"offer-uuid":        "offer-uuid",
		"source-model-uuid": s.modelUUID.String(),
		"relation-key":      "relation-key",
	})
}

func (s *authenticatorSuite) TestCheckOfferMacaroonsErrorGetOfferRequiredValues(c *tc.C) {
	defer s.setupMocks(c).Finish()

	slice := macaroon.Slice{
		newMacaroon(c, "test"),
	}
	requiredValues := map[string]string{
		"offer-uuid":        "offer-uuid",
		"source-model-uuid": s.modelUUID.String(),
	}
	exp := s.bakery.EXPECT()
	exp.GetOfferRequiredValues(s.modelUUID.String(), "offer-uuid").Return(requiredValues, coreerrors.NotValid)

	auth := s.newAuthenticator(c)
	_, err := auth.CheckOfferMacaroons(c.Context(), s.modelUUID.String(), "offer-uuid", slice, bakery.LatestVersion)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *authenticatorSuite) TestCheckOfferMacaroonsNoUserName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	slice := macaroon.Slice{
		newMacaroon(c, "test"),
	}
	requiredValues := map[string]string{
		"offer-uuid":        "offer-uuid",
		"source-model-uuid": s.modelUUID.String(),
	}

	exp := s.bakery.EXPECT()
	exp.GetOfferRequiredValues(s.modelUUID.String(), "offer-uuid").Return(requiredValues, nil)

	declaredValues := crossmodelbakery.NewDeclaredValues("", s.modelUUID.String(), "offer-uuid", "relation-key")
	exp.InferDeclaredFromMacaroon(gomock.Any(), requiredValues).Return(declaredValues)

	auth := s.newAuthenticator(c)
	_, err := auth.CheckOfferMacaroons(c.Context(), s.modelUUID.String(), "offer-uuid", slice, bakery.LatestVersion)
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *authenticatorSuite) TestCheckOfferMacaroonsAllowedAuthError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakeryMacaroon := newBakeryMacaroon(c, "test")
	slice := macaroon.Slice{
		bakeryMacaroon.M(),
	}
	requiredValues := map[string]string{
		"offer-uuid":        "offer-uuid",
		"source-model-uuid": s.modelUUID.String(),
	}

	exp := s.bakery.EXPECT()
	exp.GetOfferRequiredValues(s.modelUUID.String(), "offer-uuid").Return(requiredValues, nil)

	declaredValues := crossmodelbakery.NewDeclaredValues("mary", s.modelUUID.String(), "offer-uuid", "relation-key")
	exp.InferDeclaredFromMacaroon(gomock.Any(), requiredValues).Return(declaredValues)

	exp.AllowedAuth(gomock.Any(), crossModelConsumeOp("offer-uuid"), slice).Return([]string{}, coreerrors.NotValid)

	exp.CreateDischargeMacaroon(gomock.Any(), "mary", requiredValues, declaredValues, crossModelConsumeOp("offer-uuid"), bakery.LatestVersion).
		Return(bakeryMacaroon, nil)

	auth := s.newAuthenticator(c)
	_, err := auth.CheckOfferMacaroons(c.Context(), s.modelUUID.String(), "offer-uuid", slice, bakery.LatestVersion)
	c.Assert(err, tc.DeepEquals, &apiservererrors.DischargeRequiredError{
		Cause:          coreerrors.NotValid,
		Macaroon:       bakeryMacaroon,
		LegacyMacaroon: bakeryMacaroon.M(),
	})
}

func (s *authenticatorSuite) TestCheckOfferMacaroonsAllowedNoConditions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakeryMacaroon := newBakeryMacaroon(c, "test")
	slice := macaroon.Slice{
		bakeryMacaroon.M(),
	}
	requiredValues := map[string]string{
		"offer-uuid":        "offer-uuid",
		"source-model-uuid": s.modelUUID.String(),
	}

	exp := s.bakery.EXPECT()
	exp.GetOfferRequiredValues(s.modelUUID.String(), "offer-uuid").Return(requiredValues, nil)

	declaredValues := crossmodelbakery.NewDeclaredValues("mary", s.modelUUID.String(), "offer-uuid", "relation-key")
	exp.InferDeclaredFromMacaroon(gomock.Any(), requiredValues).Return(declaredValues)

	exp.AllowedAuth(gomock.Any(), crossModelConsumeOp("offer-uuid"), slice).Return([]string{}, nil)

	exp.CreateDischargeMacaroon(gomock.Any(), "mary", requiredValues, declaredValues, crossModelConsumeOp("offer-uuid"), bakery.LatestVersion).
		Return(bakeryMacaroon, nil)

	auth := s.newAuthenticator(c)
	_, err := auth.CheckOfferMacaroons(c.Context(), s.modelUUID.String(), "offer-uuid", slice, bakery.LatestVersion)
	c.Assert(err, tc.DeepEquals, &apiservererrors.DischargeRequiredError{
		Cause:          ErrInvalidMacaroon,
		Macaroon:       bakeryMacaroon,
		LegacyMacaroon: bakeryMacaroon.M(),
	})
}

func (s *authenticatorSuite) TestCheckOfferMacaroonsCheckMacaroonCaveatsMissingSourceModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	slice := macaroon.Slice{
		newMacaroon(c, "test"),
	}
	requiredValues := map[string]string{
		"offer-uuid":        "offer-uuid",
		"source-model-uuid": s.modelUUID.String(),
	}

	exp := s.bakery.EXPECT()
	exp.GetOfferRequiredValues(s.modelUUID.String(), "offer-uuid").Return(requiredValues, nil)

	declaredValues := crossmodelbakery.NewDeclaredValues("mary", "", "offer-uuid", "relation-key")
	exp.InferDeclaredFromMacaroon(gomock.Any(), requiredValues).Return(declaredValues)

	exp.AllowedAuth(gomock.Any(), crossModelConsumeOp("offer-uuid"), slice).Return([]string{"does-not-matter"}, nil)

	auth := s.newAuthenticator(c)
	_, err := auth.CheckOfferMacaroons(c.Context(), s.modelUUID.String(), "offer-uuid", slice, bakery.LatestVersion)
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *authenticatorSuite) TestCheckOfferMacaroonsCheckMacaroonCaveatsMissingOfferUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	slice := macaroon.Slice{
		newMacaroon(c, "test"),
	}
	requiredValues := map[string]string{
		"offer-uuid":        "offer-uuid",
		"source-model-uuid": s.modelUUID.String(),
	}

	exp := s.bakery.EXPECT()
	exp.GetOfferRequiredValues(s.modelUUID.String(), "offer-uuid").Return(requiredValues, nil)

	declaredValues := crossmodelbakery.NewDeclaredValues("mary", s.modelUUID.String(), "", "relation-key")
	exp.InferDeclaredFromMacaroon(gomock.Any(), requiredValues).Return(declaredValues)

	exp.AllowedAuth(gomock.Any(), crossModelConsumeOp("offer-uuid"), slice).Return([]string{"does-not-matter"}, nil)

	auth := s.newAuthenticator(c)
	_, err := auth.CheckOfferMacaroons(c.Context(), s.modelUUID.String(), "offer-uuid", slice, bakery.LatestVersion)
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *authenticatorSuite) TestCheckOfferMacaroonsCheckMacaroonCaveatsMissMatchOfferUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	bakeryMacaroon := newBakeryMacaroon(c, "test")
	slice := macaroon.Slice{
		bakeryMacaroon.M(),
	}
	requiredValues := map[string]string{
		"offer-uuid":        "offer-uuid",
		"source-model-uuid": s.modelUUID.String(),
	}

	exp := s.bakery.EXPECT()
	exp.GetOfferRequiredValues(s.modelUUID.String(), "offer-uuid").Return(requiredValues, nil)

	declaredValues := crossmodelbakery.NewDeclaredValues("mary", s.modelUUID.String(), "scallywags", "relation-key")
	exp.InferDeclaredFromMacaroon(gomock.Any(), requiredValues).Return(declaredValues)

	exp.AllowedAuth(gomock.Any(), crossModelConsumeOp("offer-uuid"), slice).Return([]string{"does-not-matter"}, nil)

	exp.CreateDischargeMacaroon(gomock.Any(), "mary", requiredValues, declaredValues, crossModelConsumeOp("offer-uuid"), bakery.LatestVersion).
		Return(bakeryMacaroon, nil)

	auth := s.newAuthenticator(c)
	_, err := auth.CheckOfferMacaroons(c.Context(), s.modelUUID.String(), "offer-uuid", slice, bakery.LatestVersion)

	var target *apiservererrors.DischargeRequiredError
	c.Assert(errors.As(err, &target), tc.IsTrue)

	c.Check(target.Cause, tc.ErrorIs, coreerrors.Unauthorized)
	c.Check(target.Macaroon, tc.Equals, bakeryMacaroon)
	c.Check(target.LegacyMacaroon, tc.Equals, bakeryMacaroon.M())
}

func (s *authenticatorSuite) TestCheckRelationMacaroons(c *tc.C) {
	defer s.setupMocks(c).Finish()

	slice := macaroon.Slice{
		newMacaroon(c, "test"),
	}
	requiredValues := map[string]string{
		"offer-uuid":        "offer-uuid",
		"source-model-uuid": s.modelUUID.String(),
	}

	exp := s.bakery.EXPECT()
	exp.GetRelationRequiredValues(s.modelUUID.String(), "offer-uuid", "wordpress:mysql").Return(requiredValues, nil)

	declaredValues := crossmodelbakery.NewDeclaredValues("mary", s.modelUUID.String(), "offer-uuid", "wordpress:mysql")
	exp.InferDeclaredFromMacaroon(gomock.Any(), requiredValues).Return(declaredValues)

	exp.AllowedAuth(gomock.Any(), crossModelRelateOp("wordpress:mysql"), slice).Return([]string{"does-not-matter"}, nil)

	auth := s.newAuthenticator(c)
	err := auth.CheckRelationMacaroons(c.Context(), s.modelUUID.String(), "offer-uuid", names.NewRelationTag("wordpress:mysql"), slice, bakery.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *authenticatorSuite) TestCheckRelationMacaroonsMissingUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	slice := macaroon.Slice{
		newMacaroon(c, "test"),
	}
	requiredValues := map[string]string{
		"offer-uuid":        "offer-uuid",
		"source-model-uuid": s.modelUUID.String(),
	}

	exp := s.bakery.EXPECT()
	exp.GetRelationRequiredValues(s.modelUUID.String(), "offer-uuid", "wordpress:mysql").Return(requiredValues, nil)

	declaredValues := crossmodelbakery.NewDeclaredValues("", s.modelUUID.String(), "offer-uuid", "wordpress:mysql")
	exp.InferDeclaredFromMacaroon(gomock.Any(), requiredValues).Return(declaredValues)

	auth := s.newAuthenticator(c)
	err := auth.CheckRelationMacaroons(c.Context(), s.modelUUID.String(), "offer-uuid", names.NewRelationTag("wordpress:mysql"), slice, bakery.LatestVersion)
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *authenticatorSuite) TestCheckRelationMacaroonsInvalidOp(c *tc.C) {
	defer s.setupMocks(c).Finish()

	slice := macaroon.Slice{
		newMacaroon(c, "test"),
	}
	requiredValues := map[string]string{
		"offer-uuid":        "offer-uuid",
		"source-model-uuid": s.modelUUID.String(),
	}

	exp := s.bakery.EXPECT()
	exp.GetRelationRequiredValues(s.modelUUID.String(), "offer-uuid", "wordpress:mysql").Return(requiredValues, nil)

	declaredValues := crossmodelbakery.NewDeclaredValues("mary", s.modelUUID.String(), "offer-uuid", "")
	exp.InferDeclaredFromMacaroon(gomock.Any(), requiredValues).Return(declaredValues)

	exp.AllowedAuth(gomock.Any(), crossModelRelateOp("wordpress:mysql"), slice).Return([]string{"does-not-matter"}, nil)

	auth := s.newAuthenticator(c)
	err := auth.CheckRelationMacaroons(c.Context(), s.modelUUID.String(), "offer-uuid", names.NewRelationTag("wordpress:mysql"), slice, bakery.LatestVersion)
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *authenticatorSuite) newAuthenticator(c *tc.C) *Authenticator {
	return &Authenticator{
		bakery: s.bakery,
		logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *authenticatorSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.bakery = NewMockOfferBakery(ctrl)

	s.modelUUID = modeltesting.GenModelUUID(c)

	c.Cleanup(func() {
		s.bakery = nil
	})

	return ctrl
}
