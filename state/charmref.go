// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2/txn"
)

func charmIncRefOps(st modelBackend, appName string, curl *charm.URL, canCreate bool) ([]txn.Op, error) {
	refcounts, closer := st.getCollection(refcountsC)
	defer closer()

	getIncRefOp := nsRefcounts.CreateOrIncRefOp
	if !canCreate {
		getIncRefOp = nsRefcounts.StrictIncRefOp
	}

	settingsKey := applicationSettingsKey(appName, curl)
	settingsOp, err := getIncRefOp(refcounts, settingsKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	charmKey := charmGlobalKey(curl)
	charmOp, err := getIncRefOp(refcounts, charmKey)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return []txn.Op{
		settingsOp,
		charmOp,
	}, nil
}

func charmDecRefOps(st modelBackend, appName string, curl *charm.URL) ([]txn.Op, error) {
	refcounts, closer := st.getCollection(refcountsC)
	defer closer()

	charmKey := charmGlobalKey(curl)
	charmOp, err := nsRefcounts.AliveDecRefOp(refcounts, charmKey)

	settingsKey := applicationSettingsKey(appName, curl)
	settingsOp, isFinal, err := nsRefcounts.DyingDecRefOp(refcounts, settingsKey)
	if err != nil {
		return nil, errors.Trace(err)
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
