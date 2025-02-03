// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	jujutxn "github.com/juju/txn/v3"
)

// InvalidateModelCredential invalidate cloud credential for the model
// of the given state.
func (st *State) InvalidateModelCredential(reason string) error {
	m, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	tag, exists := m.CloudCredentialTag()
	if !exists {
		// Model is on the cloud that does not require auth - nothing to do.
		return nil
	}

	if err := st.suspendCredentialModels(tag, reason); err != nil {
		// These updates are optimistic. If they fail, it's unfortunate but we are not going to stop the call.
		logger.Warningf(context.TODO(), "could not suspend models that use credential %v: %v", tag.Id(), err)
	}
	return nil
}

func (st *State) modelsWithCredential(tag names.CloudCredentialTag) ([]modelDoc, error) {
	coll, cleanup := st.db().GetCollection(modelsC)
	defer cleanup()

	sel := bson.D{
		{"cloud-credential", tag.Id()},
		{"life", bson.D{{"$ne", Dead}}},
	}

	var docs []modelDoc
	err := coll.Find(sel).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "getting models that use cloud credential %q", tag.Id())
	}
	if len(docs) == 0 {
		return nil, errors.NotFoundf("models that use cloud credentials %q", tag.Id())
	}
	return docs, nil
}

// RemoveModelsCredential clears out given credential reference from all models that have it.
func (st *State) RemoveModelsCredential(tag names.CloudCredentialTag) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		logger.Tracef(context.TODO(), "creating operations to remove models credential, attempt %d", attempt)
		coll, cleanup := st.db().GetCollection(modelsC)
		defer cleanup()

		sel := bson.D{
			{"cloud-credential", tag.Id()},
			{"life", bson.D{{"$ne", Dead}}},
		}
		iter := coll.Find(sel).Iter()
		defer iter.Close()

		var ops []txn.Op
		var doc bson.M
		for iter.Next(&doc) {
			id, ok := doc["_id"]
			if !ok {
				return nil, errors.New("no id found in model doc")
			}

			ops = append(ops, txn.Op{
				C:      modelsC,
				Id:     id,
				Assert: notDeadDoc,
				Update: bson.D{{"$set", bson.D{{"cloud-credential", ""}}}},
			})
		}
		if err := iter.Close(); err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	}
	return st.db().Run(buildTxn)
}

func (st *State) suspendCredentialModels(tag names.CloudCredentialTag, reason string) error {
	models, err := st.modelsWithCredential(tag)
	if err != nil {
		return errors.Annotatef(err, "could not determine what models use credential %v", tag.Id())
	}
	infos := make([]string, len(models))
	for i, m := range models {
		infos[i] = fmt.Sprintf("%s (%s)", m.Name, m.UUID)
		if err := st.updateModelCredentialInvalid(m.UUID, reason, true); err != nil {
			return errors.Trace(err)
		}
	}
	logger.Warningf(context.TODO(), "suspending these models:\n%s\n because their credential has become invalid:\n%s",
		strings.Join(infos, " - "),
		reason)
	return nil
}

func (st *State) updateModelCredentialInvalid(uuid, reason string, invalid bool) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		return []txn.Op{{
			C:      modelsC,
			Id:     uuid,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"invalid-credential", invalid}, {"invalid-credential-reason", reason},
			}}},
		}}, nil
	}
	return st.db().Run(buildTxn)
}

// SetCloudCredential sets new cloud credential for this model.
// Returned bool indicates if model credential was set.
func (m *Model) SetCloudCredential(tag names.CloudCredentialTag) (bool, error) {
	// If model is suspended, after this call, it may be reverted since,
	// if updated, model credential will be set to a valid credential.
	updating := true
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if tag.Id() == m.doc.CloudCredential {
			updating = false
			return nil, jujutxn.ErrNoOperations
		}
		return []txn.Op{{
			C:      modelsC,
			Id:     m.doc.UUID,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"cloud-credential", tag.Id()},
				{"invalid-credential", false}, {"invalid-credential-reason", ""},
			}}},
		}}, nil
	}
	if err := m.st.db().Run(buildTxn); err != nil {
		return false, errors.Trace(err)
	}

	return updating, m.Refresh()
}

// WatchModelCredential returns a new NotifyWatcher that watches
// a model reference to a cloud credential.
func (m *Model) WatchModelCredential() NotifyWatcher {
	current := m.doc.CloudCredential
	modelUUID := m.doc.UUID
	filter := func(id interface{}) bool {
		id, ok := id.(string)
		if !ok || id != modelUUID {
			return false
		}

		models, closer := m.st.db().GetCollection(modelsC)
		defer closer()

		var doc *modelDoc
		if err := models.FindId(id).One(&doc); err != nil {
			return false
		}

		match := current != doc.CloudCredential
		current = doc.CloudCredential
		return match
	}
	return newNotifyCollWatcher(m.st, modelsC, filter)
}
