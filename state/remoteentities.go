// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v2"
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
	ops := func(attempt int) ([]txn.Op, error) {
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

		aa := []txn.Op{{
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
		return aa, nil
	}
	if err := r.st.run(ops); err != nil {
		// Where error is AlreadyExists, we still return the
		// token so that a second api call is not required by
		// the caller to get the token.
		return token, errors.Trace(err)
	}
	return token, nil
}

// ImportRemoteEntity adds an entity to the remote entities collection
// with the specified opaque token.
//
// This method assumes that the provided token is unique within the
// source model, and does not perform any uniqueness checks on it.
func (r *RemoteEntities) ImportRemoteEntity(
	sourceModel names.ModelTag, entity names.Tag, token string,
) error {
	ops := r.importRemoteEntityOps(sourceModel, entity, token)
	err := r.st.runTransaction(ops)
	if err == txn.ErrAborted {
		return errors.AlreadyExistsf(
			"reference to %s in %s",
			names.ReadableString(entity),
			names.ReadableString(sourceModel),
		)
	}
	if err != nil {
		return errors.Annotatef(
			err, "recording reference to %s in %s",
			names.ReadableString(entity),
			names.ReadableString(sourceModel),
		)
	}
	return nil
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
			return nil, jujutxn.ErrNoOperations
		}
		ops := []txn.Op{r.removeRemoteEntityOp(sourceModel, entity)}
		if sourceModel == r.st.ModelTag() {
			ops = append(ops, txn.Op{
				C:      tokensC,
				Id:     token,
				Remove: true,
			})
		}
		return ops, nil
	}
	return r.st.run(ops)
}

// removeRemoteEntityOp returns the txn.Op to remove the remote entity
// document. It does not remove the token document for exported entities.
func (r *RemoteEntities) removeRemoteEntityOp(
	sourceModel names.ModelTag, entity names.Tag,
) txn.Op {
	return txn.Op{
		C:      remoteEntitiesC,
		Id:     r.docID(sourceModel, entity),
		Remove: true,
	}
}

// GetToken returns the token associated with the entity with the given tag
// and model.
func (r *RemoteEntities) GetToken(sourceModel names.ModelTag, entity names.Tag) (string, error) {
	remoteEntities, closer := r.st.getCollection(remoteEntitiesC)
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

// GetRemoteEntity returns the tag of the entity associated with the given
// token and model.
func (r *RemoteEntities) GetRemoteEntity(sourceModel names.ModelTag, token string) (names.Tag, error) {
	remoteEntities, closer := r.st.getCollection(remoteEntitiesC)
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
	tokens, closer := r.st.getCollection(tokensC)
	defer closer()
	n, err := tokens.FindId(token).Count()
	if err != nil {
		return false, errors.Annotatef(err, "checking existence of token %q", token)
	}
	return n != 0, nil
}
