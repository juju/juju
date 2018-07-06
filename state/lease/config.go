// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/mongo"
)

// Mongo exposes MongoDB operations for use by the lease package.
type Mongo interface {

	// RunTransaction should probably delegate to a jujutxn.Runner's Run method.
	RunTransaction(jujutxn.TransactionSource) error

	// GetCollection should probably call the mongo.CollectionFromName func.
	GetCollection(name string) (collection mongo.Collection, closer func())
}

// LocalClock provides the writer-local wall clock interface required by
// the lease package.
type LocalClock interface {

	// Now returns the current, writer-local wall-clock time.
	//
	// Now is required to return times with a monotonic component,
	// as returned by Go 1.9 and onwards, such that local times
	// can be safely compared in the face of wall clock jumps.
	Now() time.Time
}

// GlobalClock provides the global clock interface required by the lease
// package.
type GlobalClock interface {

	// Now returns the current global clock time.
	//
	// Now is required to return monotonically increasing times.
	Now() (time.Time, error)
}

// StoreConfig contains the resources and information required to create
// a Store. Multiple stores can collaborate if they share a collection and
// namespace, so long as they do not share ids; but within a collection,
// stores for different namespaces will not interfere with one another,
// regardless of id.
type StoreConfig struct {

	// Id uniquely identifies the store. Multiple stores with the same id
	// running concurrently will cause undefined behaviour.
	Id string

	// ModelUUID identifies the model the leases will be stored in.
	ModelUUID string

	// Namespace identifies a group of stores which operate on the same data.
	Namespace string

	// Collection names the MongoDB collection in which lease data is stored.
	Collection string

	// Mongo exposes the mgo[/txn] capabilities required by a Store.
	Mongo Mongo

	// LocalClock exposes the writer-local wall-clock time to a Store.
	LocalClock LocalClock

	// GlobalClock exposes the global clock to a Store.
	GlobalClock GlobalClock
}

// validate returns an error if the supplied config is not valid.
func (config StoreConfig) validate() error {
	if err := lease.ValidateString(config.Id); err != nil {
		return errors.Annotatef(err, "invalid id")
	}
	if err := lease.ValidateString(config.Namespace); err != nil {
		return errors.Annotatef(err, "invalid namespace")
	}
	if err := lease.ValidateString(config.Collection); err != nil {
		return errors.Annotatef(err, "invalid collection")
	}
	if config.Mongo == nil {
		return errors.New("missing mongo")
	}
	if config.LocalClock == nil {
		return errors.New("missing local clock")
	}
	if config.GlobalClock == nil {
		return errors.New("missing global clock")
	}
	return nil
}
