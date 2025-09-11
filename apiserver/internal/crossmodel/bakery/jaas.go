// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakery

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/bakeryutil"
	"github.com/juju/juju/core/logger"
	internalerrors "github.com/juju/juju/internal/errors"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
)

// HTTPClient is implemented by HTTP client packages to make an HTTP request.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// JAASOfferBakery is a bakery for JAAS offer access.
type JAASOfferBakery struct {
	baseBakery
	oven     Oven
	endpoint string
	clock    clock.Clock
	logger   logger.Logger
}

// NewJAASOfferBakery returns a new JAASOfferBakery.
func NewJAASOfferBakery(
	keyPair *bakery.KeyPair,
	location, endpoint string,
	backingStore BakeryStore,
	checker bakery.FirstPartyCaveatChecker,
	authorizer bakery.OpsAuthorizer,
	httpClient HTTPClient,
	clock clock.Clock,
	logger logger.Logger,
) (*JAASOfferBakery, error) {
	store := internalmacaroon.NewRootKeyStore(backingStore, offerPermissionExpiryTime, clock)

	externalKeyLocator := newExternalPublicKeyLocator(endpoint, httpClient, logger)
	bakery := &bakeryutil.StorageBakery{
		Bakery: bakery.New(bakery.BakeryParams{
			Location:      location,
			Locator:       externalKeyLocator,
			RootKeyStore:  store,
			Checker:       checker,
			OpsAuthorizer: authorizer,
		}),
	}

	return &JAASOfferBakery{
		oven:     bakery,
		endpoint: endpoint,
		clock:    clock,
		logger:   logger,
	}, nil
}

// GetConsumeOfferCaveats returns the caveats for consuming an offer.
func (o *JAASOfferBakery) GetConsumeOfferCaveats(offerUUID, sourceModelUUID, username, relation string) []checkers.Caveat {
	// We do not declare the offer UUID here since we will discharge the
	// macaroon to JAAS to verify the offer access for JAAS flow.
	return []checkers.Caveat{
		checkers.TimeBeforeCaveat(o.clock.Now().Add(offerPermissionExpiryTime)),
		checkers.DeclaredCaveat(sourceModelKey, sourceModelUUID),
		checkers.DeclaredCaveat(usernameKey, username),
	}
}

// GetRemoteRelationCaveats returns the caveats for accessing a remote relation.
func (o *JAASOfferBakery) GetRemoteRelationCaveats(offerUUID, sourceModelUUID, username, relation string) []checkers.Caveat {
	return []checkers.Caveat{
		checkers.TimeBeforeCaveat(o.clock.Now().Add(offerPermissionExpiryTime)),
		checkers.DeclaredCaveat(sourceModelKey, sourceModelUUID),
		checkers.DeclaredCaveat(offerUUIDKey, offerUUID),
		checkers.DeclaredCaveat(usernameKey, username),
		checkers.DeclaredCaveat(relationKey, relation),
	}
}

// InferDeclaredFromMacaroon returns the declared attributes from the macaroon.
func (o *JAASOfferBakery) InferDeclaredFromMacaroon(mac macaroon.Slice, requiredValues map[string]string) DeclaredValues {
	declared := checkers.InferDeclared(internalmacaroon.MacaroonNamespace, mac)

	o.logger.Debugf(context.TODO(), "check macaroons with declared attrs: %v", declared)

	// We only need to inject relationKey for JAAS flow because the relation key
	// injected in juju discharge process will not be injected in JAAS discharge
	// endpoint.
	if _, ok := declared[relationKey]; !ok {
		if relation, ok := requiredValues[relationKey]; ok {
			declared[relationKey] = relation
		}
	}

	additional := make(map[string]string)

	for k, v := range declared {
		switch k {
		case sourceModelKey, usernameKey, offerUUIDKey, relationKey:
			// These are handled below.
		default:
			additional[k] = v
		}
	}

	var userName *string
	if user, ok := declared[usernameKey]; ok {
		userName = &user
	}

	return DeclaredValues{
		userName:        userName,
		sourceModelUUID: declared[sourceModelKey],
		offerUUID:       declared[offerUUIDKey],
		relationKey:     declared[relationKey],
		additional:      additional,
	}
}

// NewMacaroon creates a new macaroon for the given version, caveats and ops.
func (o *JAASOfferBakery) NewMacaroon(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error) {
	return o.oven.NewMacaroon(ctx, version, caveats, ops...)
}

// CreateDischargeMacaroon creates a discharge macaroon.
func (o *JAASOfferBakery) CreateDischargeMacaroon(
	ctx context.Context, username string,
	requiredValues map[string]string,
	declaredValues DeclaredValues,
	op bakery.Op, version bakery.Version,
) (*bakery.Macaroon, error) {
	// TODO (stickupkid): If these are required values we should check that
	// they're not empty.
	requiredOffer := requiredValues[offerUUIDKey]
	conditionParts := []string{
		"is-consumer",
		names.NewUserTag(username).String(),
		requiredOffer,
	}

	conditionCaveat := checkers.Caveat{
		Location:  o.endpoint,
		Condition: strings.Join(conditionParts, " "),
	}

	values := declaredValues.AsMap()
	declaredCaveats := make([]checkers.Caveat, 0, len(values))
	for k, v := range values {
		declaredCaveats = append(declaredCaveats, checkers.DeclaredCaveat(k, v))
	}

	return o.oven.NewMacaroon(
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
}

type externalPublicKeyLocator struct {
	store          *bakery.ThirdPartyStore
	accessEndpoint string
	httpClient     HTTPClient
	logger         logger.Logger
}

func newExternalPublicKeyLocator(accessEndpoint string, httpClient HTTPClient, logger logger.Logger) *externalPublicKeyLocator {
	return &externalPublicKeyLocator{
		store:          bakery.NewThirdPartyStore(),
		accessEndpoint: accessEndpoint,
		httpClient:     httpClient,
		logger:         logger,
	}
}

// ThirdPartyInfo implements bakery.PublicKeyLocator.
// It first checks the local store for the public key, and if not found,
// it fetches the public key from the access endpoint and caches it.
func (e *externalPublicKeyLocator) ThirdPartyInfo(ctx context.Context, loc string) (bakery.ThirdPartyInfo, error) {
	if info, err := e.store.ThirdPartyInfo(ctx, e.accessEndpoint); err == nil {
		return info, nil
	}

	info, err := httpbakery.ThirdPartyInfoForLocation(ctx, e.httpClient, e.accessEndpoint)
	if err != nil {
		return info, internalerrors.Capture(err)
	}

	e.logger.Tracef(ctx, "got third party info %#v from %q", info, e.accessEndpoint)

	e.store.AddInfo(e.accessEndpoint, info)
	return info, nil
}
