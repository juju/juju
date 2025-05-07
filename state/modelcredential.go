// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	jujutxn "github.com/juju/txn/v3"
)

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
