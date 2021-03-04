// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"

	"github.com/juju/juju/core/globalclock"
	"github.com/juju/juju/mongo"
)

var (
	globalEpoch = time.Unix(0, 0)
)

// NewUpdater returns a new Updater using the supplied config, or an error.
//
// Updaters do not need to be cleaned up themselves, but they will not function
// past the lifetime of their configured *mgo.Session.
func NewUpdater(config UpdaterConfig) (*Updater, error) {
	if err := config.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	updater := &Updater{config: config}
	t, err := updater.ensureClock()
	if err != nil {
		return nil, errors.Trace(err)
	}
	updater.time = t
	return updater, nil
}

// Updater provides a means of updating the global clock time.
//
// Updater is not goroutine-safe.
// TODO (manadart 2020-11-03): This implementation is no longer used,
// except by upgrade steps.
// Remove it and the steps for Juju 3.0.
type Updater struct {
	config UpdaterConfig
	time   time.Time
}

// Advance adds the given duration to the global clock, ensuring
// that the clock has not been updated concurrently.
//
// Advance will return ErrOutOfSyncUpdate if another updater
// updates the clock concurrently. In this case, the updater
// will refresh its view of the clock, and the caller can
// attempt Advance later.
//
// If Advance returns any error other than ErrOutOfSyncUpdate,
// the Updater should be considered invalid, and the caller
// should obtain a new Updater. Failing to do so could lead
// to non-monotonic time, since there is no way of knowing in
// general whether or not the database was updated.
func (u *Updater) Advance(d time.Duration, _ <-chan struct{}) error {
	if d < 0 {
		return errors.NotValidf("duration %s", d)
	}

	coll, closer := u.collection()
	defer closer()
	new := u.time.Add(d)
	if err := coll.Update(matchTimeDoc(u.time), setTimeDoc(new)); err != nil {
		if err == mgo.ErrNotFound {
			// The document can only be not found if the clock
			// was updated by another updater concurrently. We
			// re-read the clock, and return a specific error
			// to indicate to the user that they should try
			// again later.
			t, err := readClock(coll)
			if err != nil {
				return errors.Annotate(err, "refreshing time after write conflict")
			}
			u.time = t
			return globalclock.ErrOutOfSyncUpdate
		}
		return errors.Annotatef(err,
			"adding %s to current time %s", d, u.time,
		)
	}
	u.time = new
	return nil
}

// ensureClock creates the initial epoch document if it doesn't already exist.
// Otherwise, the most recently written time is returned.
func (u *Updater) ensureClock() (time.Time, error) {
	coll, closer := u.collection()
	defer closer()

	// Read the existing clock document if it's there, initialising
	// it with a zero time otherwise.
	var doc clockDoc
	if _, err := coll.FindId(clockDocID).Apply(mgo.Change{
		// We can't use $set here, as otherwise we'll
		// overwrite an existing document.
		Update: bson.D{{"$inc", bson.D{{"time", 0}}}},
		Upsert: true,
	}, &doc); err != nil {
		return time.Time{}, errors.Annotate(err, "upserting clock document")
	}
	return doc.time(), nil
}

func (u *Updater) collection() (mongo.WriteCollection, func()) {
	coll, closer := u.config.Mongo.GetCollection(u.config.Collection)
	return coll.Writeable(), closer
}

func readClock(coll mongo.Collection) (time.Time, error) {
	var doc clockDoc
	if err := coll.FindId(clockDocID).One(&doc); err != nil {
		return time.Time{}, errors.Annotate(err, "reading clock document")
	}
	return doc.time(), nil
}

// GlobalEpoch returns the global clock's epoch, an arbitrary reference time
// at which the global clock started.
func GlobalEpoch() time.Time {
	return globalEpoch
}
