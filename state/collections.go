// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"

	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

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
	collection, closer := mongo.CollectionFromName(st.db, name)
	return newStateCollection(collection, st.EnvironUUID()), closer
}

// getRawCollection returns the named mgo Collection. As no automatic
// environment filtering is performed by the returned collection it
// should be rarely used. getCollection() should be used in almost all
// cases.
func (st *State) getRawCollection(name string) (*mgo.Collection, func()) {
	collection, closer := mongo.CollectionFromName(st.db, name)
	return collection.Writeable().Underlying(), closer
}

// getCollectionFromDB returns the specified collection from the given
// database.
//
// An environment UUID must be provided so that environment filtering
// can be automatically applied if the collection stores data for
// multiple environments.
func getCollectionFromDB(db *mgo.Database, name, envUUID string) mongo.Collection {
	collection := mongo.WrapCollection(db.C(name))
	return newStateCollection(collection, envUUID)
}

// This is all collections that contain data for multiple
// environments. Automatic environment filtering will be applied to
// these collections.
var multiEnvCollections = set.NewStrings(
	actionNotificationsC,
	actionsC,
	annotationsC,
	blockDevicesC,
	blocksC,
	charmsC,
	cleanupsC,
	constraintsC,
	containerRefsC,
	envUsersC,
	filesystemsC,
	filesystemAttachmentsC,
	instanceDataC,
	ipaddressesC,
	machinesC,
	meterStatusC,
	minUnitsC,
	networkInterfacesC,
	networksC,
	openedPortsC,
	rebootC,
	relationScopesC,
	relationsC,
	requestedNetworksC,
	sequenceC,
	servicesC,
	settingsC,
	settingsrefsC,
	statusesC,
	statusesHistoryC,
	storageAttachmentsC,
	storageConstraintsC,
	storageInstancesC,
	subnetsC,
	unitsC,
	volumesC,
	volumeAttachmentsC,
	workloadProcessesC,
)

func newStateCollection(collection mongo.Collection, envUUID string) mongo.Collection {
	if multiEnvCollections.Contains(collection.Name()) {
		return &envStateCollection{
			WriteCollection: collection.Writeable(),
			envUUID:         envUUID,
		}
	}
	return collection
}

// envStateCollection wraps a mongo.Collection, preserving the
// mongo.Collection interface and its Writeable behaviour.. It will
// automatically modify query selectors so that so that the query only
// interacts with data for a single environment (where possible).
// In particular, Inserts are not trapped at all. Be careful.
type envStateCollection struct {
	mongo.WriteCollection
	envUUID string
}

// Name returns the MongoDB collection name.
func (c *envStateCollection) Name() string {
	return c.WriteCollection.Name()
}

// Writeable is part of the Collection interface.
func (c *envStateCollection) Writeable() mongo.WriteCollection {
	return c
}

// Underlying returns the mgo Collection that the
// envStateCollection is ultimately wrapping.
func (c *envStateCollection) Underlying() *mgo.Collection {
	return c.WriteCollection.Underlying()
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
// relevant environment UUID prefix will be added on to it. Otherwise, the
// query will be handled as per Find().
func (c *envStateCollection) FindId(id interface{}) *mgo.Query {
	if sid, ok := id.(string); ok {
		return c.WriteCollection.FindId(addEnvUUID(c.envUUID, sid))
	}
	return c.Find(bson.D{{"_id", id}})
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
		return c.WriteCollection.UpdateId(addEnvUUID(c.envUUID, sid), update)
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
		return c.WriteCollection.RemoveId(addEnvUUID(c.envUUID, sid))
	}
	return c.Remove(bson.D{{"_id", id}})
}

// RemoveAll deletes all docuemnts that match a query. The query will
// be handled as per Find().
func (c *envStateCollection) RemoveAll(query interface{}) (*mgo.ChangeInfo, error) {
	return c.WriteCollection.RemoveAll(c.mungeQuery(query))
}

func (c *envStateCollection) mungeQuery(inq interface{}) bson.D {
	var outq bson.D
	switch inq := inq.(type) {
	case bson.D:
		for _, elem := range inq {
			switch elem.Name {
			case "_id":
				// TODO(ericsnow) We should be making a copy of elem.
				if id, ok := elem.Value.(string); ok {
					elem.Value = addEnvUUID(c.envUUID, id)
				} else if subquery, ok := elem.Value.(bson.D); ok {
					elem.Value = c.mungeIDSubQuery(subquery)
				}
			case "env-uuid":
				panic("env-uuid is added automatically and should not be provided")
			}
			outq = append(outq, elem)
		}
		outq = append(outq, bson.DocElem{"env-uuid", c.envUUID})
	case nil:
		outq = bson.D{{"env-uuid", c.envUUID}}
	default:
		panic("query must either be bson.D or nil")
	}
	return outq
}

// TODO(ericsnow) Is it okay to add support for $in?

func (c *envStateCollection) mungeIDSubQuery(inq bson.D) bson.D {
	var outq bson.D
	for _, elem := range inq {
		newElem := elem // copied
		switch elem.Name {
		case "$in":
			ids, ok := elem.Value.([]string)
			if !ok {
				panic("$in requires []string")
			}
			var fullIDs []string
			for _, id := range ids {
				fullID := addEnvUUID(c.envUUID, id)
				fullIDs = append(fullIDs, fullID)
			}
			newElem.Value = fullIDs
		}
		outq = append(outq, newElem)
	}
	return outq
}

func addEnvUUID(envUUID, id string) string {
	prefix := envUUID + ":"
	if strings.HasPrefix(id, prefix) {
		return id
	}
	return prefix + id
}
