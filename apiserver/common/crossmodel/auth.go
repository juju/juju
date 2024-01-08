// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coremacaroon "github.com/juju/juju/core/macaroon"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
)

const (
	usernameKey    = "username"
	offeruuidKey   = "offer-uuid"
	sourcemodelKey = "source-model-uuid"
	relationKey    = "relation-key"

	offerPermissionCaveat = "has-offer-permission"

	// localOfferPermissionExpiryTime is used to expire offer macaroons.
	// It should be long enough to allow machines hosting workloads to
	// be provisioned so that the macaroon is still valid when the macaroon
	// is next used. If a machine takes longer, that's ok, a new discharge
	// will be obtained.
	localOfferPermissionExpiryTime = 3 * time.Minute
)

// RelationInfoFromMacaroons returns any relation and offer in the macaroons' declared caveats.
func RelationInfoFromMacaroons(mac macaroon.Slice) (string, string, bool) {
	declared := checkers.InferDeclared(coremacaroon.MacaroonNamespace, mac)
	relKey, ok1 := declared[relationKey]
	offerUUID, ok2 := declared[offeruuidKey]
	return relKey, offerUUID, ok1 && ok2
}

// CrossModelAuthorizer authorises any cmr operation presented to it.
type CrossModelAuthorizer struct{}

// AuthorizeOps implements OpsAuthorizer.AuthorizeOps.
func (CrossModelAuthorizer) AuthorizeOps(ctx context.Context, authorizedOp bakery.Op, queryOps []bakery.Op) ([]bool, []checkers.Caveat, error) {
	authlogger.Debugf("authorize cmr query ops check for %#v: %#v", authorizedOp, queryOps)
	allowed := make([]bool, len(queryOps))
	for i := range allowed {
		allowed[i] = queryOps[i].Action == consumeOp || queryOps[i].Action == relateOp
	}
	return allowed, nil, nil
}

// AuthContext is used to validate macaroons used to access
// application offers.
type AuthContext struct {
	systemState Backend

	clock              clock.Clock
	offerThirdPartyKey *bakery.KeyPair
	offerBakery        authentication.ExpirableStorageBakery

	offerAccessEndpoint string
}

// NewAuthContext creates a new authentication context for checking
// macaroons used with application offer requests.
func NewAuthContext(
	systemState Backend,
	offerThirdPartyKey *bakery.KeyPair,
	offerBakery authentication.ExpirableStorageBakery,
) (*AuthContext, error) {
	ctxt := &AuthContext{
		systemState:        systemState,
		clock:              clock.WallClock,
		offerBakery:        offerBakery,
		offerThirdPartyKey: offerThirdPartyKey,
	}
	return ctxt, nil
}

// WithClock creates a new authentication context
// using the specified clock.
func (a *AuthContext) WithClock(clock clock.Clock) *AuthContext {
	ctxtCopy := *a
	ctxtCopy.clock = clock
	return &ctxtCopy
}

// WithDischargeURL create an auth context based on this context and used
// to perform third party discharges at the specified URL.
func (a *AuthContext) WithDischargeURL(offerAccessEndpoint string) *AuthContext {
	ctxtCopy := *a
	ctxtCopy.offerAccessEndpoint = offerAccessEndpoint
	return &ctxtCopy
}

// OfferThirdPartyKey returns the key used to discharge offer macaroons.
func (a *AuthContext) OfferThirdPartyKey() *bakery.KeyPair {
	return a.offerThirdPartyKey
}

// CheckOfferAccessCaveat checks that the specified caveat required to be satisfied
// to gain access to an offer is valid, and returns the attributes return to check
// that the caveat is satisfied.
func (a *AuthContext) CheckOfferAccessCaveat(caveat string) (*offerPermissionCheck, error) {
	op, rest, err := checkers.ParseCaveat(caveat)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot parse caveat %q", caveat)
	}
	if op != offerPermissionCaveat {
		return nil, checkers.ErrCaveatNotRecognized
	}
	var details offerPermissionCheck
	err = yaml.Unmarshal([]byte(rest), &details)
	if err != nil {
		return nil, errors.Trace(err)
	}
	authlogger.Debugf("offer access caveat details: %+v", details)
	if !names.IsValidModel(details.SourceModelUUID) {
		return nil, errors.NotValidf("source-model-uuid %q", details.SourceModelUUID)
	}
	if !names.IsValidUser(details.User) {
		return nil, errors.NotValidf("username %q", details.User)
	}
	if err := permission.ValidateOfferAccess(permission.Access(details.Permission)); err != nil {
		return nil, errors.NotValidf("permission %q", details.Permission)
	}
	return &details, nil
}

