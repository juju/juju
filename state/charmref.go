// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2/txn"
)

var errCharmInUse = errors.New("charm in use")

// appCharmIncRefOps returns the operations necessary to record a reference
// to a charm and its per-application settings and storage constraints
// documents. It will fail if the charm is not Alive.
func appCharmIncRefOps(mb modelBackend, appName string, curl *charm.URL, canCreate bool) ([]txn.Op, error) {
	charms, closer := mb.db().GetCollection(charmsC)
	defer closer()

	// If we're migrating. charm document will not be present. But
	// if we're not migrating, we need to check the charm is alive.
	var checkOps []txn.Op
	count, err := charms.FindId(curl.String()).Count()
	if err != nil {
		return nil, errors.Annotate(err, "charm")
	} else if count != 0 {
		checkOp, err := nsLife.aliveOp(charms, curl.String())
		if err != nil {
			return nil, errors.Annotate(err, "charm")
		}
		checkOps = []txn.Op{checkOp}
	}

	refcounts, closer := mb.db().GetCollection(refcountsC)
	defer closer()

	getIncRefOp := nsRefcounts.CreateOrIncRefOp
	if !canCreate {
		getIncRefOp = nsRefcounts.StrictIncRefOp
	}
	settingsKey := applicationSettingsKey(appName, curl)
	settingsOp, err := getIncRefOp(refcounts, settingsKey, 1)
	if err != nil {
		return nil, errors.Annotate(err, "settings reference")
	}
	storageConstraintsKey := applicationStorageConstraintsKey(appName, curl)
	storageConstraintsOp, err := getIncRefOp(refcounts, storageConstraintsKey, 1)
	if err != nil {
		return nil, errors.Annotate(err, "storage constraints reference")
	}
	charmKey := charmGlobalKey(curl)
	charmOp, err := getIncRefOp(refcounts, charmKey, 1)
	if err != nil {
		return nil, errors.Annotate(err, "charm reference")
	}

	return append(checkOps, settingsOp, storageConstraintsOp, charmOp), nil
}

// appCharmDecRefOps returns the operations necessary to delete a
// reference to a charm and its per-application settings and storage
// constraints document.
// If maybeDoFinal is true, and no references to a given (app, charm) pair
// remain, the operations returned will also remove the settings and
// storage constraints documents for that pair, and schedule a cleanup
// to see if the charm itself is now unreferenced and can be tidied
// away itself.
func appCharmDecRefOps(st modelBackend, appName string, curl *charm.URL, maybeDoFinal bool) ([]txn.Op, error) {
	refcounts, closer := st.db().GetCollection(refcountsC)
	defer closer()

	charmKey := charmGlobalKey(curl)
	charmOp, err := nsRefcounts.AliveDecRefOp(refcounts, charmKey)
	if err != nil {
		return nil, errors.Annotate(err, "charm reference")
	}

	settingsKey := applicationSettingsKey(appName, curl)
	settingsOp, isFinal, err := nsRefcounts.DyingDecRefOp(refcounts, settingsKey)
	if err != nil {
		return nil, errors.Annotatef(err, "settings reference %s", settingsKey)
	}

	storageConstraintsKey := applicationStorageConstraintsKey(appName, curl)
	storageConstraintsOp, _, err := nsRefcounts.DyingDecRefOp(refcounts, storageConstraintsKey)
	if err != nil {
		return nil, errors.Annotatef(err, "storage constraints reference %s", storageConstraintsKey)
	}

	ops := []txn.Op{settingsOp, storageConstraintsOp, charmOp}
	if isFinal && maybeDoFinal {
		// XXX(fwereade): this construction, in common with ~all
		// our refcount logic, is safe in parallel but not in
		// serial. If this logic is used twice while composing a
		// single transaction, the removal won't be triggered.
		// see `Application.removeOps` for the workaround.
		ops = append(ops, finalAppCharmRemoveOps(appName, curl)...)
	}
	return ops, nil
}

// finalAppCharmRemoveOps returns operations to delete the settings
// and storage constraints documents and queue a charm cleanup.
func finalAppCharmRemoveOps(appName string, curl *charm.URL) []txn.Op {
	settingsKey := applicationSettingsKey(appName, curl)
	removeSettingsOp := txn.Op{
		C:      settingsC,
		Id:     settingsKey,
		Remove: true,
	}
	storageConstraintsKey := applicationStorageConstraintsKey(appName, curl)
	removeStorageConstraintsOp := removeStorageConstraintsOp(storageConstraintsKey)
	cleanupOp := newCleanupOp(cleanupCharm, curl.String())
	return []txn.Op{removeSettingsOp, removeStorageConstraintsOp, cleanupOp}
}

// charmDestroyOps implements the logic of charm.Destroy.
func charmDestroyOps(st modelBackend, curl *charm.URL) ([]txn.Op, error) {
	db := st.db()
	charms, closer := db.GetCollection(charmsC)
	defer closer()

	charmKey := curl.String()
	charmOp, err := nsLife.destroyOp(charms, charmKey, nil)
	if err != nil {
		return nil, errors.Annotate(err, "charm")
	}

	refcounts, closer := db.GetCollection(refcountsC)
	defer closer()

	refcountKey := charmGlobalKey(curl)
	refcountOp, err := nsRefcounts.RemoveOp(refcounts, refcountKey, 0)
	switch errors.Cause(err) {
	case nil:
	case errRefcountChanged:
		return nil, errCharmInUse
	default:
		return nil, errors.Annotate(err, "charm reference")
	}

	return []txn.Op{charmOp, refcountOp}, nil
}
