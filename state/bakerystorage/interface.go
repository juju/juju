// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bakerystorage

import (
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"

	"github.com/juju/juju/internal/mongo"
)

// Config contains configuration for creating bakery storage with New.
type Config struct {
	// GetCollection returns a mongo.Collection and a function that
	// will close any associated resources.
	GetCollection func() (collection mongo.Collection, closer func())

	// GetStorage returns a bakery.Storage and a function that will close
	// any associated resources.
	GetStorage func(rootKeys *RootKeys, coll mongo.Collection, expireAfter time.Duration) (storage bakery.RootKeyStore)
}

// Validate validates the configuration.
func (c Config) Validate() error {
	if c.GetCollection == nil {
		return errors.NotValidf("nil GetCollection")
	}
	if c.GetStorage == nil {
		return errors.NotValidf("nil GetStorage")
	}
	return nil
}

// ExpirableStorage extends bakery.Storage with the ExpireAfter method,
// to expire data added at the specified time.
type ExpirableStorage interface {
	bakery.RootKeyStore

	// ExpireAfter returns a new ExpirableStorage that will expire
	// added items after the specified duration.
	ExpireAfter(time.Duration) ExpirableStorage
}

// New returns an implementation of bakery.Storage
// that stores all items in MongoDB with an expiry
// time.
func New(config Config) (ExpirableStorage, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating config")
	}
	return &storage{
		config:   config,
		rootKeys: NewRootKeys(5),
	}, nil
}
