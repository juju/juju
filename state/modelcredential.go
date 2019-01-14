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

	return errors.Trace(st.InvalidateCloudCredential(tag, reason))
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
