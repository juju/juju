// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/core/globalclock"
	"github.com/juju/loggo"
	mgo "gopkg.in/mgo.v2"
)

var (
	logger = loggo.GetLogger("juju.state.globalclock")

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
type Updater struct {
	config UpdaterConfig
	time   time.Time
}

// AddTime adds the given duration to the global clock, ensuring
// that the clock has not been updated concurrently.
//
// AddTime will return ErrConcurrentUpdate if another updater
// updates the clock concurrently. In this case, the updater
// will refresh its view of the clock, and the caller can
// attempt AddTime later.
//
// If AddTime returns any error other than ErrConcurrentUpdate,
// the Updater should be considered invalid, and the caller
// should obtain a new Updater. Failing to do so could lead
// to non-monotonic time, since there is no way of knowing in
// general whether or not the database was updated.
func (u *Updater) AddTime(d time.Duration) error {
	if d < 0 {
		return errors.NotValidf("duration %s", d)
	}
	coll := u.config.Session.DB(u.config.Database).C(u.config.Collection)
	new := u.time.Add(d)
	if err := coll.Update(matchTimeDoc(u.time), setTimeDoc(new)); err != nil {
		if err == mgo.ErrNotFound {
			// The document can only be not found if the clock
			// was updated by another updater concurrently. We
			// re-read the clock, and return a specific error
			// to indicate to the user that they should try
			// again later.
			t, err := readClock(u.config.Config)
			if err != nil {
				return errors.Annotate(err, "refreshing time after write conflict")
			}
			u.time = t
			return globalclock.ErrConcurrentUpdate
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
	coll := u.config.Session.DB(u.config.Database).C(u.config.Collection)
	err := coll.Insert(newClockDoc(globalEpoch))
	if err != nil {
		if mgo.IsDup(err) {
			// Document already exists, return the existing value.
			return readClock(u.config.Config)
		}
		return time.Time{}, errors.Annotate(err, "writing epoch clock document")
	}
	return globalEpoch, nil
}

func readClock(config Config) (time.Time, error) {
	coll := config.Session.DB(config.Database).C(config.Collection)
	var doc clockDoc
	if err := coll.FindId(clockDocID).One(&doc); err != nil {
		return time.Time{}, errors.Annotate(err, "reading clock document")
	}
	return doc.time(), nil
}
