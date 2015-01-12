// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
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
func (st *State) getCollection(name string) (stateCollection, func()) {
	coll, closer := mongo.CollectionFromName(st.db, name)
	return newStateCollection(coll, st.EnvironUUID()), closer
}

// getRawCollection returns the named mgo Collection. As no automatic
// environment filtering is performed by the returned collection it
// should be rarely used. getCollection() should be used in almost all
// cases.
func (st *State) getRawCollection(name string) (*mgo.Collection, func()) {
	return mongo.CollectionFromName(st.db, name)
}

// getCollectionFromDB returns the specified collection from the given
// database.
//
// An environment UUID must be provided so that environment filtering
// can be automatically applied if the collection stores data for
// multiple environments.
func getCollectionFromDB(db *mgo.Database, name, envUUID string) stateCollection {
	return newStateCollection(db.C(name), envUUID)
}

type stateCollection interface {
	Name() string
	Underlying() *mgo.Collection
	Count() (int, error)
	Find(query interface{}) *mgo.Query
	FindId(id interface{}) *mgo.Query
	Insert(docs ...interface{}) error
	Remove(sel interface{}) error
	RemoveId(id interface{}) error
	RemoveAll(sel interface{}) (*mgo.ChangeInfo, error)
}

// This is all collections that contain data for multiple
// environments. Automatic environment filtering will be applied to
// these collections.
var multiEnvCollections = set.NewStrings(
	actionNotificationsC,
	actionsC,
	annotationsC,
	blockDevicesC,
	charmsC,
	cleanupsC,
	constraintsC,
	containerRefsC,
	instanceDataC,
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
	subnetsC,
	unitsC,
)

func newStateCollection(coll *mgo.Collection, envUUID string) stateCollection {
	if multiEnvCollections.Contains(coll.Name) {
		return &envStateCollection{
			Collection: coll,
			envUUID:    envUUID,
		}
	}
	return &genericStateCollection{Collection: coll}
}

// genericStateCollection wraps a mgo Collection. It acts as a
// pass-through which implements the stateCollection interface.
type genericStateCollection struct {
	*mgo.Collection
}

// Name returns the MongoDB collection name.
func (c *genericStateCollection) Name() string {
	return c.Collection.Name
}

// Underlying returns the mgo Collection that the
// genericStateCollection is wrapping.
func (c *genericStateCollection) Underlying() *mgo.Collection {
	return c.Collection
}

// envStateCollection wraps a mgo Collection, implementing the
// stateCollection interface. It will automatically modify query
// selectors so that so that the query only interacts with data for a
// single environment (where possible).
type envStateCollection struct {
	*mgo.Collection
	envUUID string
}

// Name returns the MongoDB collection name.
func (c *envStateCollection) Name() string {
	return c.Collection.Name
}

// Underlying returns the mgo Collection that the
// envStateCollection is wrapping.
func (c *envStateCollection) Underlying() *mgo.Collection {
	return c.Collection
}

// Count returns the number of documents in the collection that belong
// to the environment that the envStateCollection is filtering on.
func (c *envStateCollection) Count() (int, error) {
	return c.Collection.Find(bson.D{{"env-uuid", c.envUUID}}).Count()
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
	return c.Collection.Find(c.mungeQuery(query))
}

// FindId looks up a single document by _id. The id must be a string
// or bson.ObjectId. The environment UUID will be automatically
// prefixed on to the id if it's a string and the prefix isn't there
// already.
func (c *envStateCollection) FindId(id interface{}) *mgo.Query {
	return c.Collection.FindId(c.mungeId(id))
}

// Remove deletes a single document using the query provided. The
// query will be handled as per Find().
func (c *envStateCollection) Remove(query interface{}) error {
	return c.Collection.Remove(c.mungeQuery(query))
}

// RemoveId deletes a single document by id. The id will be handled as
// per FindId().
func (c *envStateCollection) RemoveId(id interface{}) error {
	return c.Collection.RemoveId(c.mungeId(id))
}

// RemoveAll deletes all docuemnts that match a query. The query will
// be handled as per Find().
func (c *envStateCollection) RemoveAll(query interface{}) (*mgo.ChangeInfo, error) {
	return c.Collection.RemoveAll(c.mungeQuery(query))
}

func (c *envStateCollection) mungeQuery(inq interface{}) bson.D {
	var outq bson.D
	switch inq := inq.(type) {
	case bson.D:
		for _, elem := range inq {
			switch elem.Name {
			case "_id":
				if id, ok := elem.Value.(string); ok {
					elem.Value = addEnvUUID(c.envUUID, id)
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

func (c *envStateCollection) mungeId(id interface{}) interface{} {
	switch idv := id.(type) {
	case string:
		return addEnvUUID(c.envUUID, idv)
	case bson.ObjectId:
		return idv
	default:
		panic(fmt.Sprintf("multi-environment collections only use string or ObjectId ids. got: %+v", id))
	}
}

func addEnvUUID(envUUID, id string) string {
	prefix := envUUID + ":"
	if strings.HasPrefix(id, prefix) {
		return id
	}
	return prefix + id
}
