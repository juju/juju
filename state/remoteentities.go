// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"encoding/json"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/macaroon.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// RemoteEntity defines a remote entity that has a unique opaque token that
// identifies the entity within the model.
type RemoteEntity struct {
	docID    string
	token    string
	macaroon string
}

// ID returns the RemoteEntity ID.
func (e RemoteEntity) ID() string {
	return e.docID
}

// Token returns the RemoteEntity Token.
func (e RemoteEntity) Token() string {
	return e.token
}

// Macaroon returns the RemoteEntity Macaroon associated with the Token.
func (e RemoteEntity) Macaroon() string {
	return e.macaroon
}

// remoteEntityDoc represents the internal state of a remote entity in
// MongoDB. Remote entities may be exported local entities, or imported
// remote entities.
type remoteEntityDoc struct {
	DocID string `bson:"_id"`

	Token    string `bson:"token"`
	Macaroon string `bson:"macaroon,omitempty"`
}

// RemoteEntities wraps State to provide access
// to the remote entities collection.
type RemoteEntities struct {
	st *State
}

// RemoteEntities returns a wrapped state instance providing
// access to the remote entities collection.
func (st *State) RemoteEntities() *RemoteEntities {
	return &RemoteEntities{st}
}

// AllRemoteEntities returns all the remote entities for the model.
func (st *State) AllRemoteEntities() ([]RemoteEntity, error) {
	remoteEntitiesCollection, closer := st.db().GetCollection(remoteEntitiesC)
	defer closer()

	var docs []remoteEntityDoc
	if err := remoteEntitiesCollection.Find(nil).All(&docs); err != nil {
		return nil, errors.Annotatef(err, "cannot get all remote entities")
	}
	entities := make([]RemoteEntity, len(docs))
	for i, doc := range docs {
		id, err := st.strictLocalID(doc.DocID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		entities[i] = RemoteEntity{
			docID:    id,
			token:    doc.Token,
			macaroon: doc.Macaroon,
		}
	}
	return entities, nil
}

// ExportLocalEntity adds an entity to the remote entities collection,
// returning an opaque token that uniquely identifies the entity within
// the model.
//
// If an entity is exported twice, we return an error satisfying
// errors.IsAlreadyExists(); we also still return the token so that
// a second api call is not required by the caller to get the token.
func (r *RemoteEntities) ExportLocalEntity(entity names.Tag) (string, error) {
	var token string
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// The entity must not already be exported.
		var err error
		token, err = r.GetToken(entity)
		if err == nil {
			return nil, errors.AlreadyExistsf(
				"token for %s",
				names.ReadableString(entity),
			)
		} else if !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}

		// Generate a unique token within the model.
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, errors.Trace(err)
		}
		token = uuid.String()
		_, err = r.GetRemoteEntity(token)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if err == nil {
			return nil, jujutxn.ErrTransientFailure
		}

		ops := []txn.Op{{
			C:      remoteEntitiesC,
			Id:     entity.String(),
			Assert: txn.DocMissing,
			Insert: &remoteEntityDoc{
				Token: token,
			},
		}}
		return ops, nil
	}
	if err := r.st.db().Run(buildTxn); err != nil {
		// Where error is AlreadyExists, we still return the
		// token so that a second api call is not required by
		// the caller to get the token.
		return token, errors.Trace(err)
	}
	return token, nil
}

// ImportRemoteEntity adds an entity to the remote entities collection
// with the specified opaque token.
// If the entity already exists, its token will be overwritten.
// This method assumes that the provided token is unique within the
// source model, and does not perform any uniqueness checks on it.
func (r *RemoteEntities) ImportRemoteEntity(entity names.Tag, token string) error {
	if token == "" {
		return errors.NotValidf("empty token for %v", entity.Id())
	}
	buildTxn := func(int) (ops []txn.Op, _ error) {
		remoteEntity, err := r.remoteEntityDoc(entity)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		if err == nil {
			// Same token already exists.
			if remoteEntity.Token == token {
				return nil, jujutxn.ErrNoOperations
			}
			// Token already exists, so remove first.
			ops = append(ops, r.removeRemoteEntityOps(entity)...)
		}
		ops = append(ops, r.importRemoteEntityOps(entity, token)...)
		return ops, nil
	}
	err := r.st.db().Run(buildTxn)
	return errors.Annotatef(err, "recording reference to %s", names.ReadableString(entity))
}

