// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/bakeryutil"
	coremacaroon "github.com/juju/juju/core/macaroon"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state/bakerystorage"
)

// OfferBakery is a bakery service for offer access.
type OfferBakery struct {
	clock clock.Clock

	bakery authentication.ExpirableStorageBakery
}

// OfferBakeryInterface is the interface that OfferBakery implements.
type OfferBakeryInterface interface {
	getClock() clock.Clock
	setClock(clock.Clock)
	getBakery() authentication.ExpirableStorageBakery

	RefreshDischargeURL(string) (string, error)
	GetConsumeOfferCaveats(offerUUID, sourceModelUUID, username string) []checkers.Caveat
	InferDeclaredFromMacaroon(macaroon.Slice, map[string]string) map[string]string
	CreateDischargeMacaroon(
		context.Context, string, string, map[string]string, map[string]string, bakery.Op, bakery.Version,
	) (*bakery.Macaroon, error)
}

func (o *OfferBakery) getClock() clock.Clock {
	return o.clock
}

func (o *OfferBakery) setClock(clock clock.Clock) {
	o.clock = clock
}

func (o *OfferBakery) getBakery() authentication.ExpirableStorageBakery {
	return o.bakery
}

// RefreshDischargeURL updates the discharge URL for the bakery service.
func (o *OfferBakery) RefreshDischargeURL(accessEndpoint string) (string, error) {
	return accessEndpoint, nil
}

// NewOfferBakeryForTest is for testing.
func NewOfferBakeryForTest(bakery authentication.ExpirableStorageBakery, clk clock.Clock) *OfferBakery {
	return &OfferBakery{bakery: bakery, clock: clk}
}

// NewLocalOfferBakery creates a new bakery service for local offer access.
func NewLocalOfferBakery(
	location string,
	bakeryConfig bakerystorage.BakeryConfig,
	store bakerystorage.ExpirableStorage,
	checker bakery.FirstPartyCaveatChecker,
) (*OfferBakery, error) {
	key, err := bakeryConfig.GetOffersThirdPartyKey()
	if err != nil {
		return nil, errors.Trace(err)
	}
	locator := bakeryutil.BakeryThirdPartyLocator{PublicKey: key.Public}
	localOfferBakery := bakery.New(
		bakery.BakeryParams{
			Checker:       checker,
			RootKeyStore:  store,
			Locator:       locator,
			Key:           key,
			OpsAuthorizer: CrossModelAuthorizer{},
			Location:      location,
		},
	)
	bakery := &bakeryutil.ExpirableStorageBakery{
		Bakery:   localOfferBakery,
		Location: location,
		Key:      key,
		Store:    store,
		Locator:  locator,
	}
	return &OfferBakery{bakery: bakery, clock: clock.WallClock}, nil
}

// JaaSOfferBakery is a bakery service for offer access.
type JaaSOfferBakery struct {
	*OfferBakery

	location               string
	currrentAccessEndpoint string
	bakeryConfig           bakerystorage.BakeryConfig
	store                  bakerystorage.ExpirableStorage
	checker                bakery.FirstPartyCaveatChecker
}

// RefreshDischargeURL updates the discharge URL for the bakery service.
func (o *JaaSOfferBakery) RefreshDischargeURL(accessEndpoint string) (string, error) {
	accessEndpoint, err := o.cleanDischargeURL(accessEndpoint)
	if err != nil {
		return "", errors.Trace(err)
	}
	if o.currrentAccessEndpoint == accessEndpoint {
		return accessEndpoint, nil
	}
	o.currrentAccessEndpoint = accessEndpoint
	return accessEndpoint, errors.Trace(o.refreshBakery(accessEndpoint))
}

func (o *JaaSOfferBakery) cleanDischargeURL(addr string) (string, error) {
	refreshURL, err := url.Parse(addr)
	if err != nil {
		return "", errors.Trace(err)
	}
	refreshURL.Path = "macaroons"
	return refreshURL.String(), nil
}

func (o *JaaSOfferBakery) refreshBakery(accessEndpoint string) (err error) {
	thirdPartyInfo, err := httpbakery.ThirdPartyInfoForLocation(
		context.TODO(), &http.Client{Transport: DefaultTransport}, accessEndpoint,
	)
	logger.Tracef("got third party info %#v from %q", thirdPartyInfo, accessEndpoint)
	if err != nil {
		return errors.Trace(err)
	}
	key, err := o.bakeryConfig.GetExternalUsersThirdPartyKey()
	if err != nil {
		return errors.Trace(err)
	}

	pkCache := bakery.NewThirdPartyStore()
	pkCache.AddInfo(accessEndpoint, thirdPartyInfo)
	locator := httpbakery.NewThirdPartyLocator(nil, pkCache)

	o.bakery = &bakeryutil.ExpirableStorageBakery{
		Bakery: bakery.New(
			bakery.BakeryParams{
				Checker:       o.checker,
				RootKeyStore:  o.store,
				Locator:       locator,
				Key:           key,
				OpsAuthorizer: CrossModelAuthorizer{},
				Location:      o.location,
			},
		),
		Location: o.location,
		Key:      key,
		Store:    o.store,
		Locator:  locator,
	}
	return nil
}

var (
	// Override for testing.
	DefaultTransport = http.DefaultTransport
)

