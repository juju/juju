// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"encoding/json"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// remoteEntityDoc represents the internal state of a remote entity in
// MongoDB. Remote entities may be exported local entities, or imported
// remote entities.
type remoteEntityDoc struct {
	DocID string `bson:"_id"`

	SourceModelUUID string `bson:"source-model-uuid"`
	EntityTag       string `bson:"entity"`
	Token           string `bson:"token"`
	Macaroon        string `bson:"macaroon,omitempty"`
}

type tokenDoc struct {
	Token     string `bson:"_id"`
	ModelUUID string `bson:"model-uuid"`
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

// ExportLocalEntity adds an entity to the remote entities collection,
// returning an opaque token that uniquely identifies the entity within
// the model.
//
// If an entity is exported twice, we return an error satisfying
// errors.IsAlreadyExists(); we also still return the token so that
// a second api call is not required by the caller to get the token.
func (r *RemoteEntities) ExportLocalEntity(entity names.Tag) (string, error) {
	var token string
	sourceModel := r.st.ModelTag()
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// The entity must not already be exported.
		var err error
		token, err = r.GetToken(sourceModel, entity)
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
		exists, err := r.tokenExists(token)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if exists {
			return nil, jujutxn.ErrTransientFailure
		}

		ops := []txn.Op{{
			C:      tokensC,
			Id:     token,
			Assert: txn.DocMissing,
			Insert: &tokenDoc{},
		}, {
			C:      remoteEntitiesC,
			Id:     r.docID(sourceModel, entity),
			Assert: txn.DocMissing,
			Insert: &remoteEntityDoc{
				SourceModelUUID: sourceModel.Id(),
				EntityTag:       entity.String(),
				Token:           token,
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
func (r *RemoteEntities) ImportRemoteEntity(
	sourceModel names.ModelTag, entity names.Tag, token string,
) error {
	if token == "" {
		return errors.NotValidf("empty token for %v in model %v", entity.Id(), sourceModel.Id())
	}
	buildTxn := func(int) (ops []txn.Op, _ error) {
		remoteEntity, err := r.remoteEntityDoc(sourceModel, entity)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		if err == nil {
			// Same token already exists.
			if remoteEntity.Token == token {
				return nil, jujutxn.ErrNoOperations
			}
			// Token already exists, so remove first.
			ops = append(ops, r.removeRemoteEntityOps(sourceModel, entity, remoteEntity.Token)...)
		}
		ops = append(ops, r.importRemoteEntityOps(sourceModel, entity, token)...)
		return ops, nil
	}
	err := r.st.db().Run(buildTxn)
	return errors.Annotatef(
		err, "recording reference to %s in %s",
		names.ReadableString(entity),
		names.ReadableString(sourceModel),
	)
}

func (r *RemoteEntities) importRemoteEntityOps(
	sourceModel names.ModelTag, entity names.Tag, token string,
) []txn.Op {
	return []txn.Op{{
		C:      remoteEntitiesC,
		Id:     r.docID(sourceModel, entity),
		Assert: txn.DocMissing,
		Insert: &remoteEntityDoc{
			SourceModelUUID: sourceModel.Id(),
			EntityTag:       entity.String(),
			Token:           token,
		},
	}}
}

// RemoveRemoteEntity removes the entity from the remote entities collection,
// and releases the token if the entity belongs to the local model.
func (r *RemoteEntities) RemoveRemoteEntity(
	sourceModel names.ModelTag, entity names.Tag,
) error {
	ops := func(attempt int) ([]txn.Op, error) {
		token, err := r.GetToken(sourceModel, entity)
		if errors.IsNotFound(err) {
			logger.Debugf("remote entity %v from %v in model %v not found", entity, sourceModel, r.st.ModelUUID())
			return nil, jujutxn.ErrNoOperations
		}
		ops := r.removeRemoteEntityOps(sourceModel, entity, token)
		return ops, nil
	}
	return r.st.db().Run(ops)
}

// removeRemoteEntityOpa returns the txn.Ops to remove the remote entity
// document. It also removes any token document for exported entities.
func (r *RemoteEntities) removeRemoteEntityOps(
	sourceModel names.ModelTag, entity names.Tag, token string,
) []txn.Op {
	ops := []txn.Op{{
		C:      remoteEntitiesC,
		Id:     r.docID(sourceModel, entity),
		Remove: true,
	}}
	if token != "" && sourceModel == r.st.ModelTag() {
		ops = append(ops, txn.Op{
			C:      tokensC,
			Id:     token,
			Remove: true,
		})
	}
	return ops
}

// GetToken returns the token associated with the entity with the given tag
// and model.
func (r *RemoteEntities) GetToken(sourceModel names.ModelTag, entity names.Tag) (string, error) {
	remoteEntities, closer := r.st.db().GetCollection(remoteEntitiesC)
	defer closer()

	var doc remoteEntityDoc
	err := remoteEntities.FindId(r.docID(sourceModel, entity)).One(&doc)
	if err == mgo.ErrNotFound {
		return "", errors.NotFoundf(
			"token for %s in %s",
			names.ReadableString(entity),
			names.ReadableString(sourceModel),
		)
	}
	if err != nil {
		return "", errors.Annotatef(
			err, "reading token for %s in %s",
			names.ReadableString(entity),
			names.ReadableString(sourceModel),
		)
	}
	return doc.Token, nil
}

func (r *RemoteEntities) remoteEntityDoc(sourceModel names.ModelTag, entity names.Tag) (remoteEntityDoc, error) {
	remoteEntities, closer := r.st.db().GetCollection(remoteEntitiesC)
	defer closer()

	var doc remoteEntityDoc
	err := remoteEntities.FindId(r.docID(sourceModel, entity)).One(&doc)
	return doc, err
}

// GetMacaroon returns the macaroon associated with the entity with the given tag
// and model.
func (r *RemoteEntities) GetMacaroon(sourceModel names.ModelTag, entity names.Tag) (*macaroon.Macaroon, error) {
	doc, err := r.remoteEntityDoc(sourceModel, entity)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf(
			"macaroon for %s in %s",
			names.ReadableString(entity),
			names.ReadableString(sourceModel),
		)
	}
	if err != nil {
		return nil, errors.Annotatef(
			err, "reading macaroon for %s in %s",
			names.ReadableString(entity),
			names.ReadableString(sourceModel),
		)
	}
	if doc.Macaroon == "" {
		return nil, nil
	}
	var mac macaroon.Macaroon
	if err := json.Unmarshal([]byte(doc.Macaroon), &mac); err != nil {
		return nil, errors.Annotatef(
			err, "unmarshalling macaroon for %s in %s",
			names.ReadableString(entity),
			names.ReadableString(sourceModel),
		)
	}
	return &mac, nil
}

// SaveMacaroon saves the given macaroon for the specified entity.
func (r *RemoteEntities) SaveMacaroon(entity names.Tag, mac *macaroon.Macaroon) error {
	sourceModel := r.st.ModelTag()
	var macJSON string
	if mac != nil {
		b, err := json.Marshal(mac)
		if err != nil {
			return errors.Trace(err)
		}
		macJSON = string(b)
	}
	ops := func(attempt int) ([]txn.Op, error) {
		aa := []txn.Op{{
			C:      remoteEntitiesC,
			Id:     r.docID(sourceModel, entity),
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{{"macaroon", macJSON}}},
			},
		}}
		return aa, nil
	}
	return r.st.db().Run(ops)
}

// GetRemoteEntity returns the tag of the entity associated with the given
// token and model.
func (r *RemoteEntities) GetRemoteEntity(sourceModel names.ModelTag, token string) (names.Tag, error) {
	remoteEntities, closer := r.st.db().GetCollection(remoteEntitiesC)
	defer closer()

	var doc remoteEntityDoc
	err := remoteEntities.Find(bson.D{
		{"source-model-uuid", sourceModel.Id()},
		{"token", token},
	}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf(
			"entity for token %q in %s",
			token, names.ReadableString(sourceModel),
		)
	}
	if err != nil {
		return nil, errors.Annotatef(
			err, "getting entity for token %q in %s",
			token, names.ReadableString(sourceModel),
		)
	}
	return names.ParseTag(doc.EntityTag)
}

func (r *RemoteEntities) docID(sourceModel names.ModelTag, entity names.Tag) string {
	return sourceModel.Id() + "-" + entity.String()
}

func (r *RemoteEntities) tokenExists(token string) (bool, error) {
	tokens, closer := r.st.db().GetCollection(tokensC)
	defer closer()
	n, err := tokens.FindId(token).Count()
	if err != nil {
		return false, errors.Annotatef(err, "checking existence of token %q", token)
	}
	return n != 0, nil
}
