// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/model"
	"gopkg.in/mgo.v2"
)

// LXDProfileUpgradeStatus returns the lxd profile upgrade status.
func (m *Machine) LXDProfileUpgradeStatus() (model.LXDProfileUpgradeStatus, error) {
	// TODO: (Simon) - how do we get this back?
	coll, closer := m.st.db().GetCollection(machinesC)
	defer closer()

	var doc machineDoc
	err := coll.Find(m.Id()).One(&doc)
	if err == mgo.ErrNotFound {
		return "", errors.NotFoundf("machine %q", m.Id())
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	return doc.UpgradeCharmProfileComplete, nil
}
