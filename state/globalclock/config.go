// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock

import (
	"github.com/juju/errors"

	"github.com/juju/juju/v2/mongo"
)

// UpdaterConfig contains the resources and information required to
// create an Updater.
type UpdaterConfig struct {
	Config
}

// ReaderConfig contains the resources and information required to
// create a Reader.
type ReaderConfig struct {
	Config
}

// Config contains the common resources and information required to
// create an Updater or Reader.
type Config struct {
	// Collection names the MongoDB collection in which the clock
	// documents are stored.
	Collection string

	// Mongo exposes the mgo capabilities required by a Client
	// for updating and reading the clock.
	Mongo Mongo
}

// Mongo exposes MongoDB operations for use by the globalclock package.
type Mongo interface {
	// GetCollection should probably call the mongo.CollectionFromName func
	GetCollection(name string) (collection mongo.Collection, closer func())
}

// validate returns an error if the supplied config is not valid.
func (config Config) validate() error {
	if config.Collection == "" {
		return errors.New("missing collection")
	}
	if config.Mongo == nil {
		return errors.New("missing mongo client")
	}
	return nil
}
