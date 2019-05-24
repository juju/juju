// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v2"
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

	tag, exists := m.CloudCredential()
	if !exists {
		// Model is on the cloud that does not require auth - nothing to do.
		return nil
	}

	if err := st.InvalidateCloudCredential(tag, reason); err != nil {
		return errors.Trace(err)
	}
	if err := st.treatModelsForCredential(tag, false); err != nil {
		// These updates are optimistic. If they fail, it's unfortunate but we are not going to stop the call.
		logger.Warningf("could not suspend models that use credential %v: %v", tag.Id(), err)
	}
	return nil
}

func (st *State) treatModelsForCredential(tag names.CloudCredentialTag, validCredential bool) error {
	models, err := st.modelsWithCredential(tag)
	if err != nil {
		return errors.Annotatef(err, "could not determine what models use credential %v", tag.Id())
	}
	f := func(m *Model) error {
		return errors.Annotatef(m.Suspend("suspended since cloud credential is not valid"),
			"could not suspend model %v when its credential %v became invalid", m.UUID(), tag.Id())
	}
	if validCredential {
		f = func(m *Model) error {
			return errors.Annotatef(m.Unsuspend(),
				"could not unsuspend model %v when its credential %v became valid", m.UUID(), tag.Id())
		}
	}
	for _, m := range models {
		newSt, err := st.newStateNoWorkers(m.UUID)
		// We explicitly don't start the workers.
		if err != nil {
			// This model could have been removed.
			if errors.IsNotFound(err) {
				continue
			}
			return errors.Trace(err)
		}
		defer newSt.Close()

		aModel, err := newSt.Model()
		if err != nil {
			return errors.Trace(err)
		}
		if err := f(aModel); err != nil {
			// We do not want to stop processing the rest of the models.
			logger.Warningf("%v", err)
		}
	}
	return nil
}

// ValidateCloudCredential validates new cloud credential for this model.
func (m *Model) ValidateCloudCredential(tag names.CloudCredentialTag, credential cloud.Credential) error {
	cloud, err := m.st.Cloud(m.Cloud())
	if err != nil {
		return errors.Annotatef(err, "getting cloud %q", m.Cloud())
	}

	err = validateCredentialForCloud(cloud, tag, convertCloudCredentialToState(tag, credential))
	if err != nil {
		return errors.Annotatef(err, "validating credential %q for cloud %q", tag.Id(), cloud.Name)
	}
	return nil
}

// SetCloudCredential sets new cloud credential for this model.
// Returned bool indicates if model credential was set.
func (m *Model) SetCloudCredential(tag names.CloudCredentialTag) (bool, error) {
	cloud, err := m.st.Cloud(m.Cloud())
	if err != nil {
		return false, errors.Annotatef(err, "getting cloud %q", m.Cloud())
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
		if err := validateCredentialForCloud(cloud, tag, credential); err != nil {
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
	// We would not be able to set a credential if it was invalid, so the model must become unsuspended after this call.
	if err := m.Unsuspend(); err != nil {
		logger.Warningf("could not change the status of model %v from suspended", m.UUID())
	}
	return updating, m.Refresh()
}

// Suspend puts model into suspended status.
func (m *Model) Suspend(msg string) error {
	statusInfo := status.StatusInfo{
		Status:  status.Suspended,
		Message: msg,
	}
	return errors.Annotatef(m.SetStatus(statusInfo), "could not update status for model %v to suspended", m.UUID())
}

// Unsuspend reverts model to whatever status it was in before it got suspended.
// If the model is not currently suspended, do nothing.
func (m *Model) Unsuspend() error {
	current, err := m.Status()
	if err != nil {
		return errors.Annotatef(err, "could not get current status")
	}
	if current.Status != status.Suspended {
		// Nothing to do - the model is not suspended.
		return nil
	}

	histories, err := m.StatusHistory(status.StatusHistoryFilter{Size: 2})
	if err != nil {
		return errors.Annotatef(err, "could not get status history to determine last known status prior to suspension")
	}
	// There are several situations to cater for here.
	// 1. Status histories have not been cleared.
	// 		In this case, we should get at least 2 entries where the latest one will be the "suspension" entry
	// 		and the second one will be the status that the model was in prior to suspension that we want to revert to.
	// 2. Status histories have been cleared.
	// 		We have no idea what the model status was prior to suspension, so we will optimistically set it to 'available'.
	if len(histories) > 1 {
		return errors.Trace(m.SetStatus(status.StatusInfo{Status: histories[1].Status, Message: histories[1].Message}))
	}
	return errors.Trace(m.SetStatus(status.StatusInfo{Status: status.Available}))
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
