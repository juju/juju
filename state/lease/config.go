// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
)

// ClientConfig contains the resources and information required to create
// a Client. Multiple clients can collaborate if they share a collection and
// namespace; within a collection, clients for different namespaces will not
// interfere with one another.
type ClientConfig struct {

	// Id uniquely identifies the client. Multiple clients with the same id
	// running concurrently will cause undefined behaviour.
	Id string

	// Namespace identifies a group of clients which operate on the same data.
	Namespace string

	// Collection names the MongoDB collection in which lease data is stored.
	Collection string

	// Mongo exposes the mgo[/txn] capabilities required by a Client.
	Mongo Mongo

	// Clock exposes the wall-clock time to a Client.
	Clock Clock
}

// Validate returns an error if the supplied config is not valid.
func (config ClientConfig) Validate() error {
	if err := validateString(config.Id); err != nil {
		return errors.Annotatef(err, "invalid Id")
	}
	if err := validateString(config.Namespace); err != nil {
		return errors.Annotatef(err, "invalid Namespace")
	}
	if err := validateString(config.Collection); err != nil {
		return errors.Annotatef(err, "invalid Collection")
	}
	if config.Mongo == nil {
		return errors.New("missing Mongo")
	}
	if config.Clock == nil {
		return errors.New("missing Clock")
	}
	return nil
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

// SystemClock exposes wall-clock time as returned by time.Now.
type SystemClock struct{}

// Now is part of the Clock interface.
func (SystemClock) Now() time.Time {
	return time.Now()
}