// NewJaaSOfferBakery creates a new bakery service for JaaS offer access.
// Attempts to refresh the bakery information but won't fail on errors
// to prevent blocking controller startup.
func NewJaaSOfferBakery(
	loginTokenRefreshURL, location string,
	bakeryConfig bakerystorage.BakeryConfig,
	store bakerystorage.ExpirableStorage,
	checker bakery.FirstPartyCaveatChecker,
) (*JaaSOfferBakery, error) {
	offerBakery := &JaaSOfferBakery{
		location:     location,
		bakeryConfig: bakeryConfig,
		store:        store,
		checker:      checker,
		OfferBakery:  &OfferBakery{clock: clock.WallClock},
	}
	if _, err := offerBakery.RefreshDischargeURL(loginTokenRefreshURL); err != nil {
		logger.Errorf("refreshing jaas offer discharger at %q: %s", loginTokenRefreshURL, err)
	}
	return offerBakery, nil
}

// GetConsumeOfferCaveats returns the caveats for consuming an offer.
func (o *OfferBakery) GetConsumeOfferCaveats(offerUUID, sourceModelUUID, username string) []checkers.Caveat {
	return []checkers.Caveat{
		checkers.TimeBeforeCaveat(o.clock.Now().Add(offerPermissionExpiryTime)),
		checkers.DeclaredCaveat(sourcemodelKey, sourceModelUUID),
		checkers.DeclaredCaveat(usernameKey, username),
		checkers.DeclaredCaveat(offeruuidKey, offerUUID),
	}
}

// GetConsumeOfferCaveats returns the caveats for consuming an offer.
func (o *JaaSOfferBakery) GetConsumeOfferCaveats(offerUUID, sourceModelUUID, username string) []checkers.Caveat {
	// We do not declare the offer UUID here since we will discharge the
	// macaroon to JaaS to verify the offer access for JaaS flow.
	return []checkers.Caveat{
		checkers.TimeBeforeCaveat(o.clock.Now().Add(offerPermissionExpiryTime)),
		checkers.DeclaredCaveat(sourcemodelKey, sourceModelUUID),
		checkers.DeclaredCaveat(usernameKey, username),
	}
}

// InferDeclaredFromMacaroon returns the declared attributes from the macaroon.
func (o *OfferBakery) InferDeclaredFromMacaroon(mac macaroon.Slice, requiredValues map[string]string) map[string]string {
	return checkers.InferDeclared(coremacaroon.MacaroonNamespace, mac)
}

// InferDeclaredFromMacaroon returns the declared attributes from the macaroon.
func (o *JaaSOfferBakery) InferDeclaredFromMacaroon(mac macaroon.Slice, requiredValues map[string]string) map[string]string {
	declared := checkers.InferDeclared(coremacaroon.MacaroonNamespace, mac)
	authlogger.Debugf("check macaroons with declared attrs: %v", declared)
	// We only need to inject relationKey for jaas flow
	// because the relation key injected in juju discharge
	// process will not be injected in Jaas discharge endpoint.
	if _, ok := declared[relationKey]; !ok {
		if relation, ok := requiredValues[relationKey]; ok {
			declared[relationKey] = relation
		}
	}
	return declared
}

func localOfferPermissionYaml(sourceModelUUID, username, offerURL, relationKey string, permission permission.Access) (string, error) {
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

// CreateDischargeMacaroon creates a discharge macaroon.
func (o *OfferBakery) CreateDischargeMacaroon(
	ctx context.Context, accessEndpoint, username string,
	requiredValues, declaredValues map[string]string,
	op bakery.Op, version bakery.Version,
) (*bakery.Macaroon, error) {
	requiredSourceModelUUID := requiredValues[sourcemodelKey]
	requiredOffer := requiredValues[offeruuidKey]
	requiredRelation := requiredValues[relationKey]
	authYaml, err := localOfferPermissionYaml(
		requiredSourceModelUUID, username, requiredOffer, requiredRelation,
		permission.ConsumeAccess,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bakery, err := o.bakery.ExpireStorageAfter(offerPermissionExpiryTime)
	if err != nil {
		return nil, errors.Trace(err)
	}
	requiredKeys := []string{usernameKey}
	for k := range requiredValues {
		requiredKeys = append(requiredKeys, k)
	}
	sort.Strings(requiredKeys)
	return bakery.NewMacaroon(
		ctx,
		version,
		[]checkers.Caveat{
			checkers.NeedDeclaredCaveat(
				checkers.Caveat{
					Location:  accessEndpoint,
					Condition: offerPermissionCaveat + " " + authYaml,
				},
				requiredKeys...,
			),
			checkers.TimeBeforeCaveat(o.clock.Now().Add(offerPermissionExpiryTime)),
		}, op,
	)
}

// CreateDischargeMacaroon creates a discharge macaroon.
func (o *JaaSOfferBakery) CreateDischargeMacaroon(
	ctx context.Context, accessEndpoint, username string,
	requiredValues, declaredValues map[string]string,
	op bakery.Op, version bakery.Version,
) (*bakery.Macaroon, error) {
	requiredOffer := requiredValues[offeruuidKey]
	conditionParts := []string{
		"is-consumer", names.NewUserTag(username).String(), requiredOffer,
	}

	conditionCaveat := checkers.Caveat{
		Location:  accessEndpoint,
		Condition: strings.Join(conditionParts, " "),
	}
	var declaredCaveats []checkers.Caveat
	for k, v := range declaredValues {
		declaredCaveats = append(declaredCaveats, checkers.DeclaredCaveat(k, v))
	}
	bakery, err := o.bakery.ExpireStorageAfter(offerPermissionExpiryTime)
	if err != nil {
		return nil, errors.Trace(err)
	}
	macaroon, err := bakery.NewMacaroon(
		ctx,
		version,
		append(
			[]checkers.Caveat{
				conditionCaveat,
				checkers.TimeBeforeCaveat(o.clock.Now().Add(offerPermissionExpiryTime)),
			},
			declaredCaveats...,
		), op,
	)
	return macaroon, err
}