// CheckLocalAccessRequest checks that the user in the specified permission
// check details has consume access to the offer in the details.
// It returns an error with a *bakery.VerificationError cause if the macaroon
// verification failed. If the macaroon is valid, CheckLocalAccessRequest
// returns a list of caveats to add to the discharge macaroon.
func (a *AuthContext) CheckLocalAccessRequest(details *offerPermissionCheck) ([]checkers.Caveat, error) {
	authlogger.Debugf("authenticate local offer access: %+v", details)
	if err := a.checkOfferAccess(a.systemState.UserPermission, details.User, details.OfferUUID); err != nil {
		return nil, errors.Trace(err)
	}

	firstPartyCaveats := []checkers.Caveat{
		checkers.DeclaredCaveat(sourcemodelKey, details.SourceModelUUID),
		checkers.DeclaredCaveat(offeruuidKey, details.OfferUUID),
		checkers.DeclaredCaveat(usernameKey, details.User),
		checkers.TimeBeforeCaveat(a.clock.Now().Add(localOfferPermissionExpiryTime)),
	}
	if details.Relation != "" {
		firstPartyCaveats = append(firstPartyCaveats, checkers.DeclaredCaveat(relationKey, details.Relation))
	}
	return firstPartyCaveats, nil
}

type userAccessFunc func(names.UserTag, names.Tag) (permission.Access, error)

func (a *AuthContext) checkOfferAccess(userAccess userAccessFunc, username, offerUUID string) error {
	userTag := names.NewUserTag(username)
	isAdmin, err := a.hasAccess(userAccess, userTag, permission.SuperuserAccess, a.systemState.ControllerTag())
	if is := errors.Is(err, authentication.ErrorEntityMissingPermission); err != nil && !is {
		return apiservererrors.ErrPerm
	}
	if isAdmin {
		return nil
	}
	isAdmin, err = a.hasAccess(userAccess, userTag, permission.AdminAccess, a.systemState.ModelTag())
	if is := errors.Is(err, authentication.ErrorEntityMissingPermission); err != nil && !is {
		return apiservererrors.ErrPerm
	}
	if isAdmin {
		return nil
	}
	isConsume, err := a.hasAccess(userAccess, userTag, permission.ConsumeAccess, names.NewApplicationOfferTag(offerUUID))
	if is := errors.Is(err, authentication.ErrorEntityMissingPermission); err != nil && !is {
		return err
	}
	if err != nil {
		return err
	} else if !isConsume {
		return apiservererrors.ErrPerm
	}
	return nil
}

func (a *AuthContext) hasAccess(userAccess func(names.UserTag, names.Tag) (permission.Access, error), userTag names.UserTag, access permission.Access, target names.Tag) (bool, error) {
	has, err := common.HasPermission(userAccess, userTag, access, target)
	if errors.Is(err, errors.NotFound) {
		return false, nil
	}
	return has, err
}

