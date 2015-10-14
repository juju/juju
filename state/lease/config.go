// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/mongo"
)

// Mongo exposes MongoDB operations for use by the lease package.
type Mongo interface {

	// RunTransaction should probably delegate to a jujutxn.Runner's Run method.
	RunTransaction(jujutxn.TransactionSource) error

	// GetCollection should probably call the mongo.CollectionFromName func.
	GetCollection(name string) (collection mongo.Collection, closer func())
}

// ClientConfig contains the resources and information required to create
// a Client. Multiple clients can collaborate if they share a collection and
// namespace, so long as they do not share ids; but within a collection,
// clients for different namespaces will not interfere with one another,
// regardless of id.
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
	Clock clock.Clock
}

// Validate returns an error if the supplied config is not valid.
func (config ClientConfig) Validate() error {
	if err := validateString(config.Id); err != nil {
		return errors.Annotatef(err, "invalid id")
	}
	if err := validateString(config.Namespace); err != nil {
		return errors.Annotatef(err, "invalid namespace")
	}
	if err := validateString(config.Collection); err != nil {
		return errors.Annotatef(err, "invalid collection")
	}
	if config.Mongo == nil {
		return errors.New("missing mongo")
	}
	if config.Clock == nil {
		return errors.New("missing clock")
	}
	return nil
}
