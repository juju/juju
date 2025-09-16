// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakery

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"gopkg.in/yaml.v3"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	internalerrors "github.com/juju/juju/internal/errors"
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

// baseBakery provides common functionality for offer bakeries.
type baseBakery struct{}

// ParseCaveat parses the specified caveat and returns the offer access details
// it contains.
func (o *baseBakery) ParseCaveat(caveat string) (OfferAccessDetails, error) {
	op, rest, err := checkers.ParseCaveat(caveat)
	if err != nil {
		return OfferAccessDetails{}, internalerrors.Errorf("parsing caveat %q: %w", caveat, err)
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