func (r *RemoteEntities) importRemoteEntityOps(entity names.Tag, token string) []txn.Op {
	return []txn.Op{{
		C:      remoteEntitiesC,
		Id:     entity.String(),
		Assert: txn.DocMissing,
		Insert: &remoteEntityDoc{
			Token: token,
		},
	}}
}

// RemoveRemoteEntity removes the entity from the remote entities collection,
// and releases the token if the entity belongs to the local model.
func (r *RemoteEntities) RemoveRemoteEntity(entity names.Tag) error {
	ops := func(attempt int) ([]txn.Op, error) {
		ops := r.removeRemoteEntityOps(entity)
		return ops, nil
	}
	return r.st.db().Run(ops)
}

// removeRemoteEntityOpa returns the txn.Ops to remove the remote entity
// document. It also removes any token document for exported entities.
func (r *RemoteEntities) removeRemoteEntityOps(entity names.Tag) []txn.Op {
	ops := []txn.Op{{
		C:      remoteEntitiesC,
		Id:     entity.String(),
		Remove: true,
	}}
	return ops
}

// GetToken returns the token associated with the entity with the given tag
// and model.
func (r *RemoteEntities) GetToken(entity names.Tag) (string, error) {
	remoteEntities, closer := r.st.db().GetCollection(remoteEntitiesC)
	defer closer()

	var doc remoteEntityDoc
	err := remoteEntities.FindId(entity.String()).One(&doc)
	if err == mgo.ErrNotFound {
		return "", errors.NotFoundf("token for %s", names.ReadableString(entity))
	}
	if err != nil {
		return "", errors.Annotatef(err, "reading token for %s", names.ReadableString(entity))
	}
	return doc.Token, nil
}

func (r *RemoteEntities) remoteEntityDoc(entity names.Tag) (remoteEntityDoc, error) {
	remoteEntities, closer := r.st.db().GetCollection(remoteEntitiesC)
	defer closer()

	var doc remoteEntityDoc
	err := remoteEntities.FindId(entity.String()).One(&doc)
	return doc, err
}

// GetMacaroon returns the macaroon associated with the entity with the given tag
// and model.
func (r *RemoteEntities) GetMacaroon(entity names.Tag) (*macaroon.Macaroon, error) {
	doc, err := r.remoteEntityDoc(entity)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf(
			"macaroon for %s", names.ReadableString(entity),
		)
	}
	if err != nil {
		return nil, errors.Annotatef(
			err, "reading macaroon for %s", names.ReadableString(entity),
		)
	}
	if doc.Macaroon == "" {
		return nil, nil
	}
	var mac macaroon.Macaroon
	if err := json.Unmarshal([]byte(doc.Macaroon), &mac); err != nil {
		return nil, errors.Annotatef(err, "unmarshalling macaroon for %s", names.ReadableString(entity))
	}
	return &mac, nil
}

// SaveMacaroon saves the given macaroon for the specified entity.
func (r *RemoteEntities) SaveMacaroon(entity names.Tag, mac *macaroon.Macaroon) error {
	var macJSON string
	if mac != nil {
		b, err := json.Marshal(mac)
		if err != nil {
			return errors.Trace(err)
		}
		macJSON = string(b)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		ops := []txn.Op{{
			C:      remoteEntitiesC,
			Id:     entity.String(),
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{{"macaroon", macJSON}}},
			},
		}}
		return ops, nil
	}
	return r.st.db().Run(buildTxn)
}

// GetRemoteEntity returns the tag of the entity associated with the given token.
func (r *RemoteEntities) GetRemoteEntity(token string) (names.Tag, error) {
	remoteEntities, closer := r.st.db().GetCollection(remoteEntitiesC)
	defer closer()

	var doc remoteEntityDoc
	err := remoteEntities.Find(bson.D{
		{"token", token},
	}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("entity for token %q", token)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "getting entity for token %q", token)
	}
	return names.ParseTag(r.st.localID(doc.DocID))
}
