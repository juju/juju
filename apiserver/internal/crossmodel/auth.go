// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	internalerrors "github.com/juju/juju/internal/errors"
)

const (
	usernameKey    = "username"
	offerUUIDKey   = "offer-uuid"
	sourceModelKey = "source-model-uuid"
	relationKey    = "relation-key"

	offerPermissionCaveat = "has-offer-permission"

	// offerPermissionExpiryTime is used to expire offer macaroons.
	// It should be long enough to allow machines hosting workloads to
	// be provisioned so that the macaroon is still valid when the macaroon
	// is next used. If a machine takes longer, that's ok, a new discharge
	// will be obtained.
	offerPermissionExpiryTime = 3 * time.Minute
)

// AccessService validates access for user permissions.
type AccessService interface {
	// ReadUserAccessLevelForTarget returns the user access level for the
	// given user on the given target.
	ReadUserAccessLevelForTarget(ctx context.Context, subject user.Name, target permission.ID) (permission.Access, error)
}

// OfferAccessDetails represents the details encoded in an offer permission
// caveat.
type OfferAccessDetails struct {
	SourceModelUUID string
	User            string
	OfferUUID       string
	Relation        string
	Permission      string
}

// AuthContext is used to validate macaroons used to access
// application offers.
type AuthContext struct {
	accessService AccessService

	offerThirdPartyKey *bakery.KeyPair

	controllerTag names.ControllerTag
	modelTag      names.ModelTag

	clock  clock.Clock
	logger logger.Logger
}

// NewAuthContext creates a new authentication context for checking
// macaroons used with application offer requests.
func NewAuthContext(
	accessService AccessService,
	offerThirdPartyKey *bakery.KeyPair,
	controllerUUID string,
	modelUUID model.UUID,
	clock clock.Clock,
	logger logger.Logger,
) *AuthContext {
	return &AuthContext{
		accessService:      accessService,
		offerThirdPartyKey: offerThirdPartyKey,
		controllerTag:      names.NewControllerTag(controllerUUID),
		modelTag:           names.NewModelTag(modelUUID.String()),
		clock:              clock,
		logger:             logger,
	}
}

// OfferThirdPartyKey returns the key used to discharge offer macaroons.
func (a *AuthContext) OfferThirdPartyKey() *bakery.KeyPair {
	return a.offerThirdPartyKey
}

// CheckOfferAccessCaveat checks that the specified caveat required to be satisfied
// to gain access to an offer is valid, and returns the attributes return to check
// that the caveat is satisfied.
func (a *AuthContext) CheckOfferAccessCaveat(ctx context.Context, caveat string) (OfferAccessDetails, error) {
	op, rest, err := checkers.ParseCaveat(caveat)
	if err != nil {
		return OfferAccessDetails{}, errors.Annotatef(err, "cannot parse caveat %q", caveat)
	}
	if op != offerPermissionCaveat {
		return OfferAccessDetails{}, checkers.ErrCaveatNotRecognized
	}

	type offerAccessDetails struct {
		SourceModelUUID string `yaml:"source-model-uuid"`
		User            string `yaml:"username"`
		OfferUUID       string `yaml:"offer-uuid"`
		Relation        string `yaml:"relation-key"`
		Permission      string `yaml:"permission"`
	}

	var details offerAccessDetails
	err = yaml.Unmarshal([]byte(rest), &details)
	if err != nil {
		return OfferAccessDetails{}, internalerrors.Errorf("unmarshalling offer access caveat details: %w", err).Add(coreerrors.NotValid)
	}

	a.logger.Debugf(ctx, "offer access caveat details: %+v", details)
	if !names.IsValidModel(details.SourceModelUUID) {
		return OfferAccessDetails{}, errors.NotValidf("source-model-uuid %q", details.SourceModelUUID)
	}
	if !names.IsValidUser(details.User) {
		return OfferAccessDetails{}, errors.NotValidf("username %q", details.User)
	}
	if err := permission.ValidateOfferAccess(permission.Access(details.Permission)); err != nil {
		return OfferAccessDetails{}, errors.NotValidf("permission %q", details.Permission)
	}
	return OfferAccessDetails(details), nil
}

// CheckLocalAccessRequest checks that the user in the specified permission
// check details has consume access to the offer in the details.
// It returns an error with a *bakery.VerificationError cause if the macaroon
// verification failed. If the macaroon is valid, CheckLocalAccessRequest
// returns a list of caveats to add to the discharge macaroon.
func (a *AuthContext) CheckLocalAccessRequest(ctx context.Context, details OfferAccessDetails) ([]checkers.Caveat, error) {
	a.logger.Debugf(ctx, "authenticate local offer access: %+v", details)
	if err := a.checkOfferAccess(ctx, a.hasUserOfferPermission, details.User, details.OfferUUID); err != nil {
		return nil, errors.Trace(err)
	}

	firstPartyCaveats := []checkers.Caveat{
		checkers.DeclaredCaveat(sourceModelKey, details.SourceModelUUID),
		checkers.DeclaredCaveat(offerUUIDKey, details.OfferUUID),
		checkers.DeclaredCaveat(usernameKey, details.User),
		checkers.TimeBeforeCaveat(a.clock.Now().Add(offerPermissionExpiryTime)),
	}
	if details.Relation != "" {
		firstPartyCaveats = append(firstPartyCaveats, checkers.DeclaredCaveat(relationKey, details.Relation))
	}
	return firstPartyCaveats, nil
}

func (a *AuthContext) hasUserOfferPermission(ctx context.Context, userName user.Name, target permission.ID) (permission.Access, error) {
	if target.ObjectType == permission.Cloud {
		return "", errors.NotValidf("target %q", target.ObjectType)
	}

	access, err := a.accessService.ReadUserAccessLevelForTarget(ctx, userName, target)
	return access, errors.Trace(err)
}

func (a *AuthContext) checkOfferAccess(ctx context.Context, userAccess common.UserAccessFunc, username, offerUUID string) error {
	userTag := names.NewUserTag(username)

	isAdmin, err := hasAccess(ctx, userAccess, userTag, permission.SuperuserAccess, a.controllerTag)
	if is := errors.Is(err, authentication.ErrorEntityMissingPermission); err != nil && !is {
		return apiservererrors.ErrPerm
	} else if isAdmin {
		return nil
	}

	isAdmin, err = hasAccess(ctx, userAccess, userTag, permission.AdminAccess, a.modelTag)
	if is := errors.Is(err, authentication.ErrorEntityMissingPermission); err != nil && !is {
		return apiservererrors.ErrPerm
	} else if isAdmin {
		return nil
	}

	isConsume, err := hasAccess(ctx, userAccess, userTag, permission.ConsumeAccess, names.NewApplicationOfferTag(offerUUID))
	if err != nil {
		return err
	} else if !isConsume {
		return apiservererrors.ErrPerm
	}
	return nil
}

func hasAccess(ctx context.Context, userAccess common.UserAccessFunc, userTag names.UserTag, access permission.Access, target names.Tag) (bool, error) {
	has, err := common.HasPermission(ctx, userAccess, userTag, access, target)
	if errors.Is(err, errors.NotFound) {
		return false, nil
	}
	return has, err
}
