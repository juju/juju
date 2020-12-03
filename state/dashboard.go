// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// dashboardSettingsDoc represents the Juju Dashboard settings in MongoDB.
type dashboardSettingsDoc struct {
	// CurrentVersion is the version of the Juju Dashboard currently served by
	// the controller when requesting the Dashboard via HTTP.
	CurrentVersion version.Number `bson:"current-version"`
}

// DashboardSetVersion sets the Juju Dashboard version that the controller must serve.
func (st *State) DashboardSetVersion(vers version.Number) error {
	// Check that the provided version is actually present in the Dashboard storage.
	storage, err := st.DashboardStorage()
	if err != nil {
		return errors.Annotate(err, "cannot open Dashboard storage")
	}
	defer storage.Close()
	if _, err = storage.Metadata(vers.String()); err != nil {
		return errors.Annotatef(err, "cannot find %q Dashboard version in the storage", vers)
	}

	// Set the current version.
	settings, closer := st.db().GetCollection(guisettingsC)
	defer closer()
	if _, err = settings.Writeable().Upsert(nil, bson.D{{"current-version", vers}}); err != nil {
		return errors.Annotate(err, "cannot set current Dashboard version")
	}
	return nil
}

// DashboardVersion returns the Juju Dashboard version currently served by the controller.
func (st *State) DashboardVersion() (vers version.Number, err error) {
	settings, closer := st.db().GetCollection(guisettingsC)
	defer closer()

	// Retrieve the settings document.
	var doc dashboardSettingsDoc
	err = settings.Find(nil).Select(bson.D{{"current-version", 1}}).One(&doc)
	if err == nil {
		return doc.CurrentVersion, nil
	}
	if err == mgo.ErrNotFound {
		return vers, errors.NotFoundf("Juju Dashboard version")
	}
	return vers, errors.Trace(err)
}