func (a *AuthContext) offerPermissionYaml(sourceModelUUID, username, offerURL, relationKey string, permission permission.Access) (string, error) {
	out, err := yaml.Marshal(offerPermissionCheck{
		SourceModelUUID: sourceModelUUID,
		User:            username,
		OfferUUID:       offerURL,
		Relation:        relationKey,
		Permission:      string(permission),
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// CreateConsumeOfferMacaroon creates a macaroon that authorises access to the specified offer.
func (a *AuthContext) CreateConsumeOfferMacaroon(ctx context.Context, offer *params.ApplicationOfferDetails, username string, version bakery.Version) (*bakery.Macaroon, error) {
	sourceModelTag, err := names.ParseModelTag(offer.SourceModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	expiryTime := a.clock.Now().Add(localOfferPermissionExpiryTime)
	bakery, err := a.offerBakery.ExpireStorageAfter(localOfferPermissionExpiryTime)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return bakery.NewMacaroon(
		ctx,
		version,
		[]checkers.Caveat{
			checkers.TimeBeforeCaveat(expiryTime),
			checkers.DeclaredCaveat(sourcemodelKey, sourceModelTag.Id()),
			checkers.DeclaredCaveat(offeruuidKey, offer.OfferUUID),
			checkers.DeclaredCaveat(usernameKey, username),
		}, crossModelConsumeOp(offer.OfferUUID))
}

// CreateRemoteRelationMacaroon creates a macaroon that authorises access to the specified relation.
func (a *AuthContext) CreateRemoteRelationMacaroon(ctx context.Context, sourceModelUUID, offerUUID string, username string, rel names.Tag, version bakery.Version) (*bakery.Macaroon, error) {
	expiryTime := a.clock.Now().Add(localOfferPermissionExpiryTime)
	bakery, err := a.offerBakery.ExpireStorageAfter(localOfferPermissionExpiryTime)
	if err != nil {
		return nil, errors.Trace(err)
	}

	offerMacaroon, err := bakery.NewMacaroon(
		ctx,
		version,
		[]checkers.Caveat{
			checkers.TimeBeforeCaveat(expiryTime),
			checkers.DeclaredCaveat(sourcemodelKey, sourceModelUUID),
			checkers.DeclaredCaveat(offeruuidKey, offerUUID),
			checkers.DeclaredCaveat(usernameKey, username),
			checkers.DeclaredCaveat(relationKey, rel.Id()),
		}, crossModelRelateOp(rel.Id()))

	return offerMacaroon, err
}

type offerPermissionCheck struct {
	SourceModelUUID string `yaml:"source-model-uuid"`
	User            string `yaml:"username"`
	OfferUUID       string `yaml:"offer-uuid"`
	Relation        string `yaml:"relation-key"`
	Permission      string `yaml:"permission"`
}

type authenticator struct {
	clock  clock.Clock
	bakery authentication.ExpirableStorageBakery
	ctxt   *AuthContext

	// offerAccessEndpoint holds the URL of the trusted third party
	// that is used to address the has-offer-permission third party caveat.
	offerAccessEndpoint string
}

const (
	consumeOp = "consume"
	relateOp  = "relate"
)

func crossModelConsumeOp(offerUUID string) bakery.Op {
	return bakery.Op{
		Entity: offerUUID,
		Action: consumeOp,
	}
}

func crossModelRelateOp(relationID string) bakery.Op {
	return bakery.Op{
		Entity: relationID,
		Action: relateOp,
	}
}

// Authenticator returns an instance used to authenticate macaroons used to access offers.
func (a *AuthContext) Authenticator() *authenticator {
	auth := &authenticator{
		clock:               a.clock,
		bakery:              a.offerBakery,
		ctxt:                a,
		offerAccessEndpoint: a.offerAccessEndpoint,
	}
	return auth
}

func (a *authenticator) checkMacaroonCaveats(op bakery.Op, relationId, sourceModelUUID, offerUUID string) error {
	var entity string
	switch op.Action {
	case consumeOp:
		if sourceModelUUID == "" {
			return &bakery.VerificationError{Reason: errors.New("missing source model UUID")}
		}
		if offerUUID == "" {
			return &bakery.VerificationError{Reason: errors.New("missing offer UUID")}
		}
		entity = offerUUID
	case relateOp:
		if relationId == "" {
			return &bakery.VerificationError{Reason: errors.New("missing relation")}
		}
		entity = relationId
	default:
		return &bakery.VerificationError{Reason: errors.Errorf("invalid action %q", op.Action)}
	}
	if entity != op.Entity {
		return errors.Unauthorizedf("cmr operation %v not allowed for %v", op.Action, entity)
	}
	return nil
}

func (a *authenticator) checkMacaroons(
	ctx context.Context, mac macaroon.Slice, version bakery.Version, requiredValues map[string]string, op bakery.Op,
) (map[string]string, error) {
	authlogger.Debugf("check %d macaroons with required attrs: %v", len(mac), requiredValues)
	for _, m := range mac {
		if m == nil {
			authlogger.Warningf("unexpected nil cross model macaroon")
			continue
		}
		authlogger.Debugf("- mac %s", m.Id())
	}
	declared := checkers.InferDeclared(coremacaroon.MacaroonNamespace, mac)
	authlogger.Debugf("check macaroons with declared attrs: %v", declared)
	username, ok := declared[usernameKey]
	if !ok {
		return nil, apiservererrors.ErrPerm
	}
	relation := declared[relationKey]
	sourceModelUUID := declared[sourcemodelKey]
	offerUUID := declared[offeruuidKey]

	auth := a.bakery.Auth(ctx, mac)
	ai, err := auth.Allow(ctx, op)
	if err == nil && len(ai.Conditions()) > 0 {
		if err = a.checkMacaroonCaveats(op, relation, sourceModelUUID, offerUUID); err == nil {
			authlogger.Debugf("ok macaroon check ok, attr: %v, conditions: %v", declared, ai.Conditions())
			return declared, nil
		}
		if _, ok := err.(*bakery.VerificationError); !ok {
			return nil, apiservererrors.ErrPerm
		}
	}

	cause := err
	if cause == nil {
		cause = errors.New("invalid cmr macaroon")
	}
	authlogger.Debugf("generating discharge macaroon because: %v", cause)

	requiredRelation := requiredValues[relationKey]
	requiredOffer := requiredValues[offeruuidKey]
	requiredSourceModelUUID := requiredValues[sourcemodelKey]
	authYaml, err := a.ctxt.offerPermissionYaml(requiredSourceModelUUID, username, requiredOffer, requiredRelation, permission.ConsumeAccess)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bakery, err := a.bakery.ExpireStorageAfter(localOfferPermissionExpiryTime)
	if err != nil {
		return nil, errors.Trace(err)
	}
	keys := []string{usernameKey}
	for k := range requiredValues {
		keys = append(keys, k)
	}
	m, err := bakery.NewMacaroon(
		ctx,
		version,
		[]checkers.Caveat{
			checkers.NeedDeclaredCaveat(
				checkers.Caveat{
					Location:  a.offerAccessEndpoint,
					Condition: offerPermissionCaveat + " " + authYaml,
				},
				keys...,
			),
			checkers.TimeBeforeCaveat(a.clock.Now().Add(localOfferPermissionExpiryTime)),
		}, op)

	if err != nil {
		err = errors.Annotate(err, "cannot create macaroon")
		authlogger.Errorf("cannot create cross model macaroon: %v", err)
		return nil, err
	}
	return nil, &apiservererrors.DischargeRequiredError{
		Cause:          cause,
		Macaroon:       m,
		LegacyMacaroon: m.M(),
	}
}

// CheckOfferMacaroons verifies that the specified macaroons allow access to the offer.
func (a *authenticator) CheckOfferMacaroons(ctx context.Context, sourceModelUUID, offerUUID string, mac macaroon.Slice, version bakery.Version) (map[string]string, error) {
	requiredValues := map[string]string{
		sourcemodelKey: sourceModelUUID,
		offeruuidKey:   offerUUID,
	}
	return a.checkMacaroons(ctx, mac, version, requiredValues, crossModelConsumeOp(offerUUID))
}

// CheckRelationMacaroons verifies that the specified macaroons allow access to the relation.
func (a *authenticator) CheckRelationMacaroons(ctx context.Context, sourceModelUUID, offerUUID string, relationTag names.Tag, mac macaroon.Slice, version bakery.Version) error {
	requiredValues := map[string]string{
		sourcemodelKey: sourceModelUUID,
		offeruuidKey:   offerUUID,
		relationKey:    relationTag.Id(),
	}
	_, err := a.checkMacaroons(ctx, mac, version, requiredValues, crossModelRelateOp(relationTag.Id()))
	return err
}
