// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// OfferConnection represents the state of an relation
// to an offer hosted in this model.
type OfferConnection struct {
	st  *State
	doc offerConnectionDoc
}

// offerConnectionDoc represents the internal state of an offer connection in MongoDB.
type offerConnectionDoc struct {
	DocID      string `bson:"_id"`
	RelationId int    `bson:"relation-id"`
	OfferName  string `bson:"offer-name"`
	UserName   string `bson:"username"`
}

func newOfferConnection(st *State, doc *offerConnectionDoc) *OfferConnection {
	app := &OfferConnection{
		st:  st,
		doc: *doc,
	}
	return app
}

// OfferName returns the offer name.
func (oc *OfferConnection) OfferName() string {
	return oc.doc.OfferName
}

// UserName returns the name of the user who created this connection.
func (oc *OfferConnection) UserName() string {
	return oc.doc.UserName
}

// RelationId is the id of the relation to which this connection pertains.
func (oc *OfferConnection) RelationId() int {
	return oc.doc.RelationId
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
	return fmt.Sprintf("connection to %q by %q for relation %d", oc.doc.OfferName, oc.doc.UserName, oc.doc.RelationId)
}

// AddOfferConnectionParams contains the parameters for adding an offer connection
// to the model.
type AddOfferConnectionParams struct {
	// OfferName is the name of the offer.
	OfferName string

	// Username is the name of the user who created this connection.
	Username string

	// RelationId is the id of the relation to which this offer pertains.
	RelationId int
}

// AddOfferConnection creates a new offer connection record, which records details about a
// relation made from a remote model to an offer in the local model.
func (st *State) AddOfferConnection(args AddOfferConnectionParams) (_ *OfferConnection, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add offer record for %q", args.OfferName)

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	} else if model.Life() != Alive {
		return nil, errors.Errorf("model is no longer alive")
	}

	// Create the application addition operations.
	offerConnectionDoc := offerConnectionDoc{
		OfferName:  args.OfferName,
		UserName:   args.Username,
		RelationId: args.RelationId,
		DocID:      fmt.Sprintf("%d", args.RelationId),
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

// AllOfferConnections returns all the offer connections in the model.
func (st *State) AllOfferConnections() (conns []*OfferConnection, err error) {
	offerConnectionCollection, closer := st.db().GetCollection(offerConnectionsC)
	defer closer()

	connDocs := []offerConnectionDoc{}
	err = offerConnectionCollection.Find(bson.D{}).All(&connDocs)
	if err != nil {
		return nil, errors.Errorf("cannot get all offer connections")
	}
	for _, v := range connDocs {
		conns = append(conns, newOfferConnection(st, &v))
	}
	return conns, nil
}

// RemoteConnectionStatus returns summary information about connections to the specified offer.
func (st *State) RemoteConnectionStatus(offerName string) (*RemoteConnectionStatus, error) {
	offerConnectionCollection, closer := st.db().GetCollection(offerConnectionsC)
	defer closer()

	count, err := offerConnectionCollection.Find(bson.D{{"offer-name", offerName}}).Count()
	if err != nil {
		return nil, errors.Errorf("cannot get remote connection status for offer %q", offerName)
	}
	return &RemoteConnectionStatus{
		count: count,
	}, nil
}

// RemoteConnectionStatus holds summary information about connections
// to an application offer.
type RemoteConnectionStatus struct {
	count int
}

// ConnectionCount returns the number of remote applications
// related to an offer.
func (r *RemoteConnectionStatus) ConnectionCount() int {
	return r.count
}
