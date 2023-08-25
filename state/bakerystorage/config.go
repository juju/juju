// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"encoding/json"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/internal/mongo"
)

var logger = loggo.GetLogger("juju.state.bakerystorage")

const (
	// Key for bakery config attributes.
	bakeryConfigKey = "bakeryConfig"
)

type collectionGetterFunc func(name string) (mongo.Collection, func())

// BakeryConfig defines methods used to access bakery configuration.
type BakeryConfig interface {
	InitialiseBakeryConfigOp() (txn.Op, error)
	GetLocalUsersKey() (*bakery.KeyPair, error)
	GetLocalUsersThirdPartyKey() (*bakery.KeyPair, error)
	GetExternalUsersThirdPartyKey() (*bakery.KeyPair, error)
	GetOffersThirdPartyKey() (*bakery.KeyPair, error)
}

// NewBakeryConfig returns an instance used to access bakery configuration.
func NewBakeryConfig(collection string, collectionGetter collectionGetterFunc) BakeryConfig {
	return &bakeryConfig{
		collection:       collection,
		collectionGetter: collectionGetter,
	}
}

type bakeryConfig struct {
	collection       string
	collectionGetter collectionGetterFunc
}

type bakeryConfigDoc struct {
	LocalUsersKey              string `bson:"local-users-key"`
	LocalUsersThirdPartyKey    string `bson:"local-users-thirdparty-key"`
	ExternalUsersThirdPartyKey string `bson:"external-users-thirdparty-key"`
	OffersThirdPartyKey        string `bson:"offers-thirdparty-key"`
}

// InitialiseBakeryConfigOp returns a txn.Op used to create the bakery config in state.
func (b *bakeryConfig) InitialiseBakeryConfigOp() (txn.Op, error) {
	localUsersThirdPartyKey, err := bakery.GenerateKey()
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	localUsersThirdPartyKeyBytes, err := json.Marshal(localUsersThirdPartyKey)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	externalUsersThirdPartyKey, err := bakery.GenerateKey()
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	externalUsersThirdPartyKeyBytes, err := json.Marshal(externalUsersThirdPartyKey)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	localUsersKey, err := bakery.GenerateKey()
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	localUsersKeyBytes, err := json.Marshal(localUsersKey)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	offersThirdPartyKey, err := bakery.GenerateKey()
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	offersThirdPartyKeyBytes, err := json.Marshal(offersThirdPartyKey)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}

	return txn.Op{
		C:      b.collection,
		Id:     bakeryConfigKey,
		Assert: txn.DocMissing,
		Insert: &bakeryConfigDoc{
			LocalUsersKey:              string(localUsersKeyBytes),
			LocalUsersThirdPartyKey:    string(localUsersThirdPartyKeyBytes),
			ExternalUsersThirdPartyKey: string(externalUsersThirdPartyKeyBytes),
			OffersThirdPartyKey:        string(offersThirdPartyKeyBytes),
		},
	}, nil
}

// GetLocalUsersKey returns the key pair used with the local users bakery.
func (b *bakeryConfig) GetLocalUsersKey() (*bakery.KeyPair, error) {
	doc, err := b.bakeryConfigDoc()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return b.deserialiseKey(doc.LocalUsersKey, "local users key")
}

// GetLocalUsersThirdPartyKey returns the third party key pair used with the local users bakery.
func (b *bakeryConfig) GetLocalUsersThirdPartyKey() (*bakery.KeyPair, error) {
	doc, err := b.bakeryConfigDoc()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return b.deserialiseKey(doc.LocalUsersThirdPartyKey, "local users third party key")
}

// GetExternalUsersThirdPartyKey returns the third party key pair used with the external users bakery.
func (b *bakeryConfig) GetExternalUsersThirdPartyKey() (*bakery.KeyPair, error) {
	doc, err := b.bakeryConfigDoc()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return b.deserialiseKey(doc.ExternalUsersThirdPartyKey, "external users third party key")
}

// GetOffersThirdPartyKey returns the key pair used with the cross model offers bakery.
func (b *bakeryConfig) GetOffersThirdPartyKey() (*bakery.KeyPair, error) {
	doc, err := b.bakeryConfigDoc()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return b.deserialiseKey(doc.OffersThirdPartyKey, "offers third party key")
}

func (b *bakeryConfig) deserialiseKey(data, label string) (*bakery.KeyPair, error) {
	if data == "" {
		return nil, errors.NotValidf("empty " + label)
	}

	var keyPair bakery.KeyPair
	err := json.Unmarshal([]byte(data), &keyPair)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("using %s: %s", label, keyPair.Public.String())
	return &keyPair, nil
}

func (b *bakeryConfig) bakeryConfigDoc() (*bakeryConfigDoc, error) {
	var doc bakeryConfigDoc
	controllers, closer := b.collectionGetter(b.collection)
	defer closer()
	err := controllers.FindId(bakeryConfigKey).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("bakery config")
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &doc, nil
}
