// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/core/status"
	"github.com/juju/names/v4"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// OfferConnection represents the state of a relation
// to an offer hosted in this model.
type OfferConnection struct {
	st  *State
	doc offerConnectionDoc
}

// offerConnectionDoc represents the internal state of an offer connection in MongoDB.
type offerConnectionDoc struct {
	DocID           string `bson:"_id"`
	RelationId      int    `bson:"relation-id"`
	RelationKey     string `bson:"relation-key"`
	OfferUUID       string `bson:"offer-uuid"`
	UserName        string `bson:"username"`
	SourceModelUUID string `bson:"source-model-uuid"`
}

func newOfferConnection(st *State, doc *offerConnectionDoc) *OfferConnection {
	app := &OfferConnection{
		st:  st,
		doc: *doc,
	}
	return app
}

// OfferUUID returns the offer UUID.
func (oc *OfferConnection) OfferUUID() string {
	return oc.doc.OfferUUID
}

// UserName returns the name of the user who created this connection.
func (oc *OfferConnection) UserName() string {
	return oc.doc.UserName
}

// RelationId is the id of the relation to which this connection pertains.
func (oc *OfferConnection) RelationId() int {
	return oc.doc.RelationId
}

// SourceModelUUID is the uuid of the consuming model.
func (oc *OfferConnection) SourceModelUUID() string {
	return oc.doc.SourceModelUUID
}

// RelationKey is the key of the relation to which this connection pertains.
func (oc *OfferConnection) RelationKey() string {
	return oc.doc.RelationKey
}

func removeOfferConnectionsForRelationOps(relId int) []txn.Op {
	op := txn.Op{
		C:      offerConnectionsC,
		Id:     fmt.Sprintf("%d", relId),
		Remove: true,
	}
	return []txn.Op{op}
}

// String returns the details of the connection.
func (oc *OfferConnection) String() string {
	return fmt.Sprintf("connection to %q by %q for relation %d", oc.doc.OfferUUID, oc.doc.UserName, oc.doc.RelationId)
}

// AddOfferConnectionParams contains the parameters for adding an offer connection
// to the model.
type AddOfferConnectionParams struct {
	// SourceModelUUID is the UUID of the consuming model.
	SourceModelUUID string

	// OfferUUID is the UUID of the offer.
	OfferUUID string

	// Username is the name of the user who created this connection.
	Username string

	// RelationId is the id of the relation to which this offer pertains.
	RelationId int

	// RelationKey is the key of the relation to which this offer pertains.
	RelationKey string
}

func validateOfferConnectionParams(args AddOfferConnectionParams) (err error) {
	// Sanity checks.
	if !names.IsValidModel(args.SourceModelUUID) {
		return errors.NotValidf("source model %q", args.SourceModelUUID)
	}
	if !names.IsValidUser(args.Username) {
		return errors.NotValidf("offer connection user %q", args.Username)
	}
	return nil
}

// AddOfferConnection creates a new offer connection record, which records details about a
// relation made from a remote model to an offer in the local model.
func (st *State) AddOfferConnection(args AddOfferConnectionParams) (_ *OfferConnection, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add offer record for %q", args.OfferUUID)

	if err := validateOfferConnectionParams(args); err != nil {
		return nil, errors.Trace(err)
	}

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	} else if model.Life() != Alive {
		return nil, errors.Errorf("model is no longer alive")
	}

	// Create the application addition operations.
	offerConnectionDoc := offerConnectionDoc{
		SourceModelUUID: args.SourceModelUUID,
		OfferUUID:       args.OfferUUID,
		UserName:        args.Username,
		RelationId:      args.RelationId,
		RelationKey:     args.RelationKey,
		DocID:           fmt.Sprintf("%d", args.RelationId),
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(st); err != nil {
				return nil, errors.Trace(err)
			}
			return nil, errors.AlreadyExistsf("offer connection for relation id %d", args.RelationId)
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:      offerConnectionsC,
				Id:     offerConnectionDoc.DocID,
				Assert: txn.DocMissing,
				Insert: &offerConnectionDoc,
			},
		}
		return ops, nil
	}
	if err = st.db().Run(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}
	return &OfferConnection{doc: offerConnectionDoc}, nil
}

// AllOfferConnections returns all offer connections in the model.
func (st *State) AllOfferConnections() ([]*OfferConnection, error) {
	conns, err := st.offerConnections(nil)
	return conns, errors.Annotate(err, "getting offer connections")
}

// OfferConnections returns the offer connections for an offer.
func (st *State) OfferConnections(offerUUID string) ([]*OfferConnection, error) {
	conns, err := st.offerConnections(bson.D{{"offer-uuid", offerUUID}})
	return conns, errors.Annotatef(err, "getting offer connections for %v", offerUUID)
}

// OfferConnectionsForUser returns the offer connections for the specified user.
func (st *State) OfferConnectionsForUser(username string) ([]*OfferConnection, error) {
	conns, err := st.offerConnections(bson.D{{"username", username}})
	return conns, errors.Annotatef(err, "getting offer connections for user %q", username)
}

// offerConnections returns the offer connections for the input condition
func (st *State) offerConnections(condition bson.D) ([]*OfferConnection, error) {
	offerConnectionCollection, closer := st.db().GetCollection(offerConnectionsC)
	defer closer()

	var connDocs []offerConnectionDoc
	if err := offerConnectionCollection.Find(condition).All(&connDocs); err != nil {
		return nil, errors.Trace(err)
	}

	conns := make([]*OfferConnection, len(connDocs))
	for i, v := range connDocs {
		conns[i] = newOfferConnection(st, &v)
	}
	return conns, nil
}

// OfferConnectionForRelation returns the offer connection for the specified relation.
func (st *State) OfferConnectionForRelation(relationKey string) (*OfferConnection, error) {
	offerConnectionCollection, closer := st.db().GetCollection(offerConnectionsC)
	defer closer()

	var connDoc offerConnectionDoc
	err := offerConnectionCollection.Find(bson.D{{"relation-key", relationKey}}).One(&connDoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("offer connection for relation %q", relationKey)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get offer connection details for relation %q", relationKey)
	}
	return newOfferConnection(st, &connDoc), nil
}

// RemoteConnectionStatus returns summary information about connections to the specified offer.
func (st *State) RemoteConnectionStatus(offerUUID string) (*RemoteConnectionStatus, error) {
	conns, err := st.OfferConnections(offerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := &RemoteConnectionStatus{
		totalCount: len(conns),
	}
	for _, conn := range conns {
		rel, err := st.KeyRelation(conn.RelationKey())
		if err != nil {
			return nil, errors.Trace(err)
		}
		relStatus, err := rel.Status()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if relStatus.Status == status.Joined {
			result.activeCount++
		}
	}
	return result, nil
}

// RemoteConnectionStatus holds summary information about connections
// to an application offer.
type RemoteConnectionStatus struct {
	activeCount int
	totalCount  int
}

// TotalConnectionCount returns the number of remote applications
// related to an offer.
func (r *RemoteConnectionStatus) TotalConnectionCount() int {
	return r.totalCount
}

// ActiveConnectionCount returns the number of active remote applications
// related to an offer.
func (r *RemoteConnectionStatus) ActiveConnectionCount() int {
	return r.activeCount
}
