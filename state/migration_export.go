// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/state/migration"
)

// Export the current environment for the State. If a different environment
// is required, the caller is expected to use st.ForEnviron(...) and close
// the session as required.
func (st *State) Export() (migration.Description, error) {
	environment, err := st.Environment()
	if err != nil {
		return nil, errors.Trace(err)
	}

	export := exporter{
		st:          st,
		environment: environment,
	}

	settings, err := export.readAllSettings()
	if err != nil {
		return nil, errors.Trace(err)
	}

	envConfig, found := settings[environGlobalKey]
	if !found {
		return nil, errors.New("missing environ config")
	}

	args := migration.ModelArgs{
		Owner:              environment.Owner(),
		Config:             envConfig.Settings,
		LatestToolsVersion: environment.LatestToolsVersion(),
	}
	result := migration.NewDescription(args)

	export.model = result.Model()
	if err := export.environmentUsers(); err != nil {
		return nil, errors.Trace(err)
	}
	// Add machines...

	return result, nil
}

type exporter struct {
	st          *State
	environment *Environment
	model       migration.Model
}

func (e *exporter) environmentUsers() error {
	users, err := e.environment.Users()
	if err != nil {
		return errors.Trace(err)
	}
	lastConnections, err := e.readLastConnectionTimes()
	if err != nil {
		return errors.Trace(err)
	}

	for _, user := range users {
		lastConn := lastConnections[strings.ToLower(user.UserName())]
		arg := migration.UserArgs{
			Name:           user.UserTag(),
			DisplayName:    user.DisplayName(),
			CreatedBy:      names.NewUserTag(user.CreatedBy()),
			DateCreated:    user.DateCreated(),
			LastConnection: lastConn,
			ReadOnly:       user.ReadOnly(),
		}
		e.model.AddUser(arg)
	}
	return nil
}

func (e *exporter) readLastConnectionTimes() (map[string]time.Time, error) {
	lastConnections, closer := e.st.getCollection(envUserLastConnectionC)
	defer closer()

	var docs []envUserLastConnectionDoc
	if err := lastConnections.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]time.Time)
	for _, doc := range docs {
		result[doc.UserName] = doc.LastConnection.UTC()
	}
	return result, nil
}

func (e *exporter) readAllSettings() (map[string]settingsDoc, error) {
	settings, closer := e.st.getCollection(settingsC)
	defer closer()

	var docs []settingsDoc
	if err := settings.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]settingsDoc)
	for _, doc := range docs {
		key := e.st.localID(doc.DocID)
		result[key] = doc
	}
	return result, nil
}
