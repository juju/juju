// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/version"
)

// guiSettingsDoc represents the Juju GUI settings in MongoDB.
type guiSettingsDoc struct {
	// CurrentVersion is the version of the Juju GUI currently served by
	// the controller when requesting the GUI via HTTP.
	CurrentVersion version.Number `bson:"current-version"`
}

// GUISetVersion sets the Juju GUI version that the controller must serve.
func (st *State) GUISetVersion(vers version.Number) error {
	// Check that the provided version is actually present in the GUI storage.
	storage, err := st.GUIStorage()
	if err != nil {
		return errors.Annotate(err, "cannot open GUI storage")
	}
	defer storage.Close()
	if _, err = storage.Metadata(vers.String()); err != nil {
		return errors.Annotatef(err, "cannot find %q GUI version in the storage", vers)
	}

	// Set the current version.
	settings, closer := st.db().GetCollection(guisettingsC)
	defer closer()
	if _, err = settings.Writeable().Upsert(nil, bson.D{{"current-version", vers}}); err != nil {
		return errors.Annotate(err, "cannot set current GUI version")
	}
	return nil
}

// GUIVersion returns the Juju GUI version currently served by the controller.
func (st *State) GUIVersion() (vers version.Number, err error) {
	settings, closer := st.db().GetCollection(guisettingsC)
	defer closer()

	// Retrieve the settings document.
	var doc guiSettingsDoc
	err = settings.Find(nil).Select(bson.D{{"current-version", 1}}).One(&doc)
	if err == nil {
		return doc.CurrentVersion, nil
	}
	if err == mgo.ErrNotFound {
		return vers, errors.NotFoundf("Juju GUI version")
	}
	return vers, errors.Trace(err)
}
