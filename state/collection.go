// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/mgo/v3"

	"github.com/juju/juju/internal/mongo"
)

// modelStateCollection wraps a mongo.Collection, preserving the
// mongo.Collection interface and its Writeable behaviour. It will
// automatically modify query selectors and documents so that queries
// and inserts only interact with data for a single model (where
// possible).
type modelStateCollection struct {
	mongo.WriteCollection
}

// Writeable is part of the Collection interface.
func (c *modelStateCollection) Writeable() mongo.WriteCollection {
	// Note that we can't delegate this to the embedded WriteCollection:
	// that would return a writeable collection without any env-handling.
	return c
}

// Count returns the number of documents in the collection that belong
// to the model that the modelStateCollection is filtering on.
func (c *modelStateCollection) Count() (int, error) {
	return 0, nil
}

// Find performs a query on the collection. The query must be given as
// either nil or a bson.D.
//
// An "model-uuid" condition will always be added to the query to ensure
// that only data for the model being filtered on is returned.
//
// If a simple "_id" field selector is included in the query
// (e.g. "{{"_id", "foo"}}" the relevant model UUID prefix will
// be added on to the id. Note that more complex selectors using the
// "_id" field (e.g. using the $in operator) will not be modified. In
// these cases it is up to the caller to add model UUID
// prefixes when necessary.
func (c *modelStateCollection) Find(query interface{}) mongo.Query {
	return nil
}

// FindId looks up a single document by _id. If the id is a string the
// relevant model UUID prefix will be added to it. Otherwise, the
// query will be handled as per Find().
func (c *modelStateCollection) FindId(id interface{}) mongo.Query {
	return nil
}

// Insert adds one or more documents to a collection. If the document
// id is a string the model UUID prefix will be automatically
// added to it. The model-uuid field will also be automatically added if
// it is missing. An error will be returned if an model-uuid field is
// provided but is the wrong value.
func (c *modelStateCollection) Insert(docs ...interface{}) error {
	return nil
}

// Update finds a single document matching the provided query document and
// modifies it according to the update document.
//
// An "model-uuid" condition will always be added to the query to ensure
// that only data for the model being filtered on is returned.
//
// If a simple "_id" field selector is included in the query
// (e.g. "{{"_id", "foo"}}" the relevant model UUID prefix will
// be added on to the id. Note that more complex selectors using the
// "_id" field (e.g. using the $in operator) will not be modified. In
// these cases it is up to the caller to add model UUID
// prefixes when necessary.
func (c *modelStateCollection) Update(query interface{}, update interface{}) error {
	return nil
}

// UpdateId finds a single document by _id and modifies it according to the
// update document. The id must be a string or bson.ObjectId. The model
// UUID will be automatically prefixed on to the id if it's a string and the
// prefix isn't there already.
func (c *modelStateCollection) UpdateId(id interface{}, update interface{}) error {
	return nil
}

// Remove deletes a single document using the query provided. The
// query will be handled as per Find().
func (c *modelStateCollection) Remove(query interface{}) error {
	return nil
}

// RemoveId deletes a single document by id. If the id is a string the
// relevant model UUID prefix will be added on to it. Otherwise, the
// query will be handled as per Find().
func (c *modelStateCollection) RemoveId(id interface{}) error {
	return nil
}

// RemoveAll deletes all documents that match a query. The query will
// be handled as per Find().
func (c *modelStateCollection) RemoveAll(query interface{}) (*mgo.ChangeInfo, error) {
	return nil, nil
}
