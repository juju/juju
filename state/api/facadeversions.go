// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// FacadeVersions lists the best version of facades that we know about. This
// will be used to pick out a default version for communication, given the list
// of known versions that the API server tells us it is capable of supporting.
var FacadeVersions = map[string]int{
	"Agent":                0,
	"AllWatcher":           0,
	"Deployer":             0,
	"KeyUpdater":           0,
	"Machiner":             0,
	"Networker":            0,
	"StringsWatcher":       0,
	"Environment":          0,
	"KeyManager":           0,
	"Logger":               0,
	"Pinger":               0,
	"Provisioner":          0,
	"RelationUnitsWatcher": 0,
	"UserManager":          0,
	"CharmRevisionUpdater": 0,
	"Client":               0,
	"NotifyWatcher":        0,
	"Upgrader":             0,
	"Firewaller":           0,
	"Rsyslog":              0,
	"Uniter":               0,
}
