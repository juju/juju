// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// facadeVersions lists the best version of facades that we know about. This
// will be used to pick out a default version for communication, given the list
// of known versions that the API server tells us it is capable of supporting.
// This map should be updated whenever the API server exposes a new version (so
// that the client will use it whenever it is available).
var facadeVersions = map[string]int{
	"Agent":                1,
	"AllWatcher":           0,
	"Backups":              0,
	"Deployer":             0,
	"DiskManager":          1,
	"KeyUpdater":           0,
	"HighAvailability":     1,
	"Machiner":             0,
	"Networker":            0,
	"StringsWatcher":       0,
	"Environment":          0,
	"KeyManager":           0,
	"Logger":               0,
	"MetricsManager":       0,
	"Pinger":               0,
	"Provisioner":          0,
	"Reboot":               1,
	"RelationUnitsWatcher": 0,
	"UserManager":          0,
	"CharmRevisionUpdater": 0,
	"Client":               0,
	"NotifyWatcher":        0,
	"Upgrader":             0,
	"Firewaller":           1,
	"Rsyslog":              0,
	"Uniter":               1,
	"Action":               0,
	"Service":              1,
}

// bestVersion tries to find the newest version in the version list that we can
// use.
func bestVersion(desiredVersion int, versions []int) int {
	best := 0
	for _, version := range versions {
		if version <= desiredVersion && version > best {
			best = version
		}
	}
	return best
}
