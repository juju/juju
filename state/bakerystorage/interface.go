// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bakerystorage provides an implementation
// of the bakery Storage interface that uses MongoDB
// to store items.
//
// This is based on gopkg.in/macaroon-bakery.v1/bakery/mgostorage.
package bakerystorage

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/mongo"
)

// Config contains configuration for creating bakery storage with New.
type Config struct {
	// GetCollection returns a mongo.Collection and a function that
	// will close any associated resources.
	GetCollection func() (collection mongo.Collection, closer func())
}

// Validate validates the configuration.
func (c Config) Validate() error {
	if c.GetCollection == nil {
		return errors.NotValidf("nil GetCollection")
	}
	return nil
}

// ExpirableStorage extends bakery.Storage with the ExpireAt method,
// to expire data added at the specified time.
type ExpirableStorage interface {
	bakery.Storage

	// ExpireAt returns a new ExpirableStorage that will expire
	// added items at the specified time.
	ExpireAt(time.Time) ExpirableStorage
}

// New returns an implementation of bakery.Storage
// that stores all items in MongoDB with an expiry
// time.
func New(config Config) (ExpirableStorage, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating config")
	}
	return &storage{config, time.Time{}}, nil
}

// MongoIndexes returns the indexes to apply to the MongoDB collection.
func MongoIndexes() []mgo.Index {
	return []mgo.Index{expiryTimeIndex}
}
