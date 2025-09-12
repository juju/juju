// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	crossmodelbakery "github.com/juju/juju/apiserver/internal/crossmodel/bakery"
	"github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
)

// Authenticator is used to authenticate macaroons used to access
// application offers.
type Authenticator struct {
	bakery OfferBakery
	logger logger.Logger
}

// CheckOfferMacaroons verifies that the specified macaroons allow access to the offer.
func (a *Authenticator) CheckOfferMacaroons(ctx context.Context, sourceModelUUID, offerUUID string, mac macaroon.Slice, version bakery.Version) (map[string]string, error) {
	requiredValues, err := a.bakery.GetOfferRequiredValues(sourceModelUUID, offerUUID)
	if err != nil {
		return nil, internalerrors.Errorf("getting required values for offer access: %w", err)
	}
	return a.checkMacaroons(ctx, mac, version, crossModelConsumeOp(offerUUID), requiredValues)
}

// CheckRelationMacaroons verifies that the specified macaroons allow access to the relation.
func (a *Authenticator) CheckRelationMacaroons(ctx context.Context, sourceModelUUID, offerUUID string, relationTag names.Tag, mac macaroon.Slice, version bakery.Version) error {
	requiredValues, err := a.bakery.GetRelationRequiredValues(sourceModelUUID, offerUUID, relationTag.Id())
	if err != nil {
		return internalerrors.Errorf("getting required values for relation access: %w", err)
	}
	_, err = a.checkMacaroons(ctx, mac, version, crossModelRelateOp(relationTag.Id()), requiredValues)
	return err
}

func (a *Authenticator) checkMacaroons(
	ctx context.Context,
	mac macaroon.Slice,
	version bakery.Version, op bakery.Op,
	requiredValues map[string]string,
) (map[string]string, error) {
	if a.logger.IsLevelEnabled(logger.DEBUG) {
		debugInfo := macaroonDebug(mac)
		a.logger.Debugf(ctx, "check %d macaroons with required attrs: %v\n%v", len(mac), requiredValues, debugInfo)
	}

	declared := a.bakery.InferDeclaredFromMacaroon(mac, requiredValues)
	a.logger.Debugf(ctx, "check macaroons with declared attrs: %v", declared)

	username, ok := declared.UserName()
	if !ok {
		return nil, apiservererrors.ErrPerm
	}

	conditions, err := a.bakery.AllowedAuth(ctx, op, mac)
	if err == nil && len(conditions) > 0 {
		if err = a.checkMacaroonCaveats(op, declared); err == nil {
			a.logger.Debugf(ctx, "ok macaroon check ok, attr: %v, conditions: %v", declared, conditions)
			return declared.AsMap(), nil
		}
		var target *bakery.VerificationError
		if errors.As(err, &target) {
			a.logger.Tracef(ctx, "macaroon verification error: %v", target)
			return nil, apiservererrors.ErrPerm
		}
	} else if err == nil {
		// There are no conditions, so the macaroon is not valid.
		a.logger.Tracef(ctx, "no macaroon conditions")
		err = errors.New("invalid cmd macaroon")
	}

	cause := err
	a.logger.Debugf(ctx, "generating discharge macaroon because: %v", cause)

	m, err := a.bakery.CreateDischargeMacaroon(ctx, username, requiredValues, declared, op, version)
	if err != nil {
		a.logger.Errorf(ctx, "cannot create cross model macaroon: %v", err)
		return nil, internalerrors.Errorf("creating discharge macaroon: %w", err)
	}

	return nil, &apiservererrors.DischargeRequiredError{
		Cause:          cause,
		Macaroon:       m,
		LegacyMacaroon: m.M(),
	}
}

func (a *Authenticator) checkMacaroonCaveats(op bakery.Op, declared crossmodelbakery.DeclaredValues) error {
	var entity string

	switch op.Action {
	case consumeOp:
		if declared.SourceModelUUID() == "" {
			return &bakery.VerificationError{Reason: errors.New("missing source model UUID")}
		}
		offerUUID := declared.OfferUUID()
		if offerUUID == "" {
			return &bakery.VerificationError{Reason: errors.New("missing offer UUID")}
		}
		entity = offerUUID

	case relateOp:
		relationKey := declared.RelationKey()
		if relationKey == "" {
			return &bakery.VerificationError{Reason: errors.New("missing relation")}
		}
		entity = relationKey

	default:
		return &bakery.VerificationError{Reason: errors.Errorf("invalid action %q", op.Action)}
	}

	if entity != op.Entity {
		return errors.Unauthorizedf("cmr operation %v not allowed for %v", op.Action, entity)
	}
	return nil
}

func macaroonDebug(slice macaroon.Slice) string {
	builder := new(strings.Builder)
	for _, mac := range slice {
		if mac == nil {
			continue
		}
		builder.WriteString(" - macaroon: ")
		builder.WriteString(string(mac.Id()))
		builder.WriteString("\n")
	}
	return builder.String()
}
