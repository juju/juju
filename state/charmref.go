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
// to a charm and its per-application settings document. It will fail if
// the charm is not Alive.
func appCharmIncRefOps(st modelBackend, appName string, curl *charm.URL, canCreate bool) ([]txn.Op, error) {

	charms, closer := st.getCollection(charmsC)
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

	refcounts, closer := st.getCollection(refcountsC)
	defer closer()

	getIncRefOp := nsRefcounts.CreateOrIncRefOp
	if !canCreate {
		getIncRefOp = nsRefcounts.StrictIncRefOp
	}
	settingsKey := applicationSettingsKey(appName, curl)
	settingsOp, err := getIncRefOp(refcounts, settingsKey)
	if err != nil {
		return nil, errors.Annotate(err, "settings reference")
	}
	charmKey := charmGlobalKey(curl)
	charmOp, err := getIncRefOp(refcounts, charmKey)
	if err != nil {
		return nil, errors.Annotate(err, "charm reference")
	}

	return append(checkOps, settingsOp, charmOp), nil
}

// appCharmDecRefOps returns the operations necessary to delete a
// reference to a charm and its per-application settings document. If no
// references to a given (app, charm) pair remain, the operations
// returned will also remove the settings document for that pair.
func appCharmDecRefOps(st modelBackend, appName string, curl *charm.URL) ([]txn.Op, error) {

	refcounts, closer := st.getCollection(refcountsC)
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

	ops := []txn.Op{settingsOp, charmOp}
	if isFinal {
		ops = append(ops, txn.Op{
			C:      settingsC,
			Id:     settingsKey,
			Remove: true,
		})
	}
	return ops, nil
}

// charmDestroyOps implements the logic of charm.Destroy.
func charmDestroyOps(st modelBackend, curl *charm.URL) ([]txn.Op, error) {

	if curl.Schema != "local" {
		// local charms keep a document around to prevent reuse
		// of charm URLs, which several components believe to be
		// unique keys (this is always true within a model).
		//
		// it's not so much that it's bad to delete store
		// charms; but we don't have a way to reinstate them
		// once purged, so we don't allow removal in the first
		// place.
		return nil, errors.New("cannot destroy non-local charms")
	}

	charms, closer := st.getCollection(charmsC)
	defer closer()

	charmKey := curl.String()
	charmOp, err := nsLife.destroyOp(charms, charmKey, nil)
	if err != nil {
		return nil, errors.Annotate(err, "charm")
	}

	refcounts, closer := st.getCollection(refcountsC)
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

// charmRemoveOps implements the logic of charm.Remove.
func charmRemoveOps(st modelBackend, curl *charm.URL) ([]txn.Op, error) {

	charms, closer := st.getCollection(charmsC)
	defer closer()

	// NOTE: we do *not* actually remove the charm document, to
	// prevent its URL from being recycled, and breaking caches.
	// The "remove" terminology refers to the client's view of the
	// change (after which the charm really will be inaccessible).
	charmKey := curl.String()
	charmOp, err := nsLife.dieOp(charms, charmKey, nil)
	if err != nil {
		return nil, errors.Annotate(err, "charm")
	}
	return []txn.Op{charmOp}, nil
}
