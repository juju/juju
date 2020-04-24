// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/cloud"
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

	if err := st.InvalidateCloudCredential(tag, reason); err != nil {
		return errors.Trace(err)
	}
	if err := st.suspendCredentialModels(tag); err != nil {
		// These updates are optimistic. If they fail, it's unfortunate but we are not going to stop the call.
		logger.Warningf("could not suspend models that use credential %v: %v", tag.Id(), err)
	}
	return nil
}

func (st *State) suspendCredentialModels(tag names.CloudCredentialTag) error {
	models, err := st.modelsWithCredential(tag)
	if err != nil {
		return errors.Annotatef(err, "could not determine what models use credential %v", tag.Id())
	}
	doc := statusDoc{
		Status:     status.Suspended,
		StatusInfo: "suspended since cloud credential is not valid",
		Updated:    timeOrNow(nil, st.clock()).UnixNano(),
	}
	for _, m := range models {
		one, closer, err := st.model(m.UUID)
		if err != nil {
			// Something has gone wrong with this model... keep going.
			logger.Warningf("model %v error: %v", m.UUID, err)
			continue
		}
		defer closer()
		if _, err = probablyUpdateStatusHistory(one.st.db(), one.globalKey(), doc); err != nil {
			// We do not want to stop processing the rest of the models.
			logger.Warningf("%v", err)
		}
	}
	return nil
}

// ValidateCloudCredential validates new cloud credential for this model.
func (m *Model) ValidateCloudCredential(tag names.CloudCredentialTag, credential cloud.Credential) error {
	aCloud, err := m.st.Cloud(m.CloudName())
	if err != nil {
		return errors.Annotatef(err, "getting cloud %q", m.CloudName())
	}

	err = validateCredentialForCloud(aCloud, tag, convertCloudCredentialToState(tag, credential))
	if err != nil {
		return errors.Annotatef(err, "validating credential %q for cloud %q", tag.Id(), aCloud.Name)
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
	aCloud, err := m.st.Cloud(m.CloudName())
	if err != nil {
		return false, errors.Annotatef(err, "getting cloud %q", m.CloudName())
	}
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
		// Must be a valid credential that is already on the controller.
		credential, err := m.st.CloudCredential(tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !credential.IsValid() {
			return nil, errors.NotValidf("credential %q", tag.Id())
		}
		if err := validateCredentialForCloud(aCloud, tag, credential); err != nil {
			return nil, errors.Trace(err)
		}
		return []txn.Op{{
			C:      modelsC,
			Id:     m.doc.UUID,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{{"cloud-credential", tag.Id()}}}},
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

// WatchModelCredential returns a new NotifyWatcher that watches
// a model reference to a cloud credential.
func (m *Model) WatchModelCredential() NotifyWatcher {
	current := m.doc.CloudCredential
	filter := func(id interface{}) bool {
		id, ok := id.(string)
		if !ok || id != m.doc.UUID {
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
