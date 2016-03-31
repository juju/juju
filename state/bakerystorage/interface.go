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
	"github.com/juju/utils/clock"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/juju/juju/mongo"
)

// Config contains configuration for creating bakery storage with New.
type Config struct {
	// GetCollection returns a mongo.Collection, and a function that will close
	// any associated resources, given a collection name.
	GetCollection func(name string) (collection mongo.Collection, closer func())

	// Collection is the name of the storage collection.
	Collection string

	// Clock is used to calculate the expiry time for storage items.
	Clock clock.Clock

	// ExpireAfter is the amount of time a storage item will remain in
	// the collection. It is expected that there is an "expireAfterSeconds"
	// index on the collection on the "expireAt" field, with a value of 1
	// (not 0, which is impossible with mgo's EnsureIndex interface).
	ExpireAfter time.Duration
}

// Validate validates the configuration.
func (c Config) Validate() error {
	if c.GetCollection == nil {
		return errors.NotValidf("nil GetCollection")
	}
	if c.Collection == "" {
		return errors.NotValidf("empty Collection")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.ExpireAfter == 0 {
		return errors.NotValidf("unspecified ExpireAfter")
	}
	return nil
}

// New returns an implementation of bakery.Storage
// that stores all items in MongoDB with an expiry
// time.
func New(config Config) (bakery.Storage, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating config")
	}
	return &storage{config}, nil
}
