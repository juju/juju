// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/secrets"
)

// Until we add 3.0 upgrade steps, keep static analysis happy.
var _ = func() {
	_ = runForAllModelStates(nil, nil)
	_ = applyToAllModelSettings(nil, nil)
}

// runForAllModelStates will run runner function for every model passing a state
// for that model.
func runForAllModelStates(pool *StatePool, runner func(st *State) error) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}
	models, closer := st.db().GetCollection(modelsC)
	defer closer()

	var modelDocs []bson.M
	err = models.Find(nil).Select(bson.M{"_id": 1}).All(&modelDocs)
	if err != nil {
		return errors.Annotate(err, "failed to read models")
	}

	for _, modelDoc := range modelDocs {
		modelUUID := modelDoc["_id"].(string)
		model, err := pool.Get(modelUUID)
		if err != nil {
			return errors.Annotatef(err, "failed to open model %q", modelUUID)
		}
		defer func() {
			model.Release()
		}()
		if err := runner(model.State); err != nil {
			return errors.Annotatef(err, "model UUID %q", modelUUID)
		}
	}
	return nil
}

// applyToAllModelSettings iterates the model settings documents and applies the
// passed in function to them.  If the function returns 'true' it indicates the
// settings have been modified, and they should be written back to the
// database.
// Note that if there are any problems with updating settings, then none of the
// changes will be applied, as they are all updated in a single transaction.
func applyToAllModelSettings(st *State, change func(*settingsDoc) (bool, error)) error {
	uuids, err := st.AllModelUUIDs()
	if err != nil {
		return errors.Trace(err)
	}

	coll, closer := st.db().GetRawCollection(settingsC)
	defer closer()

	var ids []string
	for _, uuid := range uuids {
		ids = append(ids, uuid+":e")
	}

	iter := coll.Find(bson.M{"_id": bson.M{"$in": ids}}).Iter()
	defer iter.Close()

	var ops []txn.Op
	var doc settingsDoc
	for iter.Next(&doc) {
		settingsChanged, err := change(&doc)
		if err != nil {
			return errors.Trace(err)
		}
		if settingsChanged {
			ops = append(ops, txn.Op{
				C:      settingsC,
				Id:     doc.DocID,
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{"settings": doc.Settings}},
			})
		}
	}
	if err := iter.Close(); err != nil {
		return errors.Trace(err)
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// EnsureInitalRefCountForExternalSecretBackends creates an initial refcount for each
// external secret backend if there is no one found.
func EnsureInitalRefCountForExternalSecretBackends(pool *StatePool) error {
	st, err := pool.SystemState()
	if err != nil {
		return errors.Trace(err)
	}

	backends := NewSecretBackends(st)
	allBackends, err := backends.ListSecretBackends()
	if err != nil {
		return errors.Annotate(err, "loading secret backends")
	}
	refCountCollection, ccloser := st.db().GetCollection(globalRefcountsC)
	defer ccloser()
	var ops []txn.Op
	for _, backend := range allBackends {
		if secrets.IsInternalSecretBackendID(backend.ID) {
			continue
		}
		_, err := nsRefcounts.read(refCountCollection, secretBackendRefCountKey(backend.ID))
		if err == nil {
			continue
		}
		if !errors.Is(err, errors.NotFound) {
			return errors.Annotatef(err, "cannot read refcount for backend %q", backend.ID)
		}
		refOps, err := st.createSecretBackendRefCountOp(backend.ID)
		if err != nil {
			return errors.Annotatef(err, "cannot get creating refcount op for backend %q", backend.ID)
		}
		ops = append(ops, refOps...)
	}
	if len(ops) > 0 {
		return errors.Trace(st.runRawTransaction(ops))
	}
	return nil
}

// EnsureApplicationCharmOriginsNormalised fixes application charm origins which may
// have been broken
//
// Previous versions of the Juju server and clients have treated applications charm
// origins very loosely, particularly during `refresh -- switch`s. The server performed
// no validation on origins received from the client, and client often mutated them
// incorrectly. For instance, when switching from a ch charm to local, pylibjuju simply
// send back a copy of the ch charm origin, whereas the CLI only set the source to local.
// Both resulted in incorrect/invalidate origins.
//
// Calculate the origin Source and Revision from the charm url. Ensure ID, Hash and Channel
// are dropped from local charm. Keep ID, Hash and Channel (for ch charms) and Platform (always)
// we get from the origin. We can trust these since supported clients cannot break these
//
// This was fixed in pylibjuju 3.2.3.0 and juju 3.3.0. As of writing, no versions of the
// server validate new charm origins on calls to SetCharm. Ideally, the client shouldn't
// handle charm origins at all, being an implementation detail. But this will probably have
// to wait until the api re-write
//
// https://bugs.launchpad.net/juju/+bug/2039267
// https://github.com/juju/python-libjuju/issues/962
//
// TODO: Drop this step once we have confidence in our application charm origins
func EnsureApplicationCharmOriginsNormalised(pool *StatePool) error {
	return errors.Trace(runForAllModelStates(pool, func(st *State) error {
		allApps, err := st.AllApplications()
		if err != nil {
			return errors.Annotate(err, "loading applications")
		}

		var ops []txn.Op

		for _, app := range allApps {
			origin := app.CharmOrigin()
			if origin == nil {
				return errors.Errorf("application %q has no origin", app.Name())
			}
			curlStr, _ := app.CharmURL()
			if curlStr == nil {
				return errors.Errorf("application %q has no charm url", app.Name())
			}
			curl, err := charm.ParseURL(*curlStr)
			if err != nil {
				return errors.Annotatef(err, "parsing charm url %q", *curlStr)
			}
			if curl.Revision == -1 {
				return errors.Errorf("charm url %q has no revision", curl.String())
			}

			if charm.Local.Matches(curl.Schema) {
				origin.Source = corecharm.Local.String()
				origin.Channel = nil
				origin.Hash = ""
				origin.ID = ""
				origin.Revision = &curl.Revision
			} else if charm.CharmHub.Matches(curl.Schema) {
				origin.Source = corecharm.CharmHub.String()
				origin.Revision = &curl.Revision
			} else {
				return errors.Errorf("Unrecognised schema charm url schema %q", curl.Schema)
			}

			ops = append(ops, txn.Op{
				C:      applicationsC,
				Id:     app.doc.DocID,
				Assert: txn.DocExists,
				Update: bson.D{{"$set", bson.D{{"charm-origin", origin}}}},
			})
		}
		if len(ops) > 0 {
			return errors.Trace(st.runRawTransaction(ops))
		}
		return nil
	}))
}
