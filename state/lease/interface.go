// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"
)

// Storage exposes lease management functionality on top of MongoDB.
type Storage interface {

	// Refresh reads all lease info for the storage's namespace. You probably
	// don't want to have too many thousand leases in a given namespace.
	Refresh() error

	// Leases returns a recent snapshot of lease state. Expiry times
	// expressed according to the Clock the Storage was configured with.
	Leases() map[string]Info

	// ClaimLease records the supplied holder's claim to the supplied lease. If
	// it succeeds, the claim is guaranteed until at least the supplied duration
	// after the call to ClaimLease was initiated. If it returns ErrInvalid,
	// check Leases() for recent state and issue a new claim if warranted.
	ClaimLease(lease, holder string, duration time.Duration) error

	// ExtendLease records the supplied holder's continued claim to the supplied
	// lease, if necessary. If it succeeds, the claim is guaranteed until at
	// least the supplied duration after the call to ExtendLease was initiated.
	ExtendLease(lease, holder string, duration time.Duration) error

	// ExpireLease records the vacation of the supplied lease. It will fail if
	// we cannot verify that the lease's writer considers the expiry time to
	// have passed.
	ExpireLease(lease string) error
}

// ErrInvalid indicates that a storage operation failed because latest state
// indicates that it's a logical impossibility.
var ErrInvalid = errors.New("invalid lease operation")

// Info is the information a Storage is willing to give out about a given lease.
type Info struct {

	// Holder is the name of the current lease holder.
	Holder string

	// EarliestExpiry is the earliest time at which it's possible the lease
	// might expire.
	EarliestExpiry time.Time

	// LatestExpiry is the latest time at which it's possible the lease might
	// still be valid.
	LatestExpiry time.Time

	// AssertOp is filthy abstraction-breaking garbage that is necessary to
	// allow us to make mgo/txn assertions about leases in the state package;
	// and which thus allows us to gate certain state changes on a particular
	// unit's leadership of a service, for example.
	AssertOp txn.Op
}

// Mongo exposes MongoDB operations for use by the lease package.
type Mongo interface {

	// RunTransaction should probably delegate to a jujutxn.Runner's Run method.
	RunTransaction(jujutxn.TransactionSource) error

	// GetCollection should probably call the mongo.CollectionFromName func.
	GetCollection(name string) (collection *mgo.Collection, closer func())
}

// Clock exposes wall-clock time for use by the lease package.
type Clock interface {

	// Now returns the current wall-clock time.
	Now() time.Time
}

// SystemClock exposes wall-clock time as returned by time.Now().
type SystemClock struct{}

// Now is part of the Clock interface.
func (SystemClock) Now() time.Time {
	return time.Now()
}
