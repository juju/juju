// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakery

import (
	"context"
	"sort"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/apiserver/bakeryutil"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/permission"
	internalerrors "github.com/juju/juju/internal/errors"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
)

const (
	usernameKey    = "username"
	offerUUIDKey   = "offer-uuid"
	sourceModelKey = "source-model-uuid"
	relationKey    = "relation-key"
)

const (
	offerPermissionCaveat = "has-offer-permission"

	// offerPermissionExpiryTime is used to expire offer macaroons.
	// It should be long enough to allow machines hosting workloads to
	// be provisioned so that the macaroon is still valid when the macaroon
	// is next used. If a machine takes longer, that's ok, a new discharge
	// will be obtained.
	offerPermissionExpiryTime = 3 * time.Minute
)

// BakeryStore is the interface required to back a bakery with expirable
// storage.
type BakeryStore interface {
	dbrootkeystore.ContextBacking

	// GetOffersThirdPartyKey returns the key pair used with the cross-model
	// offers bakery.
	GetOffersThirdPartyKey(ctx context.Context) (*bakery.KeyPair, error)
}

// Oven bakes new macaroons.
type Oven interface {
	// NewMacaroon creates a new macaroon.
	NewMacaroon(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error)
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

// LocalOfferBakery provides a bakery for local offer access.
type LocalOfferBakery struct {
	oven   Oven
	clock  clock.Clock
	logger logger.Logger
}

// NewLocalOfferBakery returns a new LocalOfferBakery.
func NewLocalOfferBakery(
	ctx context.Context,
	location string,
	backingStore BakeryStore,
	checker bakery.FirstPartyCaveatChecker,
	authorizer bakery.OpsAuthorizer,
	clock clock.Clock,
	logger logger.Logger,
) (*LocalOfferBakery, error) {
	key, err := backingStore.GetOffersThirdPartyKey(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "getting offers third party key")
	}

	store := internalmacaroon.NewExpirableStorage(backingStore, offerPermissionExpiryTime, clock)

	locator := bakeryutil.BakeryThirdPartyLocator{PublicKey: key.Public}
	localOfferBakery := bakery.New(bakery.BakeryParams{
		Location:      location,
		Locator:       locator,
		RootKeyStore:  store,
		Checker:       checker,
		OpsAuthorizer: authorizer,
	})
	bakery := &bakeryutil.ExpirableStorageBakery{
		Location: location,
		Locator:  locator,
		Store:    store,
		Bakery:   localOfferBakery,
		Key:      key,
	}

	return &LocalOfferBakery{
		oven:   bakery,
		clock:  clock,
		logger: logger,
	}, nil
}

// ParseCaveat parses the specified caveat and returns the offer access details
// it contains.
func (o *LocalOfferBakery) ParseCaveat(caveat string) (OfferAccessDetails, error) {
	op, rest, err := checkers.ParseCaveat(caveat)
	if err != nil {
		return OfferAccessDetails{}, errors.Annotatef(err, "parsing caveat %q", caveat)
	}
	if op != offerPermissionCaveat {
		return OfferAccessDetails{}, checkers.ErrCaveatNotRecognized
	}

	var details offerAccessDetails
	err = yaml.Unmarshal([]byte(rest), &details)
	if err != nil {
		return OfferAccessDetails{}, internalerrors.Errorf("unmarshalling offer access caveat details: %w", err).Add(coreerrors.NotValid)
	}

	return OfferAccessDetails(details), nil
}

// GetConsumeOfferCaveats returns the caveats for consuming an offer.
func (o *LocalOfferBakery) GetConsumeOfferCaveats(offerUUID, sourceModelUUID, username, relation string) []checkers.Caveat {
	caveats := []checkers.Caveat{
		checkers.TimeBeforeCaveat(o.clock.Now().Add(offerPermissionExpiryTime)),
		checkers.DeclaredCaveat(sourceModelKey, sourceModelUUID),
		checkers.DeclaredCaveat(usernameKey, username),
		checkers.DeclaredCaveat(offerUUIDKey, offerUUID),
	}

	if relation != "" {
		caveats = append(caveats, checkers.DeclaredCaveat(relationKey, relation))
	}

	return caveats
}

// InferDeclaredFromMacaroon returns the declared attributes from the macaroon.
func (o *LocalOfferBakery) InferDeclaredFromMacaroon(mac macaroon.Slice, requiredValues map[string]string) map[string]string {
	return checkers.InferDeclared(internalmacaroon.MacaroonNamespace, mac)
}

// CreateDischargeMacaroon creates a discharge macaroon.
func (o *LocalOfferBakery) CreateDischargeMacaroon(
	ctx context.Context, accessEndpoint, username string,
	requiredValues, declaredValues map[string]string,
	op bakery.Op, version bakery.Version,
) (*bakery.Macaroon, error) {
	// TODO (stickupkid): If these are required values we should check that
	// they're not empty.
	requiredSourceModelUUID := requiredValues[sourceModelKey]
	requiredOffer := requiredValues[offerUUIDKey]
	requiredRelation := requiredValues[relationKey]

	authYaml, err := encodeOfferAccessDetails(
		requiredSourceModelUUID, username, requiredOffer, requiredRelation,
		permission.ConsumeAccess,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	requiredKeys := []string{usernameKey}
	for k := range requiredValues {
		requiredKeys = append(requiredKeys, k)
	}
	sort.Strings(requiredKeys)

	return o.oven.NewMacaroon(
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

func encodeOfferAccessDetails(sourceModelUUID, username, offerURL, relationKey string, permission permission.Access) (string, error) {
	out, err := yaml.Marshal(offerAccessDetails{
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

type offerAccessDetails struct {
	SourceModelUUID string `yaml:"source-model-uuid"`
	User            string `yaml:"username"`
	OfferUUID       string `yaml:"offer-uuid"`
	Relation        string `yaml:"relation-key"`
	Permission      string `yaml:"permission"`
}
