// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/clock"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/mongo"
)

// Clock exposes time via Now, and can be controlled via Reset and Advance. It
// can be configured to Advance automatically whenever Now is called. Attempts
// to call Alarm will panic: they're not useful to a clock.Client itself, but
// are extremely helpful when driving one.
// This differs from github.com/juju/testing.Clock in that it has a Reset() function.
type Clock struct {
	clock.Clock
	now time.Time
}

// NewClock returns a *Clock, set to now.
func NewClock(now time.Time) *Clock {
	return &Clock{now: now}
}

// Now is part of the clock.Clock interface.
func (clock *Clock) Now() time.Time {
	return clock.now
}

// Reset causes the clock to act as though it had just been created with
// NewClock(now).
func (clock *Clock) Reset(now time.Time) {
	clock.now = now
}

// Advance advances the clock by the supplied time.
func (clock *Clock) Advance(duration time.Duration) {
	clock.now = clock.now.Add(duration)
}

type GlobalClock struct {
	*Clock
}

func (clock GlobalClock) Now() (time.Time, error) {
	return clock.Clock.Now(), nil
}

// Mongo exposes database operations. It uses a real database -- we can't mock
// mongo out, we need to check it really actually works -- but it's good to
// have the runner accessible for adversarial transaction tests.
type Mongo struct {
	database *mgo.Database
	runner   jujutxn.Runner
}

// NewMongo returns a *Mongo backed by the supplied database.
func NewMongo(database *mgo.Database) *Mongo {
	return &Mongo{
		database: database,
		runner: jujutxn.NewRunner(jujutxn.RunnerParams{
			Database: database,
		}),
	}
}

// GetCollection is part of the lease.Mongo interface.
func (m *Mongo) GetCollection(name string) (mongo.Collection, func()) {
	return mongo.CollectionFromName(m.database, name)
}

// RunTransaction is part of the lease.Mongo interface.
func (m *Mongo) RunTransaction(getTxn jujutxn.TransactionSource) error {
	return m.runner.Run(getTxn)
}
