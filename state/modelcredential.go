// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
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
		logger.Warningf("could not suspend models that use credential %v: %v", tag.Id(), err)
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

// CredentialModelsAndOwnerAccess returns all models that use given cloud credential as well as
// what access the credential owner has on these models.
func (st *State) CredentialModelsAndOwnerAccess(tag names.CloudCredentialTag) ([]cloud.CredentialOwnerModelAccess, error) {
	models, err := st.modelsWithCredential(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []cloud.CredentialOwnerModelAccess
	for _, m := range models {
		ownerAccess, err := st.UserAccess(tag.Owner(), names.NewModelTag(m.UUID))
		if err != nil {
			if errors.Is(err, errors.NotFound) {
				results = append(results, cloud.CredentialOwnerModelAccess{ModelName: m.Name, ModelUUID: m.UUID, OwnerAccess: permission.NoAccess})
				continue
			}
			results = append(results, cloud.CredentialOwnerModelAccess{ModelName: m.Name, ModelUUID: m.UUID, Error: errors.Trace(err)})
			continue
		}
		results = append(results, cloud.CredentialOwnerModelAccess{ModelName: m.Name, ModelUUID: m.UUID, OwnerAccess: ownerAccess.Access})
	}
	return results, nil
}

// RemoveModelsCredential clears out given credential reference from all models that have it.
func (st *State) RemoveModelsCredential(tag names.CloudCredentialTag) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		logger.Tracef("creating operations to remove models credential, attempt %d", attempt)
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
	logger.Warningf("suspending these models:\n%s\n because their credential has become invalid:\n%s",
		strings.Join(infos, " - "),
		reason)
	sts := ModelStatusInvalidCredential(reason)
	doc := statusDoc{
		Status:     sts.Status,
		StatusInfo: sts.Message,
		StatusData: sts.Data,
		Updated:    timeOrNow(nil, st.clock()).UnixNano(),
	}
	for _, m := range models {
		st.maybeSetModelStatusHistoryDoc(m.UUID, doc)
	}
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

func (st *State) model(uuid string) (*Model, func() error, error) {
	closer := func() error { return nil }
	// We explicitly don't start the workers.
	modelState, err := st.newStateNoWorkers(uuid)
	if err != nil {
		// This model could have been removed.
		if errors.Is(err, errors.NotFound) {
			return nil, closer, nil
		}
		return nil, closer, errors.Trace(err)
	}

	closer = func() error { return modelState.Close() }
	m, err := modelState.Model()
	if err != nil {
		return nil, closer, errors.Trace(err)
	}
	return m, closer, nil
}

func (st *State) maybeSetModelStatusHistoryDoc(modelUUID string, doc statusDoc) {
	one, closer, err := st.model(modelUUID)
	defer func() { _ = closer() }()
	if err != nil {
		logger.Warningf("model %v error: %v", modelUUID, err)
		return
	}

	if _, err = probablyUpdateStatusHistory(one.st.db(), one.Kind(), one.globalKey(), one.globalKey(), doc, status.NoopStatusHistoryRecorder); err != nil {
		logger.Warningf("%v", err)
	}
}

func (m *Model) maybeRevertModelStatus() error {
	// I don't know where you've been before you got here - get a clean slate.
	err := m.Refresh()
	if err != nil {
		logger.Warningf("could not refresh model %v to revert its status: %v", m.UUID(), err)
	}
	modelStatus, err := m.Status()
	if err != nil {
		return errors.Trace(err)
	}
	if modelStatus.Status != status.Suspended {
		doc := statusDoc{
			Status:     modelStatus.Status,
			StatusInfo: modelStatus.Message,
			Updated:    timeOrNow(nil, m.st.clock()).UnixNano(),
		}

		if _, err = probablyUpdateStatusHistory(m.st.db(), m.Kind(), m.globalKey(), m.globalKey(), doc, status.NoopStatusHistoryRecorder); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// SetCloudCredential sets new cloud credential for this model.
// Returned bool indicates if model credential was set.
func (m *Model) SetCloudCredential(tag names.CloudCredentialTag) (bool, error) {
	// If model is suspended, after this call, it may be reverted since,
	// if updated, model credential will be set to a valid credential.
	modelStatus, err := m.Status()
	if err != nil {
		return false, errors.Annotatef(err, "getting model status %q", m.UUID())
	}
	revert := modelStatus.Status == status.Suspended
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
	if updating && revert {
		if err := m.maybeRevertModelStatus(); err != nil {
			logger.Warningf("could not revert status for model %v: %v", m.UUID(), err)
		}
	}
	return updating, m.Refresh()
}

func (st *State) modelsToRevert(tag names.CloudCredentialTag) (map[*Model]func() error, error) {
	revert := map[*Model]func() error{}
	credentialModels, err := st.modelsWithCredential(tag)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return revert, errors.Annotatef(err, "getting models for credential %v", tag)
	}
	for _, m := range credentialModels {
		one, closer, err := st.model(m.UUID)
		if err != nil {
			_ = closer()
			logger.Warningf("model %v error: %v", m.UUID, err)
			continue
		}
		modelStatus, err := one.Status()
		if err != nil {
			_ = closer()
			return revert, errors.Trace(err)
		}
		// We're only interested if the models are currently suspended.
		if modelStatus.Status == status.Suspended {
			revert[one] = closer
			continue
		}

		// We're not interested in this model; close its session now.
		_ = closer()
	}
	return revert, nil
}

// CloudCredentialUpdated updates models which use a credential
// to have their suspended status reverted.
func (st *State) CloudCredentialUpdated(tag names.CloudCredentialTag) error {
	revert, err := st.modelsToRevert(tag)
	if err != nil {
		logger.Warningf("could not figure out if models for credential %v need to revert: %v", tag.Id(), err)
	}

	for m, closer := range revert {
		if err := m.st.updateModelCredentialInvalid(m.UUID(), "", false); err != nil {
			return errors.Trace(err)
		}
		if err := m.maybeRevertModelStatus(); err != nil {
			logger.Warningf("could not revert status for model %v: %v", m.UUID(), err)
		}
		_ = closer()
	}
	return nil
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
