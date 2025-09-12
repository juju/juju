// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakery

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"gopkg.in/macaroon.v2"
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

// MacaroonChecker exposes the methods needed from bakery.Checker.
type MacaroonChecker interface {
	// Auth returns an AuthChecker for the specified macaroons.
	Auth(mss ...macaroon.Slice) *bakery.AuthChecker
}

// DeclaredValues represents the declared values encoded in a macaroon.
type DeclaredValues struct {
	userName        *string
	sourceModelUUID string
	offerUUID       string
	relationKey     string
	additional      map[string]string
}

// NewDeclaredValues creates a new DeclaredValues instance.
func NewDeclaredValues(userName, sourceModelUUID, offerUUID, relationKey string) DeclaredValues {
	var p *string
	if userName != "" {
		p = ptr(userName)
	}
	return DeclaredValues{
		userName:        p,
		sourceModelUUID: sourceModelUUID,
		offerUUID:       offerUUID,
		relationKey:     relationKey,
		additional:      make(map[string]string),
	}
}

// User returns the user if set.
func (d DeclaredValues) UserName() (string, bool) {
	if d.userName == nil {
		return "", false
	}
	return *d.userName, true
}

// SourceModelUUID returns the source model UUID.
func (d DeclaredValues) SourceModelUUID() string {
	return d.sourceModelUUID
}

// OfferUUID returns the offer UUID.
func (d DeclaredValues) OfferUUID() string {
	return d.offerUUID
}

// RelationKey returns the relation key.
func (d DeclaredValues) RelationKey() string {
	return d.relationKey
}

// Additional returns any additional declared values.
func (d DeclaredValues) AsMap() map[string]string {
	out := make(map[string]string)
	if d.userName != nil {
		out[usernameKey] = *d.userName
	}
	if d.sourceModelUUID != "" {
		out[sourceModelKey] = d.sourceModelUUID
	}
	if d.offerUUID != "" {
		out[offerUUIDKey] = d.offerUUID
	}
	if d.relationKey != "" {
		out[relationKey] = d.relationKey
	}
	for k, v := range d.additional {
		if v == "" {
			continue
		}
		out[k] = v
	}
	return out
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
type baseBakery struct {
	checker MacaroonChecker
}

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

// GetOfferRequiredValues returns the required values for the specified
// offer access.
func (o *baseBakery) GetOfferRequiredValues(sourceModelUUID, offerUUID string) (map[string]string, error) {
	if sourceModelUUID == "" {
		return nil, internalerrors.New("source model uuid is required").Add(coreerrors.NotValid)
	}
	if offerUUID == "" {
		return nil, internalerrors.New("offer uuid is required").Add(coreerrors.NotValid)
	}
	return map[string]string{
		sourceModelKey: sourceModelUUID,
		offerUUIDKey:   offerUUID,
	}, nil
}

// GetRelationRequiredValues returns the required values for the specified
// relation access.
func (o *baseBakery) GetRelationRequiredValues(sourceModelUUID, offerUUID, relationKey string) (map[string]string, error) {
	if sourceModelUUID == "" {
		return nil, internalerrors.New("source model uuid is required").Add(coreerrors.NotValid)
	}
	if offerUUID == "" {
		return nil, internalerrors.New("offer uuid is required").Add(coreerrors.NotValid)
	}
	if relationKey == "" {
		return nil, internalerrors.New("relation key is required").Add(coreerrors.NotValid)
	}

	return map[string]string{
		sourceModelKey: sourceModelUUID,
		offerUUIDKey:   offerUUID,
		relationKey:    relationKey,
	}, nil
}

// AllowedMacaroonAuth checks the specified macaroon is valid for the operation
// and returns the associated AuthInfo.
func (o *baseBakery) AllowedAuth(ctx context.Context, op bakery.Op, mac macaroon.Slice) ([]string, error) {
	authInfo, err := o.checker.Auth(mac).Allow(ctx, op)
	if err != nil {
		return nil, err
	}
	return authInfo.Conditions(), nil
}

func ptr[T any](v T) *T {
	return &v
}
