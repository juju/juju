// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
)

// UpgradeSeriesLockApplicationIntersect returns application names represented
// by the input machine's units that also have units on other machines that are
// locked for upgrade.
func (st *State) UpgradeSeriesLockApplicationIntersect(machineId string) ([]string, error) {
	locks, err := st.getAllUpgradeSeriesLocks()
	if err != nil {
		return nil, errors.Annotatef(err,
			"retrieving intersecting upgrade series applications for machine %v", machineId)
	}

	machineApps := set.NewStrings()
	otherApps := set.NewStrings()

	// Use the lock's unit record map to accrue sets for the input machine,
	// and for all others that are currently locked.
	for _, lock := range locks {
		apps := set.NewStrings()
		for unit := range lock.UnitStatuses {
			app, err := names.UnitApplication(unit)
			if err != nil {
				return nil, errors.Trace(err)
			}
			apps.Add(app)
		}

		if machineId == lock.Id {
			machineApps = machineApps.Union(apps)
		} else {
			otherApps = otherApps.Union(apps)
		}
	}
	return machineApps.Intersection(otherApps).Values(), nil
}

func (st *State) getAllUpgradeSeriesLocks() ([]upgradeSeriesLockDoc, error) {
	coll, closer := st.db().GetCollection(machineUpgradeSeriesLocksC)
	defer closer()

	var docs []upgradeSeriesLockDoc
	err := coll.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "retrieving upgrade series locks")
	}
	return docs, nil
}
