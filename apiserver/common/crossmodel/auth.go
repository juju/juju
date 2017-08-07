// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/permission"
)

const (
	usernameKey    = "username"
	offerurlKey    = "offer-url"
	sourcemodelKey = "source-model-uuid"
	relationKey    = "relation-key"

	offerPermissionCaveat          = "has-offer-permission"
	localOfferPermissionExpiryTime = 2 * time.Minute
)

// AuthContext is used to validate macaroons used to access
// application offers.
type AuthContext struct {
	pool StatePool

	clock                             clock.Clock
	localOfferThirdPartyBakeryService authentication.BakeryService
	localOfferBakeryService           authentication.ExpirableStorageBakeryService

	offerAccessEndpoint string
}

// NewAuthContext creates a new authentication context for checking
// macaroons used with application offer requests.
func NewAuthContext(
	pool StatePool,
	localOfferThirdPartyBakeryService authentication.BakeryService,
	localOfferBakeryService authentication.ExpirableStorageBakeryService,
) (*AuthContext, error) {
	ctxt := &AuthContext{
		pool:  pool,
		clock: clock.WallClock,
		localOfferBakeryService:           localOfferBakeryService,
		localOfferThirdPartyBakeryService: localOfferThirdPartyBakeryService,
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

// ThirdPartyBakeryService returns the third party bakery service.
func (a *AuthContext) ThirdPartyBakeryService() authentication.BakeryService {
	return a.localOfferThirdPartyBakeryService
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
	logger.Debugf("offer access caveat details: %+v", details)
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
	logger.Debugf("authenticate local offer access: %+v", details)
	st, releaser, err := a.pool.Get(details.SourceModelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer releaser()
	if err := a.checkOfferAccess(st, details.User, details.Offer); err != nil {
		return nil, errors.Trace(err)
	}

	firstPartyCaveats := []checkers.Caveat{
		checkers.DeclaredCaveat(sourcemodelKey, details.SourceModelUUID),
		checkers.DeclaredCaveat(offerurlKey, details.Offer),
		checkers.DeclaredCaveat(usernameKey, details.User),
		checkers.TimeBeforeCaveat(a.clock.Now().Add(localOfferPermissionExpiryTime)),
	}
	if details.Relation != "" {
		firstPartyCaveats = append(firstPartyCaveats, checkers.DeclaredCaveat(relationKey, details.Relation))
	}
	return firstPartyCaveats, nil
}

func (a *AuthContext) checkOfferAccess(st Backend, username, offerName string) error {
	userTag := names.NewUserTag(username)
	isAdmin, err := a.hasControllerAdminAccess(st, userTag)
	if err != nil {
		return common.ErrPerm
	}
	if isAdmin {
		return nil
	}
	isAdmin, err = a.hasModelAdminAccess(st, userTag)
	if err != nil {
		return common.ErrPerm
	}
	if isAdmin {
		return nil
	}
	access, err := st.GetOfferAccess(names.NewApplicationOfferTag(offerName), userTag)
	if err != nil && !errors.IsNotFound(err) {
		return common.ErrPerm
	}
	if !access.EqualOrGreaterOfferAccessThan(permission.ConsumeAccess) {
		return common.ErrPerm
	}
	return nil
}

func (api *AuthContext) hasControllerAdminAccess(st Backend, userTag names.UserTag) (bool, error) {
	isAdmin, err := common.HasPermission(st.UserPermission, userTag, permission.SuperuserAccess, st.ControllerTag())
	if errors.IsNotFound(err) {
		return false, nil
	}
	return isAdmin, err
}

func (api *AuthContext) hasModelAdminAccess(st Backend, userTag names.UserTag) (bool, error) {
	isAdmin, err := common.HasPermission(st.UserPermission, userTag, permission.AdminAccess, st.ModelTag())
	if errors.IsNotFound(err) {
		return false, nil
	}
	return isAdmin, err
}

func (a *AuthContext) offerPermissionYaml(sourceModelUUID, username, offerURL, relationKey string, permission permission.Access) (string, error) {
	out, err := yaml.Marshal(offerPermissionCheck{
		SourceModelUUID: sourceModelUUID,
		User:            username,
		Offer:           offerURL,
		Relation:        relationKey,
		Permission:      string(permission),
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// CreateConsumeOfferMacaroon creates a macaroon that authorises access to the specified offer.
func (a *AuthContext) CreateConsumeOfferMacaroon(offer *params.ApplicationOffer, username string) (*macaroon.Macaroon, error) {
	sourceModelTag, err := names.ParseModelTag(offer.SourceModelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	expiryTime := a.clock.Now().Add(localOfferPermissionExpiryTime)
	bakery, err := a.localOfferBakeryService.ExpireStorageAt(expiryTime)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return bakery.NewMacaroon("", nil,
		[]checkers.Caveat{
			checkers.TimeBeforeCaveat(expiryTime),
			checkers.DeclaredCaveat(sourcemodelKey, sourceModelTag.Id()),
			checkers.DeclaredCaveat(offerurlKey, offer.OfferURL),
			checkers.DeclaredCaveat(usernameKey, username),
		})
}

// CreateRemoteRelationMacaroon creates a macaroon that authorises access to the specified relation.
func (a *AuthContext) CreateRemoteRelationMacaroon(sourceModelUUID, offerURL string, username string, rel names.Tag) (*macaroon.Macaroon, error) {
	expiryTime := a.clock.Now().Add(localOfferPermissionExpiryTime)
	bakery, err := a.localOfferBakeryService.ExpireStorageAt(expiryTime)
	if err != nil {
		return nil, errors.Trace(err)
	}

	offerMacaroon, err := bakery.NewMacaroon("", nil,
		[]checkers.Caveat{
			checkers.TimeBeforeCaveat(expiryTime),
			checkers.DeclaredCaveat(sourcemodelKey, sourceModelUUID),
			checkers.DeclaredCaveat(offerurlKey, offerURL),
			checkers.DeclaredCaveat(usernameKey, username),
			checkers.DeclaredCaveat(relationKey, rel.Id()),
		})
	return offerMacaroon, err
}

type offerPermissionCheck struct {
	SourceModelUUID string `yaml:"source-model-uuid"`
	User            string `yaml:"username"`
	Offer           string `yaml:"offer-url"`
	Relation        string `yaml:"relation-key"`
	Permission      string `yaml:"permission"`
}

type authenticator struct {
	clock  clock.Clock
	bakery authentication.ExpirableStorageBakeryService
	ctxt   *AuthContext

	sourceModelUUID string
	offerURL        string

	// offerAccessEndpoint holds the URL of the trusted third party
	// that is used to address the has-offer-permission third party caveat.
	offerAccessEndpoint string
}

// Authenticator returns an instance used to authenticate macaroons used to
// access the spcified offer.
func (a *AuthContext) Authenticator(sourceModelUUID, offerURL string) *authenticator {
	auth := &authenticator{
		clock:               a.clock,
		bakery:              a.localOfferBakeryService,
		ctxt:                a,
		sourceModelUUID:     sourceModelUUID,
		offerURL:            offerURL,
		offerAccessEndpoint: a.offerAccessEndpoint,
	}
	return auth
}

func (a *authenticator) checkMacaroons(mac macaroon.Slice, requiredValues map[string]string) (map[string]string, error) {
	logger.Debugf("check macaroons with required attrs: %v", requiredValues)
	for _, m := range mac {
		logger.Debugf("- mac %s", m.Id())
	}
	declared := checkers.InferDeclared(mac)
	logger.Debugf("check macaroons with declared attrs: %v", declared)
	username, ok := declared[usernameKey]
	if !ok {
		return nil, common.ErrPerm
	}
	relation := declared[relationKey]
	attrs, err := a.bakery.CheckAny([]macaroon.Slice{mac}, requiredValues, checkers.TimeBefore)
	if err == nil {
		logger.Debugf("macaroon check ok, attr: %v", attrs)
		return attrs, nil
	}

	if _, ok := errgo.Cause(err).(*bakery.VerificationError); !ok {
		logger.Debugf("macaroon verification failed: %+v", err)
		return nil, common.ErrPerm
	}

	logger.Debugf("generating discharge macaroon because: %v", err)
	cause := err
	authYaml, err := a.ctxt.offerPermissionYaml(a.sourceModelUUID, username, a.offerURL, relation, permission.ConsumeAccess)
	if err != nil {
		return nil, errors.Trace(err)
	}
	expiryTime := a.ctxt.clock.Now().Add(localOfferPermissionExpiryTime)
	bakery, err := a.bakery.ExpireStorageAt(expiryTime)
	if err != nil {
		return nil, errors.Trace(err)
	}
	keys := []string{usernameKey}
	for k := range requiredValues {
		keys = append(keys, k)
	}
	m, err := bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.NeedDeclaredCaveat(
			checkers.Caveat{
				Location:  a.offerAccessEndpoint,
				Condition: offerPermissionCaveat + " " + authYaml,
			},
			keys...,
		),
		checkers.TimeBeforeCaveat(a.clock.Now().Add(localOfferPermissionExpiryTime)),
	})
	if err != nil {
		return nil, errors.Annotate(err, "cannot create macaroon")
	}
	return nil, &common.DischargeRequiredError{
		Cause:    cause,
		Macaroon: m,
	}
}

// CheckOfferMacaroons verifies that the specified macaroons allow access to the offer.
func (a *authenticator) CheckOfferMacaroons(offerURL string, mac macaroon.Slice) (map[string]string, error) {
	requiredValues := map[string]string{
		sourcemodelKey: a.sourceModelUUID,
		offerurlKey:    offerURL,
	}
	return a.checkMacaroons(mac, requiredValues)
}

// CheckRelationMacaroons verifies that the specified macaroons allow access to the relation.
func (a *authenticator) CheckRelationMacaroons(relationTag names.Tag, mac macaroon.Slice) error {
	requiredValues := map[string]string{
		sourcemodelKey: a.sourceModelUUID,
		relationKey:    relationTag.Id(),
	}
	_, err := a.checkMacaroons(mac, requiredValues)
	return err
}
