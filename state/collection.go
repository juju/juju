// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/errors"
	"github.com/juju/juju/mongo"
)

// getCollection fetches a named collection using a new session if the
// database has previously been logged in to. It returns the
// collection and a closer function for the session.
//
// If the collection stores documents for multiple environments, the
// returned collection will automatically perform environment
// filtering where possible. See envStateCollection below.
func (st *State) getCollection(name string) (mongo.Collection, func()) {
	return st.database.GetCollection(name)
}

// getRawCollection returns the named mgo Collection. As no automatic
// environment filtering is performed by the returned collection it
// should be rarely used. getCollection() should be used in almost all
// cases.
func (st *State) getRawCollection(name string) (*mgo.Collection, func()) {
	collection, closer := st.database.GetCollection(name)
	return collection.Writeable().Underlying(), closer
}

// envStateCollection wraps a mongo.Collection, preserving the
// mongo.Collection interface and its Writeable behaviour. It will
// automatically modify query selectors and documents so that queries
// and inserts only interact with data for a single environment (where
// possible).
type envStateCollection struct {
	mongo.WriteCollection
	envUUID string
}

// Writeable is part of the Collection interface.
func (c *envStateCollection) Writeable() mongo.WriteCollection {
	// Note that we can't delegate this to the embedded WriteCollection:
	// that would return a writeable collection without any env-handling.
	return c
}

// Count returns the number of documents in the collection that belong
// to the environment that the envStateCollection is filtering on.
func (c *envStateCollection) Count() (int, error) {
	return c.WriteCollection.Find(bson.D{{"env-uuid", c.envUUID}}).Count()
}

// Find performs a query on the collection. The query must be given as
// either nil or a bson.D.
//
// An "env-uuid" condition will always be added to the query to ensure
// that only data for the environment being filtered on is returned.
//
// If a simple "_id" field selector is included in the query
// (e.g. "{{"_id", "foo"}}" the relevant environment UUID prefix will
// be added on to the id. Note that more complex selectors using the
// "_id" field (e.g. using the $in operator) will not be modified. In
// these cases it is up to the caller to add environment UUID
// prefixes when necessary.
func (c *envStateCollection) Find(query interface{}) *mgo.Query {
	return c.WriteCollection.Find(c.mungeQuery(query))
}

// FindId looks up a single document by _id. If the id is a string the
// relevant environment UUID prefix will be added to it. Otherwise, the
// query will be handled as per Find().
func (c *envStateCollection) FindId(id interface{}) *mgo.Query {
	if sid, ok := id.(string); ok {
		return c.WriteCollection.FindId(ensureEnvUUID(c.envUUID, sid))
	}
	return c.Find(bson.D{{"_id", id}})
}

// Insert adds one or more documents to a collection. If the document
// id is a string the environment UUID prefix will be automatically
// added to it. The env-uuid field will also be automatically added if
// it is missing. An error will be returned if an env-uuid field is
// provided but is the wrong value.
func (c *envStateCollection) Insert(docs ...interface{}) error {
	var mungedDocs []interface{}
	for _, doc := range docs {
		mungedDoc, err := mungeDocForMultiEnv(doc, c.envUUID, envUUIDRequired)
		if err != nil {
			return errors.Trace(err)
		}
		mungedDocs = append(mungedDocs, mungedDoc)
	}
	return c.WriteCollection.Insert(mungedDocs...)
}

// Update finds a single document matching the provided query document and
// modifies it according to the update document.
//
// An "env-uuid" condition will always be added to the query to ensure
// that only data for the environment being filtered on is returned.
//
// If a simple "_id" field selector is included in the query
// (e.g. "{{"_id", "foo"}}" the relevant environment UUID prefix will
// be added on to the id. Note that more complex selectors using the
// "_id" field (e.g. using the $in operator) will not be modified. In
// these cases it is up to the caller to add environment UUID
// prefixes when necessary.
func (c *envStateCollection) Update(query interface{}, update interface{}) error {
	return c.WriteCollection.Update(c.mungeQuery(query), update)
}

// UpdateId finds a single document by _id and modifies it according to the
// update document. The id must be a string or bson.ObjectId. The environment
// UUID will be automatically prefixed on to the id if it's a string and the
// prefix isn't there already.
func (c *envStateCollection) UpdateId(id interface{}, update interface{}) error {
	if sid, ok := id.(string); ok {
		return c.WriteCollection.UpdateId(ensureEnvUUID(c.envUUID, sid), update)
	}
	return c.WriteCollection.UpdateId(bson.D{{"_id", id}}, update)
}

// Remove deletes a single document using the query provided. The
// query will be handled as per Find().
func (c *envStateCollection) Remove(query interface{}) error {
	return c.WriteCollection.Remove(c.mungeQuery(query))
}

// RemoveId deletes a single document by id. If the id is a string the
// relevant environment UUID prefix will be added on to it. Otherwise, the
// query will be handled as per Find().
func (c *envStateCollection) RemoveId(id interface{}) error {
	if sid, ok := id.(string); ok {
		return c.WriteCollection.RemoveId(ensureEnvUUID(c.envUUID, sid))
	}
	return c.Remove(bson.D{{"_id", id}})
}

// RemoveAll deletes all documents that match a query. The query will
// be handled as per Find().
func (c *envStateCollection) RemoveAll(query interface{}) (*mgo.ChangeInfo, error) {
	return c.WriteCollection.RemoveAll(c.mungeQuery(query))
}

func (c *envStateCollection) mungeQuery(inq interface{}) bson.D {
	outq, err := mungeDocForMultiEnv(inq, c.envUUID, envUUIDRequired|noEnvUUIDInInput)
	if err != nil {
		panic(err)
	}
	return outq
}
