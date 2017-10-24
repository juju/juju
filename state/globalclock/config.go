// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock

import (
	"github.com/juju/errors"
	mgo "gopkg.in/mgo.v2"
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
// create an Updater or Watcher.
type Config struct {
	// Database names the MongoDB database in which the clock
	// collection is stored.
	Database string

	// Collection names the MongoDB collection in which the clock
	// documents are stored.
	Collection string

	// Session is the *mgo.Session to be used for updating and
	// watching the clock.
	//
	// For the Updater, the session should not be copied, as we
	// rely on the session being closed when mastership changes,
	// to guarantee at most one Updater is running at any time.
	Session *mgo.Session
}

// validate returns an error if the supplied config is not valid.
func (config Config) validate() error {
	if config.Database == "" {
		return errors.New("missing database")
	}
	if config.Collection == "" {
		return errors.New("missing collection")
	}
	if config.Session == nil {
		return errors.New("missing mongo session")
	}
	return nil
}
